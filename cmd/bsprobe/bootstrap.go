package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The Business Suite inbox is a Comet app: its runtime configuration is
// embedded in the initial HTML as a series of `require`/`define` blocks. This
// file extracts the values we know how to name, and — just as importantly —
// lists the blocks we do not, so field names get read off a real response
// instead of guessed into production code.

type Bootstrap struct {
	CurrentUserID string `json:"current_user_id"`
	BusinessID    string `json:"business_id,omitempty"`
	AssetID       string `json:"asset_id,omitempty"`
	MailboxID     string `json:"mailbox_id,omitempty"`
	PageID        string `json:"page_id,omitempty"`

	FBDTSG  string `json:"fb_dtsg"`
	LSD     string `json:"lsd"`
	Jazoest string `json:"jazoest"`
	AppID   string `json:"app_id"`
	Rev     string `json:"revision"`
	Haste   string `json:"haste_session,omitempty"`

	// Blocks records every `["Name",[],{...}]` key present in the document.
	// Anything interesting that is not yet parsed above shows up here.
	Blocks []string `json:"define_blocks"`
}

var (
	dtsgRe     = regexp.MustCompile(`\["DTSGInitialData",\s*\[\],\s*\{"token":"([^"]+)"`)
	dtsgAltRe  = regexp.MustCompile(`"dtsg":\s*\{"token":"([^"]+)"`)
	lsdRe      = regexp.MustCompile(`\["LSD",\s*\[\],\s*\{"token":"([^"]+)"`)
	revRe      = regexp.MustCompile(`"(?:client_revision|__spin_r|rev)":\s*(\d{5,})`)
	appIDRe    = regexp.MustCompile(`"(?:appID|app_id|APP_ID)":\s*"?(\d{6,})"?`)
	userIDRe   = regexp.MustCompile(`"(?:USER_ID|ACCOUNT_ID)":\s*"(\d+)"`)
	hasteRe    = regexp.MustCompile(`"haste_session":\s*"([^"]+)"`)
	blockRe    = regexp.MustCompile(`\["([A-Za-z0-9_]{3,60})",\s*\[\],\s*\{`)
	genericIDs = map[string]*regexp.Regexp{
		"business_id": regexp.MustCompile(`"business(?:_id|ID)":\s*"?(\d{6,})"?`),
		"asset_id":    regexp.MustCompile(`"asset(?:_id|ID)":\s*"?(\d{6,})"?`),
		"mailbox_id":  regexp.MustCompile(`"mailbox(?:_id|ID)":\s*"?(\d{6,})"?`),
		"page_id":     regexp.MustCompile(`"page(?:_id|ID)":\s*"?(\d{6,})"?`),
	}
)

func ParseBootstrap(html string) *Bootstrap {
	b := &Bootstrap{}

	if m := dtsgRe.FindStringSubmatch(html); m != nil {
		b.FBDTSG = m[1]
	} else if m := dtsgAltRe.FindStringSubmatch(html); m != nil {
		b.FBDTSG = m[1]
	}
	if m := lsdRe.FindStringSubmatch(html); m != nil {
		b.LSD = m[1]
	}
	if m := revRe.FindStringSubmatch(html); m != nil {
		b.Rev = m[1]
	}
	if m := appIDRe.FindStringSubmatch(html); m != nil {
		b.AppID = m[1]
	}
	if m := userIDRe.FindStringSubmatch(html); m != nil {
		b.CurrentUserID = m[1]
	}
	if m := hasteRe.FindStringSubmatch(html); m != nil {
		b.Haste = m[1]
	}
	if b.FBDTSG != "" {
		b.Jazoest = ComputeJazoest(b.FBDTSG)
	}

	for key, re := range genericIDs {
		m := re.FindStringSubmatch(html)
		if m == nil {
			continue
		}
		switch key {
		case "business_id":
			b.BusinessID = m[1]
		case "asset_id":
			b.AssetID = m[1]
		case "mailbox_id":
			b.MailboxID = m[1]
		case "page_id":
			b.PageID = m[1]
		}
	}

	seen := map[string]bool{}
	for _, m := range blockRe.FindAllStringSubmatch(html, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			b.Blocks = append(b.Blocks, m[1])
		}
	}
	sort.Strings(b.Blocks)
	return b
}

