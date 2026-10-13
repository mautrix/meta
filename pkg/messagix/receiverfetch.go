package messagix

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/receiverfetch"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

const receiverFetchProjectName = "LS_MediaReceiverFetch"

type receiverFetchToken struct {
	configID      string
	credential    string
	tag           string
	remainingUses int
	expiresAt     time.Time
}

type receiverFetchState struct {
	sync.Mutex
	keyID        byte
	publicKey    []byte
	keyExpiresAt time.Time
	tokens       []*receiverFetchToken
}

type ReceiverFetchMedia struct {
	URL      string
	MimeType string
	Width    int
	Height   int
}

type receiverFetchOHAIConfigs struct {
	Configurations struct {
		Configs []struct {
			KeyID     json.Number `json:"key_id"`
			PublicKey string      `json:"public_key"`
			KEMID     json.Number `json:"kem_id"`
			KDFID     json.Number `json:"kdf_id"`
			AEADID    json.Number `json:"aead_id"`
			ExpiresAt json.Number `json:"expiration_date"`
		} `json:"ohai_configs"`
	} `json:"xfb_ohai_configurations"`
}

type receiverFetchACSConfig struct {
	Config struct {
		ID              string      `json:"id"`
		CipherSuite     json.Number `json:"cipher_suite"`
		MaxEvals        json.Number `json:"max_evals"`
		RedemptionLimit json.Number `json:"redemption_limit"`
		ExpiresAt       json.Number `json:"expire_time"`
		TokenTTL        json.Number `json:"token_ttl"`
	} `json:"acs_config"`
}

type receiverFetchACSIssue struct {
	Issue struct {
		Evaluations []struct {
			Data string `json:"data"`
		} `json:"evaluation"`
	} `json:"xfb_messaging_acs_issue"`
}

type receiverFetchMediaResponse struct {
	Media *struct {
		URL      string      `json:"cdn_url"`
		MimeType string      `json:"mime_type"`
		Width    json.Number `json:"width"`
		Height   json.Number `json:"height"`
	} `json:"media_receiver_fetch_deidentified"`
}

func receiverFetchInt(number json.Number) int64 {
	value, err := number.Int64()
	if err != nil {
		return 0
	}
	return value
}

func parseReceiverFetchResponse(data []byte, result any) error {
	var envelope graphql.Response[json.RawMessage]
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	} else if err = envelope.AsError(); err != nil {
		return err
	} else if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
		return fmt.Errorf("response didn't contain data")
	}
	return json.Unmarshal(envelope.Data, result)
}

func (c *Client) receiverFetchGraphQL(ctx context.Context, name string, variables, result any) error {
	_, data, err := c.http.MakeGraphQLRequest(ctx, name, variables)
	if err != nil {
		return err
	}
	return parseReceiverFetchResponse(data, result)
}

func (c *Client) refreshReceiverFetchKey(ctx context.Context) error {
	if time.Now().Add(time.Minute).Before(c.receiverFetch.keyExpiresAt) {
		return nil
	}
	var data receiverFetchOHAIConfigs
	if err := c.receiverFetchGraphQL(ctx, "OhaiWebClientMessengerConfigsQuery", struct{}{}, &data); err != nil {
		return err
	}
	for _, key := range data.Configurations.Configs {
		expiresAt := time.Unix(receiverFetchInt(key.ExpiresAt), 0)
		id := receiverFetchInt(key.KeyID)
		if !time.Now().Add(time.Minute).Before(expiresAt) || id < 0 || id > 255 ||
			receiverFetchInt(key.KEMID) != 32 || receiverFetchInt(key.KDFID) != 1 || receiverFetchInt(key.AEADID) != 1 {
			continue
		}
		publicKey, err := hex.DecodeString(key.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid ohai public key: %w", err)
		}
		c.receiverFetch.keyID = byte(id)
		c.receiverFetch.publicKey = publicKey
		c.receiverFetch.keyExpiresAt = expiresAt
		return nil
	}
	return fmt.Errorf("no supported, valid ohai key in response")
}

