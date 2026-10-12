package receiverfetch

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/gtank/ristretto255"
	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/random"
)

type Nonce struct {
	nonce []byte
	blind *ristretto255.Scalar
}

func BlindNonce() (*Nonce, string) {
	nonce := random.Bytes(32)
	seed := random.Bytes(64)
	blind := exerrors.Must(ristretto255.NewScalar().SetUniformBytes(seed))
	hash := sha512.Sum512(nonce)
	point := exerrors.Must(ristretto255.NewIdentityElement().SetUniformBytes(hash[:]))
	blinded := ristretto255.NewIdentityElement().ScalarMult(blind, point)
	return &Nonce{nonce, blind}, base64.RawURLEncoding.EncodeToString(blinded.Bytes())
}

func (n *Nonce) Redeem(evaluation string) (credential, tag string, err error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(evaluation, "="))
	if err != nil {
		return "", "", fmt.Errorf("invalid acs evaluation: %w", err)
	}
	point, err := ristretto255.NewIdentityElement().SetCanonicalBytes(data)
	if err != nil {
		return "", "", fmt.Errorf("invalid evaluated element: %w", err)
	}
	unblinded := ristretto255.NewIdentityElement().ScalarMult(ristretto255.NewScalar().Invert(n.blind), point)
	hash := sha512.New()
	hash.Write(n.nonce)
	hash.Write(unblinded.Bytes())
	mac := hmac.New(sha512.New, hash.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(n.nonce), base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