// ComputeJazoest mirrors the client-side derivation: the literal "2" followed
// by the sum of the UTF-8 byte values of fb_dtsg.
func ComputeJazoest(dtsg string) string {
	sum := 0
	for _, c := range []byte(dtsg) {
		sum += int(c)
	}
	return "2" + strconv.Itoa(sum)
}

// Redacted returns a copy safe to write to fixtures: tokens replaced, IDs kept.
func (b *Bootstrap) Redacted() *Bootstrap {
	c := *b
	if c.FBDTSG != "" {
		c.FBDTSG = "<redacted:secret>"
	}
	if c.LSD != "" {
		c.LSD = "<redacted:secret>"
	}
	if c.Jazoest != "" {
		c.Jazoest = "<redacted:secret>"
	}
	return &c
}

func cmdBootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	sessionPath := fs.String("session", defaultSessionFile, "session file")
	asset := fs.String("asset", "", "asset/page id to select (appended as a query param)")
	save := fs.String("save", "", "write the redacted bootstrap to this file")
	dumpHTML := fs.String("dump-html", "", "write the raw inbox HTML here (NOT redacted - do not commit)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s, err := LoadSession(*sessionPath)
	if err != nil {
		return err
	}

	target := businessHost + "/latest/inbox/all"
	if *asset != "" {
		target += "?asset_id=" + *asset
	}
	resp, body, err := s.Get(target)
	if err != nil {
		return err
	}
	fmt.Printf("GET %s -> %d (%d bytes)\n\n", target, resp.StatusCode, len(body))

	if *dumpHTML != "" {
		if err := os.MkdirAll(filepath.Dir(*dumpHTML), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(*dumpHTML, []byte(body), 0o600); err != nil {
			return err
		}
		fmt.Printf("raw html written to %s (contains live tokens - do not commit)\n\n", *dumpHTML)
	}

	b := ParseBootstrap(body)

	fmt.Println("extracted:")
	report := [][2]string{
		{"current_user_id", b.CurrentUserID},
		{"business_id", b.BusinessID},
		{"asset_id", b.AssetID},
		{"mailbox_id", b.MailboxID},
		{"page_id", b.PageID},
		{"app_id", b.AppID},
		{"revision", b.Rev},
		{"haste_session", b.Haste},
	}
	for _, kv := range report {
		v := kv[1]
		if v == "" {
			v = "-"
		}
		fmt.Printf("  %-16s %s\n", kv[0], v)
	}
	for _, kv := range [][2]string{{"fb_dtsg", b.FBDTSG}, {"lsd", b.LSD}, {"jazoest", b.Jazoest}} {
		status := "MISSING"
		if kv[1] != "" {
			status = fmt.Sprintf("present (len %d)", len(kv[1]))
		}
		fmt.Printf("  %-16s %s\n", kv[0], status)
	}

	fmt.Printf("\ndefine blocks in document: %d\n", len(b.Blocks))
	for _, blk := range b.Blocks {
		if strings.Contains(strings.ToLower(blk), "inbox") ||
			strings.Contains(strings.ToLower(blk), "business") ||
			strings.Contains(strings.ToLower(blk), "mailbox") ||
			strings.Contains(strings.ToLower(blk), "messenger") ||
			strings.Contains(strings.ToLower(blk), "lightspeed") ||
			strings.Contains(strings.ToLower(blk), "realtime") ||
			strings.Contains(strings.ToLower(blk), "mqtt") {
			fmt.Printf("  * %s\n", blk)
		}
	}
	fmt.Println("  (starred blocks look inbox/transport related - inspect these first)")

	if b.FBDTSG == "" {
		fmt.Println("\nfb_dtsg not found. Either the session is not authenticated, or the token")
		fmt.Println("moved to a block name this parser does not know yet. Check define blocks above.")
	}

	if *save != "" {
		if err := os.MkdirAll(filepath.Dir(*save), 0o755); err != nil {
			return err
		}
		out, err := json.MarshalIndent(b.Redacted(), "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*save, out, 0o644); err != nil {
			return err
		}
		fmt.Printf("\nredacted bootstrap written to %s\n", *save)
	}
	return nil
}
