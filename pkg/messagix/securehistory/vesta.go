package securehistory

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"go.mau.fi/util/jsonbytes"
	"google.golang.org/protobuf/proto"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory/opaque"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

var ErrNoPINBackup = errors.New("no Messenger PIN backup found")

type Client struct {
	HTTP           *httpclient.HTTPClient
	RegistrationID string
	AppID          string
}

type Backup struct {
	ID          string `json:"id"`
	DelegatorID string `json:"encryption_management_delegator_id"`
	Devices     []struct {
		Type     string `json:"device_type"`
		ClientID string `json:"client_generated_id"`
	} `json:"virtual_devices"`
}

func (b *Backup) vestaID() string {
	var clientID string
	for _, device := range b.Devices {
		if device.Type == "HSM" {
			clientID = device.ClientID
		}
	}
	return clientID
}

type Recovery struct {
	Code                     []byte
	TriggerPostLoginRegister bool
}

type RecoveryError struct {
	Code              string
	AttemptsRemaining *int
}

func (e *RecoveryError) Error() string {
	if e.AttemptsRemaining == nil {
		return "Messenger recovery failed: " + e.Code
	}
	return fmt.Sprintf("Messenger recovery failed: %s (%d attempts remaining)", e.Code, *e.AttemptsRemaining)
}

type vestaInit struct {
	RSAKey            jsonbytes.UnpaddedURLBytes `json:"base64url_encoded_island_rsa_pub"`
	RSASignature      jsonbytes.UnpaddedURLBytes `json:"base64url_encoded_island_rsa_pub_signature"`
	Ed25519Key        jsonbytes.UnpaddedURLBytes `json:"base64url_encoded_island_ed25519_pub"`
	Ed25519Signature  jsonbytes.UnpaddedURLBytes `json:"base64url_encoded_island_ed25519_pub_signature"`
	Registered        bool                       `json:"is_registered"`
	AttemptsRemaining *int                       `json:"attempts_remaining"`
}

type vestaBegin struct {
	L2        jsonbytes.UnpaddedURLBytes `json:"base64url_encoded_opaque_l2"`
	Signature jsonbytes.UnpaddedURLBytes `json:"base64url_encoded_opaque_l2_sig"`
}

type vestaFinish struct {
	Type                     string                     `json:"__typename"`
	Payload                  jsonbytes.UnpaddedURLBytes `json:"base64url_encoded_login_payload"`
	Success                  bool                       `json:"login_success"`
	ErrorCode                string                     `json:"vesta_error_code"`
	AttemptsRemaining        *int                       `json:"attempts_remaining"`
	TriggerPostLoginRegister bool                       `json:"trigger_post_login_register"`
}

func (c *Client) GetBackup(ctx context.Context) (*Backup, error) {
	backup, err := request[Backup](ctx, c.HTTP, "useMWEncryptedBackupsFetchBackupIdsV2Query", "xfb_backup", struct{}{})
	if err != nil {
		return nil, err
	} else if backup == nil || backup.ID == "" || backup.vestaID() == "" {
		return nil, ErrNoPINBackup
	}
	return backup, nil
}

