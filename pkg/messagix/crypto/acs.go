package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/gtank/ristretto255"
)

type ACSNonce struct {
	nonce []byte
	blind *ristretto255.Scalar
}

func BlindACSNonce() (*ACSNonce, string, error) {
	nonce := make([]byte, 32)
	seed := make([]byte, 64)
	if _, err := rand.Read(nonce); err != nil {
		return nil, "", err
	} else if _, err = rand.Read(seed); err != nil {
		return nil, "", err
	}
	blind, _ := ristretto255.NewScalar().SetUniformBytes(seed)
	hash := sha512.Sum512(nonce)
	point, _ := ristretto255.NewElement().SetUniformBytes(hash[:])
	blinded := ristretto255.NewElement().ScalarMult(blind, point)
	return &ACSNonce{nonce, blind}, base64.RawURLEncoding.EncodeToString(blinded.Bytes()), nil
}

func (n *ACSNonce) Redeem(evaluation string) (credential, tag string, err error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(evaluation, "="))
	if err != nil {
		return "", "", fmt.Errorf("invalid acs evaluation: %w", err)
	}
	point, err := ristretto255.NewElement().SetCanonicalBytes(data)
	if err != nil {
		return "", "", fmt.Errorf("invalid evaluated element: %w", err)
	}
	unblinded := ristretto255.NewElement().ScalarMult(ristretto255.NewScalar().Invert(n.blind), point)
	hash := sha512.New()
	hash.Write(n.nonce)
	hash.Write(unblinded.Bytes())
	mac := hmac.New(sha512.New, hash.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(n.nonce), base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
