package messagix

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/acs"
	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/ohai"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"
)

const (
	receiverFetchProjectName = "LS_MediaReceiverFetch"
	receiverFetchQuery       = "MSGRMediaReceiverFetchDeidentifiedQuery"
	ohaiRelayURL             = "https://meta-ohttp-relay-prod.fastly-edge.com/"
	deidentifiedGraphQLURL   = "https://www.facebook.com/api/graphql"
)

type receiverFetchState struct {
	lock      sync.Mutex
	ohaiKey   *ohai.KeyConfig
	acsConfig *acs.Config
	tokens    []*acs.Token
}

type ReceiverFetchMedia struct {
	URL       string
	MimeType  string
	Width     int
	Height    int
	ExpiresAt time.Time
}

type graphQLResponse[T any] struct {
	Data *T `json:"data"`
	types.ErrorResponse
}

func makeGraphQLQuery[T any](ctx context.Context, c *Client, name string, variables any) (*T, error) {
	_, respData, err := c.http.MakeGraphQLRequest(ctx, name, variables)
	if err != nil {
		return nil, err
	}
	return parseGraphQLResponse[T](respData)
}

func parseGraphQLResponse[T any](respData []byte) (*T, error) {
	var resp graphQLResponse[T]
	err := json.Unmarshal(bytes.TrimPrefix(respData, httpclient.AntiJSPrefix), &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	} else if len(resp.Errors) > 0 {
		return nil, resp.Errors[0]
	} else if resp.ErrorCode != 0 {
		return nil, &resp.ErrorResponse
	} else if resp.Data == nil {
		return nil, fmt.Errorf("response didn't contain data")
	}
	return resp.Data, nil
}

func jsonNumberToInt(num json.Number) int64 {
	val, err := strconv.ParseInt(num.String(), 10, 64)
	if err != nil {
		return 0
	}
	return val
}

type ohaiConfigsResponse struct {
	Configurations struct {
		Configs []struct {
			KeyID          json.Number `json:"key_id"`
			PublicKey      string      `json:"public_key"`
			KEMID          json.Number `json:"kem_id"`
			KDFID          json.Number `json:"kdf_id"`
			AEADID         json.Number `json:"aead_id"`
			ExpirationDate json.Number `json:"expiration_date"`
		} `json:"ohai_configs"`
	} `json:"xfb_ohai_configurations"`
}