func (c *Client) RecoverPIN(ctx context.Context, backup *Backup, pin []byte) (*Recovery, error) {
	if backup == nil || backup.vestaID() == "" {
		return nil, ErrNoPINBackup
	}
	variables := map[string]any{
		"fbid":                     backup.vestaID(),
		"product_use_case":         "MSGR_ENC_BACKUP",
		"vesta_client_api_version": 6,
	}
	init, err := request[vestaInit](ctx, c.HTTP,
		"VestaServerGraphQLLoginProviderInitQuery", "fbid_based_auth_layer_vesta_login_init",
		map[string]any{"request": variables})
	if err != nil {
		return nil, err
	} else if init == nil || !init.Registered {
		return nil, ErrNoPINBackup
	} else if init.AttemptsRemaining != nil && *init.AttemptsRemaining == 0 {
		return nil, &RecoveryError{Code: "no attempts remaining", AttemptsRemaining: init.AttemptsRemaining}
	} else if !verifyHSMKey(init.RSAKey, init.RSASignature) ||
		!verifyHSMKey(init.Ed25519Key, init.Ed25519Signature) || len(init.Ed25519Key) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Messenger recovery server signature")
	}
	login, l1, err := opaque.Begin(pin)
	if err != nil {
		return nil, err
	}
	defer login.Close()
	variables["vesta_client_protocol_version"] = 1
	variables["base64url_encoded_opaque_l1"] = base64.RawURLEncoding.EncodeToString(l1)
	begin, err := request[vestaBegin](ctx, c.HTTP, "VestaServerGraphQLLoginProviderBeginLoginMutation", "fbid_based_auth_layer_vesta_begin_login", map[string]any{"input": variables})
	if err != nil {
		return nil, err
	} else if begin == nil || !ed25519.Verify(ed25519.PublicKey(init.Ed25519Key), append([]byte{2}, begin.L2...), begin.Signature) {
		return nil, errors.New("invalid Messenger recovery response signature")
	}
	result, err := login.Finish(begin.L2)
	if err != nil {
		return nil, err
	}
	delete(variables, "base64url_encoded_opaque_l1")
	variables["base64url_encoded_opaque_l3"] = base64.RawURLEncoding.EncodeToString(result.Message)
	finish, err := request[vestaFinish](ctx, c.HTTP, "VestaServerGraphQLLoginProviderFinishLoginMutation", "fbid_based_auth_layer_vesta_finish_login_v2", map[string]any{"input": variables})
	if err != nil {
		return nil, err
	} else if finish == nil {
		return nil, errors.New("missing Messenger recovery result")
	} else if finish.Type == "XFBVestaError" || !finish.Success {
		return nil, &RecoveryError{Code: finish.ErrorCode, AttemptsRemaining: finish.AttemptsRemaining}
	}
	code, err := unpackLoginPayload(finish.Payload, result.SessionKey, result.ExportKey)
	if err != nil {
		return nil, err
	}
	return &Recovery{Code: code, TriggerPostLoginRegister: finish.TriggerPostLoginRegister}, nil
}

func request[T any](ctx context.Context, client *httpclient.HTTPClient, name, field string, variables any) (*T, error) {
	response, body, err := client.MakeGraphQLRequest(ctx, name, variables)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", name, err)
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d", name, response.StatusCode)
	}
	var envelope struct {
		types.ErrorResponse
		Data map[string]json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("invalid %s response: %w", name, err)
	} else if err = envelope.AsError(); err != nil {
		return nil, fmt.Errorf("%s failed: %w", name, err)
	}
	data, ok := envelope.Data[field]
	if !ok {
		return nil, fmt.Errorf("%s response is missing %s", name, field)
	} else if bytes.Equal(data, []byte("null")) {
		return nil, nil
	}
	var result T
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid %s result: %w", name, err)
	}
	return &result, nil
}

func verifyHSMKey(key, signature []byte) bool {
	digest := sha256.Sum256(key)
	for _, encoded := range vestaTrustRoots {
		der, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		public, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			continue
		}
		root, ok := public.(*rsa.PublicKey)
		if ok && rsa.VerifyPKCS1v15(root, crypto.SHA256, digest[:], signature) == nil {
			return true
		}
	}
	return false
}

func unpackLoginPayload(payload, sessionKey, exportKey []byte) ([]byte, error) {
	var outer LoginPayload
	if err := proto.Unmarshal(payload, &outer); err != nil {
		return nil, fmt.Errorf("invalid recovery payload: %w", err)
	}
	plaintext, err := decryptGCM(sessionKey, outer.GetRkNonce(), outer.GetBackupKeyDataEncrypted())
	if err != nil {
		return nil, err
	}
	var inner WaBackupKeyData
	if err = proto.Unmarshal(plaintext, &inner); err != nil {
		return nil, fmt.Errorf("invalid recovery key data: %w", err)
	}
	return decryptGCM(exportKey, inner.GetKNonce(), inner.GetAesK())
}

func decryptGCM(key, nonce, ciphertext []byte) ([]byte, error) {
	if len(key) != 64 || len(nonce) != 12 {
		return nil, errors.New("invalid recovery payload key or nonce")
	}
	block, err := aes.NewCipher(key[:16])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, nil)
}
