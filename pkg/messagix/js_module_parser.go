package messagix

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

var jsDatrPattern = regexp.MustCompile(`"_js_datr","([^"]+)"`)
var versionPattern = regexp.MustCompile(`__d\("LSVersion"[^)]+\)\{\w\.exports="(\d+)"\}`)

type BBoxContainer struct {
	BBox *BBox `json:"__bbox,omitempty"`
}

type BBox struct {
	Require []*ModuleEntry `json:"require"`
	Define  []*ModuleEntry `json:"define"`
	// Might also contain instances, markup, elements, contexts
}

type ModuleEntry struct {
	Name string
	Data []json.RawMessage
}

func (me *ModuleEntry) UnmarshalJSON(data []byte) error {
	var fullData []json.RawMessage
	err := json.Unmarshal(data, &fullData)
	if err != nil {
		return err
	}
	err = json.Unmarshal(fullData[0], &me.Name)
	if err != nil {
		return fmt.Errorf("failed to unmarshal ssjs entry name: %w", err)
	}
	me.Data = fullData[1:]
	return nil
}

type AttributeMap map[string]string
type ScriptTag struct {
	Attributes AttributeMap
	Content    string
}

type LinkTag struct {
	Attributes AttributeMap
}

type InputTag struct {
	Attributes AttributeMap
}

type ModuleParser struct {
	client *Client
	JSDatr string

	LS *table.LSTable
}

func (m *ModuleParser) SetClientInstance(cli *Client) {
	m.client = cli
}

func (m *ModuleParser) fetchPageData(ctx context.Context, page string) ([]byte, error) { // just log.fatal if theres an error because the library should not be able to continue then
	headers := m.client.buildHeaders(true, true)
	//headers.Set("host", m.client.getEndpoint("host"))
	_, responseBody, err := m.client.MakeRequest(ctx, page, "GET", headers, nil, types.NONE)
	return responseBody, err
}

func (m *ModuleParser) Load(ctx context.Context, page string) error {
	htmlData, err := m.fetchPageData(ctx, page)
	if err != nil {
		return err
	}
	if m.client.Platform.IsMessenger() && !strings.Contains(page, "login") && bytes.Contains(htmlData, []byte(`"USER_ID":"0"`)) {
		return ErrUserIDIsZero
	}

	doc, err := html.Parse(bytes.NewReader(htmlData))
	if err != nil {
		return fmt.Errorf("messagix-moduleparser: failed to parse doc string: %w", err)
	}

	scriptTags := m.findScriptTags(doc)
	for _, tag := range scriptTags {
		id := tag.Attributes["id"]
		switch id {
		case "envjson", "__eqmc":
			err = m.HandleRawJSON([]byte(tag.Content), id)
			if err != nil {
				return fmt.Errorf("failed to handle raw JSON: %w", err)
			}
		default:
			if tag.Content == "" {
				continue
			}
			var data *BBox
			err = json.Unmarshal([]byte(tag.Content), &data)
			if err != nil {
				if strings.HasPrefix(tag.Content, "requireLazy") {
					err = m.requireLazyModule(tag.Content)
					if err != nil {
						return err
					}
					continue
				}
				m.client.Logger.Trace().
					Str("script_tag_content", base64.StdEncoding.EncodeToString([]byte(tag.Content))).
					Msg("Errored script tag data")
				m.client.Logger.Warn().Err(err).Msg("Failed to parse script tag into bbox")
				continue
			}

			for _, mod := range data.Require {
				err = m.handleModule(mod)
				if err != nil {
					return err
				}
			}
		}
	}

	authenticated := m.client.IsAuthenticated()
	// on certain occasions, the server does not return the lightspeed data or version
	// when this is the case, the server "preloads" the js files in the link tags, so we need to loop through them until we can find the "LSVersion" module and extract the exported version string
	if m.client.configs.VersionID == 0 && authenticated {
		m.client.Logger.Warn().Msg("Version ID not found in index page")
		var doneCrawling bool
		linkTags := m.findLinkTags(doc)
		for _, tag := range linkTags {
			as := tag.Attributes["as"]
			href := tag.Attributes["href"]
			if as != "script" || href == "" {
				continue
			}

			doneCrawling, err = m.crawlJavascriptFile(ctx, href)
			if err != nil {
				return fmt.Errorf("messagix-moduleparser: failed to crawl js file %s (%w)", href, err)
			}
			if doneCrawling {
				break
			}
		}
		if !doneCrawling {
			for _, tag := range scriptTags {
				href := tag.Attributes["src"]
				if href == "" || !strings.HasPrefix(href, "https://") {
					continue
				}
				doneCrawling, err = m.crawlJavascriptFile(ctx, href)
				if err != nil {
					return fmt.Errorf("messagix-moduleparser: failed to crawl js file %s (%w)", href, err)
				}
				if doneCrawling {
					break
				}
			}
		}
		if !doneCrawling {
			return ErrVersionIDNotFound
		}
	}

	return nil
}