func (c *Client) getOHAIKey(ctx context.Context) (*ohai.KeyConfig, error) {
	if key := c.receiverFetch.ohaiKey; key != nil && time.Now().Add(time.Minute).Before(key.ExpiresAt) {
		return key, nil
	}
	resp, err := makeGraphQLQuery[ohaiConfigsResponse](ctx, c, "OhaiWebClientMessengerConfigsQuery", struct{}{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ohai configs: %w", err)
	}
	for _, cfg := range resp.Configurations.Configs {
		expiresAt := time.Unix(jsonNumberToInt(cfg.ExpirationDate), 0)
		if time.Now().Add(time.Minute).After(expiresAt) {
			continue
		}
		publicKey, err := hex.DecodeString(cfg.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("invalid ohai public key: %w", err)
		}
		c.receiverFetch.ohaiKey = &ohai.KeyConfig{
			KeyID:     uint8(jsonNumberToInt(cfg.KeyID)),
			KEMID:     uint16(jsonNumberToInt(cfg.KEMID)),
			KDFID:     uint16(jsonNumberToInt(cfg.KDFID)),
			AEADID:    uint16(jsonNumberToInt(cfg.AEADID)),
			PublicKey: publicKey,
			ExpiresAt: expiresAt,
		}
		return c.receiverFetch.ohaiKey, nil
	}
	return nil, fmt.Errorf("no valid ohai configs in response")
}

type acsConfigResponse struct {
	Config struct {
		ID              string      `json:"id"`
		CipherSuite     json.Number `json:"cipher_suite"`
		PublicKey       string      `json:"public_key"`
		MaxEvals        json.Number `json:"max_evals"`
		RedemptionLimit json.Number `json:"redemption_limit"`
		ExpireTime      json.Number `json:"expire_time"`
		TokenTTL        json.Number `json:"token_ttl"`
	} `json:"acs_config"`
}

func (c *Client) getACSConfig(ctx context.Context) (*acs.Config, error) {
	if cfg := c.receiverFetch.acsConfig; cfg != nil && time.Now().Add(time.Minute).Before(cfg.ExpiresAt) {
		return cfg, nil
	}
	resp, err := makeGraphQLQuery[acsConfigResponse](ctx, c, "MessagingWebACSGraphQLServerProviderConfigQuery", map[string]any{
		"project_name": receiverFetchProjectName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch acs config: %w", err)
	}
	publicKey, err := acs.DecodeBase64(resp.Config.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid acs public key: %w", err)
	}
	cfg := &acs.Config{
		ID:              resp.Config.ID,
		CipherSuite:     int(jsonNumberToInt(resp.Config.CipherSuite)),
		PublicKey:       publicKey,
		MaxEvals:        int(jsonNumberToInt(resp.Config.MaxEvals)),
		RedemptionLimit: int(jsonNumberToInt(resp.Config.RedemptionLimit)),
		ExpiresAt:       time.Unix(jsonNumberToInt(resp.Config.ExpireTime), 0),
		TokenTTL:        time.Duration(jsonNumberToInt(resp.Config.TokenTTL)) * time.Second,
	}
	if cfg.CipherSuite != acs.CipherSuiteRistrettoHMACSHA512 {
		return nil, fmt.Errorf("unsupported acs cipher suite %d", cfg.CipherSuite)
	} else if cfg.MaxEvals <= 0 || cfg.RedemptionLimit <= 0 {
		return nil, fmt.Errorf("invalid acs limits (max evals %d, redemption limit %d)", cfg.MaxEvals, cfg.RedemptionLimit)
	}
	c.receiverFetch.acsConfig = cfg
	return cfg, nil
}

type acsIssueResponse struct {
	Issue struct {
		Evaluation []struct {
			Data string `json:"data"`
		} `json:"evaluation"`
	} `json:"xfb_messaging_acs_issue"`
}

func (c *Client) issueACSTokens(ctx context.Context, cfg *acs.Config) ([]*acs.Token, error) {
	batchSize := min(cfg.MaxEvals, 5)
	blinded := make([]*acs.BlindedToken, batchSize)
	issueElements := make([]map[string]string, batchSize)
	for i := range blinded {
		var err error
		blinded[i], err = acs.NewBlindedToken()
		if err != nil {
			return nil, err
		}
		issueElements[i] = map[string]string{"issue_data": acs.EncodeBase64(blinded[i].Blinded)}
	}
	resp, err := makeGraphQLQuery[acsIssueResponse](ctx, c, "MessagingWebACSGraphQLServerProviderTokenIssuanceQuery", map[string]any{
		"projectName": receiverFetchProjectName,
		"configId":    cfg.ID,
		"message": map[string]any{
			"issue_element": issueElements,
			"request_proof": false,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to issue acs tokens: %w", err)
	} else if len(resp.Issue.Evaluation) != len(blinded) {
		return nil, fmt.Errorf("got %d acs evaluations for %d blinded tokens", len(resp.Issue.Evaluation), len(blinded))
	}
	tokens := make([]*acs.Token, len(blinded))
	for i, eval := range resp.Issue.Evaluation {
		var data struct {
			EvaluationData string `json:"evaluation_data"`
		}
		if err = json.Unmarshal([]byte(eval.Data), &data); err != nil {
			return nil, fmt.Errorf("failed to parse acs evaluation: %w", err)
		}
		evaluated, err := acs.DecodeBase64(data.EvaluationData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode acs evaluation: %w", err)
		}
		tokens[i], err = blinded[i].Unblind(cfg, evaluated)
		if err != nil {
			return nil, err
		}
	}
	return tokens, nil
}

func (c *Client) redeemACSToken(ctx context.Context) (*acs.Token, error) {
	for len(c.receiverFetch.tokens) > 0 && !c.receiverFetch.tokens[0].Usable() {
		c.receiverFetch.tokens = c.receiverFetch.tokens[1:]
	}
	if len(c.receiverFetch.tokens) == 0 {
		cfg, err := c.getACSConfig(ctx)
		if err != nil {
			return nil, err
		}
		c.receiverFetch.tokens, err = c.issueACSTokens(ctx, cfg)
		if err != nil {
			return nil, err
		}
	}
	token := c.receiverFetch.tokens[0]
	token.RemainingUses--
	return token, nil
}

type receiverFetchResponse struct {
	Media *struct {
		ReceiverFetchID     string      `json:"receiver_fetch_id"`
		CDNURL              string      `json:"cdn_url"`
		ExpirationTimestamp json.Number `json:"expiration_timestamp_ms"`
		MimeType            string      `json:"mime_type"`
		Width               json.Number `json:"width"`
		Height              json.Number `json:"height"`
	} `json:"media_receiver_fetch_deidentified"`
}

type receiverFetchRetryError struct {
	error
}

func (err receiverFetchRetryError) Unwrap() error {
	return err.error
}

func (c *Client) FetchReceiverFetchMedia(ctx context.Context, receiverFetchID string) (*ReceiverFetchMedia, error) {
	c.receiverFetch.lock.Lock()
	defer c.receiverFetch.lock.Unlock()
	for attempt := 0; ; attempt++ {
		media, err := c.fetchReceiverFetchMedia(ctx, receiverFetchID)
		if _, retryable := errors.AsType[receiverFetchRetryError](err); !retryable || attempt == 2 || ctx.Err() != nil {
			return media, err
		}
		select {
		case <-time.After(time.Duration(attempt+1) * 3 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *Client) fetchReceiverFetchMedia(ctx context.Context, receiverFetchID string) (*ReceiverFetchMedia, error) {
	key, err := c.getOHAIKey(ctx)
	if err != nil {
		return nil, err
	}
	token, err := c.redeemACSToken(ctx)
	if err != nil {
		return nil, err
	}
	doc := graphql.GraphQLDocs[receiverFetchQuery]
	variables, _ := json.Marshal(map[string]string{"receiverFetchId": receiverFetchID})
	body := url.Values{
		"doc_id":                   {doc.DocID},
		"fb_api_caller_class":      {doc.CallerClass},
		"fb_api_req_friendly_name": {doc.FriendlyName},
		"variables":                {string(variables)},
	}.Encode()
	innerHeaders := http.Header{}
	for name, value := range token.Headers(receiverFetchProjectName, nil) {
		innerHeaders[name] = []string{value}
	}
	innerHeaders.Set("Content-Type", "application/x-www-form-urlencoded")
	innerHeaders.Set("X-FB-Friendly-Name", doc.FriendlyName)
	target, _ := url.Parse(deidentifiedGraphQLURL)
	encapsulated, respCtx, err := key.Encapsulate(ohai.EncodeRequest(http.MethodPost, target, innerHeaders, []byte(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to encapsulate request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ohaiRelayURL, bytes.NewReader(encapsulated))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "message/ohttp-req")
	req.Header.Set("User-Agent", useragent.UserAgent)
	req.Header.Set("Origin", c.GetEndpoint("base_url"))
	resp, err := c.http.HTTP.Do(req)
	if err != nil {
		return nil, receiverFetchRetryError{fmt.Errorf("failed to send ohttp request: %w", err)}
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, receiverFetchRetryError{fmt.Errorf("failed to read ohttp response: %w", err)}
	} else if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("ohttp relay returned HTTP %d: %s", resp.StatusCode, respData)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			err = receiverFetchRetryError{err}
		}
		return nil, err
	}
	decapsulated, err := respCtx.Decapsulate(respData)
	if err != nil {
		return nil, fmt.Errorf("failed to decapsulate response: %w", err)
	}
	status, _, innerBody, err := ohai.DecodeResponse(decapsulated)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	} else if status != http.StatusOK {
		err = fmt.Errorf("receiver fetch returned HTTP %d: %s", status, innerBody)
		if status == http.StatusTooManyRequests || status >= 500 {
			err = receiverFetchRetryError{err}
		}
		return nil, err
	}
	parsed, err := parseGraphQLResponse[receiverFetchResponse](innerBody)
	if err != nil {
		return nil, err
	} else if parsed.Media == nil || parsed.Media.CDNURL == "" {
		return nil, errors.New("receiver fetch response didn't contain media")
	}
	return &ReceiverFetchMedia{
		URL:       parsed.Media.CDNURL,
		MimeType:  parsed.Media.MimeType,
		Width:     int(jsonNumberToInt(parsed.Media.Width)),
		Height:    int(jsonNumberToInt(parsed.Media.Height)),
		ExpiresAt: time.UnixMilli(jsonNumberToInt(parsed.Media.ExpirationTimestamp)),
	}, nil
}