func (c *Client) getReceiverFetchToken(ctx context.Context) (*receiverFetchToken, error) {
	for len(c.receiverFetch.tokens) > 0 {
		token := c.receiverFetch.tokens[0]
		if token.remainingUses > 0 && time.Now().Add(time.Minute).Before(token.expiresAt) {
			token.remainingUses--
			return token, nil
		}
		c.receiverFetch.tokens = c.receiverFetch.tokens[1:]
	}
	var data receiverFetchACSConfig
	if err := c.receiverFetchGraphQL(ctx, "MessagingWebACSGraphQLServerProviderConfigQuery", map[string]string{"project_name": receiverFetchProjectName}, &data); err != nil {
		return nil, err
	}
	config := data.Config
	configID := config.ID
	batchSize := int(min(receiverFetchInt(config.MaxEvals), 5))
	uses := int(receiverFetchInt(config.RedemptionLimit))
	if configID == "" || receiverFetchInt(config.CipherSuite) != 1 || batchSize <= 0 || uses <= 0 {
		return nil, fmt.Errorf("invalid or unsupported acs configuration")
	}
	expiresAt := time.Unix(receiverFetchInt(config.ExpiresAt), 0)
	if ttl := receiverFetchInt(config.TokenTTL); ttl > 0 {
		expiresAt = time.Now().Add(time.Duration(ttl) * time.Second)
	}
	nonces := make([]*receiverfetch.Nonce, batchSize)
	issueElements := make([]map[string]string, batchSize)
	for i := range nonces {
		nonce, blinded := receiverfetch.BlindNonce()
		nonces[i] = nonce
		issueElements[i] = map[string]string{"issue_data": blinded}
	}
	var issue receiverFetchACSIssue
	if err := c.receiverFetchGraphQL(ctx, "MessagingWebACSGraphQLServerProviderTokenIssuanceQuery", map[string]any{
		"projectName": receiverFetchProjectName,
		"configId":    configID,
		"message": map[string]any{
			"issue_element": issueElements,
			"request_proof": false,
		},
	}, &issue); err != nil {
		return nil, err
	}
	evaluations := issue.Issue.Evaluations
	if len(evaluations) != len(nonces) {
		return nil, fmt.Errorf("got %d acs evaluations for %d nonces", len(evaluations), len(nonces))
	}
	tokens := make([]*receiverFetchToken, len(nonces))
	for i, evaluation := range evaluations {
		var evaluated struct {
			Data string `json:"evaluation_data"`
		}
		if err := json.Unmarshal([]byte(evaluation.Data), &evaluated); err != nil {
			return nil, fmt.Errorf("failed to parse acs evaluation: %w", err)
		}
		credential, tag, err := nonces[i].Redeem(evaluated.Data)
		if err != nil {
			return nil, err
		}
		tokens[i] = &receiverFetchToken{
			configID:      configID,
			credential:    credential,
			tag:           tag,
			remainingUses: uses,
			expiresAt:     expiresAt,
		}
	}
	c.receiverFetch.tokens = tokens
	tokens[0].remainingUses--
	return tokens[0], nil
}

func (c *Client) FetchReceiverFetchMedia(ctx context.Context, receiverFetchID string) (*ReceiverFetchMedia, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	c.receiverFetch.Lock()
	defer c.receiverFetch.Unlock()
	if err := c.refreshReceiverFetchKey(ctx); err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		token, err := c.getReceiverFetchToken(ctx)
		if err != nil {
			return nil, err
		}
		doc := graphql.GraphQLDocs["MSGRMediaReceiverFetchDeidentifiedQuery"]
		variables, _ := json.Marshal(map[string]string{"receiverFetchId": receiverFetchID})
		form := url.Values{
			"doc_id":                   {doc.DocID},
			"fb_api_caller_class":      {doc.CallerClass},
			"fb_api_req_friendly_name": {doc.FriendlyName},
			"variables":                {string(variables)},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.facebook.com/api/graphql", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-FB-Friendly-Name", doc.FriendlyName)
		req.Header.Set("X-ACS-ATTRIBUTE-IDENTIFIER", "")
		req.Header.Set("X-ACS-CONFIG-ID", token.configID)
		req.Header.Set("X-ACS-CREDENTIAL", token.credential)
		req.Header.Set("X-ACS-PROJECT-NAME", receiverFetchProjectName)
		req.Header.Set("X-ACS-TAG", token.tag)
		binaryRequest := receiverfetch.EncodeRequest(req, []byte(form.Encode()))
		encapsulated, decrypt, err := receiverfetch.Encapsulate(c.receiverFetch.keyID, c.receiverFetch.publicKey, binaryRequest)
		if err != nil {
			return nil, err
		}
		_, response, err := c.http.MakeRequest(ctx, "https://meta-ohttp-relay-prod.fastly-edge.com/", http.MethodPost, http.Header{
			"Content-Type": {"message/ohttp-req"},
			"User-Agent":   {useragent.UserAgent},
			"Origin":       {c.GetEndpoint("base_url")},
		}, encapsulated, types.NONE)
		if err != nil {
			return nil, err
		}
		decrypted, err := decrypt(response)
		if err != nil {
			return nil, err
		}
		status, response, err := receiverfetch.DecodeResponse(decrypted)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			if attempt < 2 && (status == http.StatusTooManyRequests || status >= 500) {
				select {
				case <-time.After(time.Duration(attempt+1) * 3 * time.Second):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, fmt.Errorf("receiver fetch returned HTTP %d", status)
		}
		var data receiverFetchMediaResponse
		if err := parseReceiverFetchResponse(response, &data); err != nil {
			return nil, err
		}
		media := data.Media
		if media == nil || media.URL == "" {
			return nil, fmt.Errorf("receiver fetch response didn't contain media")
		}
		return &ReceiverFetchMedia{
			URL:      media.URL,
			MimeType: media.MimeType,
			Width:    int(receiverFetchInt(media.Width)),
			Height:   int(receiverFetchInt(media.Height)),
		}, nil
	}
}