type BigPipe struct {
	JSMods *BBox `json:"jsmods,omitempty"`
}

func (m *ModuleParser) requireLazyModule(data string) error {
	moduleSplit := strings.Split(data[12:], "],")
	var moduleNames []string
	err := json.Unmarshal([]byte(moduleSplit[0]+"]"), &moduleNames)
	if err != nil {
		return fmt.Errorf("messagix-moduleparser: failed to get module names from requireLazy module (%w)", err)
	}

	for _, mName := range moduleNames {
		switch mName {
		case "__bigPipe":
			if len(data) < 5000 {
				continue
			}
			handleData := strings.Split(strings.Split(data, "bigPipe.onPageletArrive(")[1], ");requireLazy")[0]
			handleData = methods.PreprocessJSObject(handleData[:len(handleData)-6])
			var bigPipeData *BigPipe
			err = json.Unmarshal([]byte(handleData), &bigPipeData)
			if err != nil {
				return fmt.Errorf("messagix-moduleparser: failed to unmarshal BigPipe data (%w)", err)
			}

			for _, d := range bigPipeData.JSMods.Define {
				err := m.SSJSHandle(d.Data[2])
				if err != nil {
					return fmt.Errorf("messagix-moduleparser: failed to handle serverjs module (%w)", err)
				}
			}

			if m.client.cookies == nil {
				jsDatrMatches := jsDatrPattern.FindStringSubmatch(handleData)
				if len(jsDatrMatches) > 1 {
					m.JSDatr = jsDatrMatches[1]
				}
			}
		case "ServerJS":
			handleData := "{" + strings.Split(strings.Split(data, ".handle({")[1], ");requireLazy")[0]
			var moduleData *BBox
			err = json.Unmarshal([]byte(handleData), &moduleData)
			if err != nil {
				return fmt.Errorf("messagix-moduleparser: failed to unmarshal handleData[0] into struct *ModuleData (%w)", err)
			}

			for _, d := range moduleData.Define {
				err := m.SSJSHandle(d.Data[2])
				if err != nil {
					return fmt.Errorf("messagix-moduleparser: failed to handle serverjs module (%w)", err)
				}
			}
		case "HasteSupportData":
			handleSplit := strings.Split(data, ".handle({")
			if len(handleSplit) <= 2 {
				//log.Println("skipping HasteSupportData because it contains irrelevant stuff")
				continue
			}
			handleData := "{" + strings.Split(strings.Split(data, ".handle({")[2], ");requireLazy")[0]
			handleData = handleData[:len(handleData)-5]
			var moduleData *BBox
			err = json.Unmarshal([]byte(handleData), &moduleData)
			if err != nil {
				return fmt.Errorf("messagix-moduleparser: failed to unmarshal handleData[0] into struct *ModuleData (%w)", err)
			}
			for _, d := range moduleData.Define {
				err := m.SSJSHandle(d.Data[2])
				if err != nil {
					return fmt.Errorf("messagix-moduleparser: failed to handle hastesupportdata module (%w)", err)
				}
			}
		case "bootstrapWebSession":
			continue
		default:
			//log.Println(fmt.Sprintf("Skipping requireLazy module with name: %s", mName))
			continue
		}
	}
	return nil
}

func (m *ModuleParser) crawlJavascriptFile(ctx context.Context, href string) (bool, error) {
	_, jsContent, err := m.client.MakeRequest(ctx, href, "GET", http.Header{}, nil, types.NONE)
	if err != nil {
		return false, err
	}

	versionMatches := versionPattern.FindStringSubmatch(string(jsContent))
	if len(versionMatches) > 0 {
		versionInt, err := strconv.ParseInt(versionMatches[1], 10, 64)
		if err != nil {
			return false, err
		}
		m.client.Logger.Info().Int64("ls_version", versionInt).Msg("Found LSVersion in JS file")
		m.client.configs.VersionID = versionInt
		return true, nil
	}
	return false, nil
}

func (m *ModuleParser) handleModule(data *ModuleEntry) error {
	if len(data.Data) < 3 {
		return fmt.Errorf("module %s has less than 3 data elements", data.Name)
	}
	switch data.Name {
	case "ScheduledServerJS", "ScheduledServerJSWithCSS":
		if string(data.Data[0]) != `"handle"` {
			return fmt.Errorf("unexpected SSJS command %s", data.Data[0])
		}
		var datas []json.RawMessage
		err := json.Unmarshal(data.Data[2], &datas)
		if err != nil {
			return fmt.Errorf("failed to unmarshal ssjs list: %w", err)
		}
		for _, innerData := range datas {
			err = m.SSJSHandle(innerData)
			if err != nil {
				return fmt.Errorf("failed to handle serverjs module: %w", err)
			}
		}
	case "Bootloader":
		if string(data.Data[0]) != `"handlePayload"` {
			return fmt.Errorf("unexpected Bootloader command %s", data.Data[0])
		}
		err := m.HandleBootloaderPayload(data.Data[2], &m.client.configs.BrowserConfigTable.BootloaderConfig)
		if err != nil {
			return fmt.Errorf("failed to handle bootloader payload: %w", err)
		}
	case "HasteSupportData":
		//m.client.Logger.Debug().Bytes("data", data.Data[2]).Msg("Got haste support data")
	}
	return nil
}

type NodeProcessor func(*html.Node) interface{}

func (m *ModuleParser) processNode(n *html.Node, tag string, processor NodeProcessor) interface{} {
	if n.Type == html.ElementNode && n.Data == tag {
		return processor(n)
	}
	return nil
}

func (m *ModuleParser) recursiveSearch(n *html.Node, tag string, processor NodeProcessor) []interface{} {
	var result []interface{}

	if item := m.processNode(n, tag, processor); item != nil {
		result = append(result, item)
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		result = append(result, m.recursiveSearch(c, tag, processor)...)
	}

	return result
}

func (m *ModuleParser) findTags(tag string, processor NodeProcessor, n *html.Node) []interface{} {
	return m.recursiveSearch(n, tag, processor)
}

func (m *ModuleParser) findScriptTags(n *html.Node) []ScriptTag {
	processor := func(n *html.Node) interface{} {
		attributes := make(AttributeMap)
		for _, a := range n.Attr {
			attributes[a.Key] = a.Val
		}
		content := ""
		if n.FirstChild != nil {
			content = n.FirstChild.Data
		}
		return ScriptTag{Attributes: attributes, Content: strings.Replace(content, ",BootloaderConfig", ",", -1)}
	}

	tags := m.findTags("script", processor, n)
	scriptTags := make([]ScriptTag, len(tags))
	for i, t := range tags {
		scriptTags[i] = t.(ScriptTag)
	}
	return scriptTags
}

func (m *ModuleParser) findLinkTags(n *html.Node) []LinkTag {
	processor := func(n *html.Node) interface{} {
		attributes := make(AttributeMap)
		for _, a := range n.Attr {
			attributes[a.Key] = a.Val
		}
		return LinkTag{Attributes: attributes}
	}

	tags := m.findTags("link", processor, n)
	linkTags := make([]LinkTag, len(tags))
	for i, t := range tags {
		linkTags[i] = t.(LinkTag)
	}
	return linkTags
}
