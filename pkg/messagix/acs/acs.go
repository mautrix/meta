package acs

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/gtank/ristretto255"
)

const CipherSuiteRistrettoHMACSHA512 = 1

type Config struct {
	ID              string
	CipherSuite     int
	PublicKey       []byte
	MaxEvals        int
	RedemptionLimit int
	ExpiresAt       time.Time
	TokenTTL        time.Duration
}

type BlindedToken struct {
	nonce   []byte
	blind   *ristretto255.Scalar
	Blinded []byte
}

type Token struct {
	ConfigID      string
	Nonce         []byte
	SharedSecret  []byte
	RemainingUses int
	ExpiresAt     time.Time
}

func hashToElement(input []byte) *ristretto255.Element {
	hash := sha512.Sum512(input)
	element, _ := ristretto255.NewElement().SetUniformBytes(hash[:])
	return element
}

func NewBlindedToken() (*BlindedToken, error) {
	nonce := make([]byte, 32)
	scalarSeed := make([]byte, 64)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	} else if _, err = rand.Read(scalarSeed); err != nil {
		return nil, err
	}
	blind, err := ristretto255.NewScalar().SetUniformBytes(scalarSeed)
	if err != nil {
		return nil, err
	}
	blinded := ristretto255.NewElement().ScalarMult(blind, hashToElement(nonce))
	return &BlindedToken{nonce: nonce, blind: blind, Blinded: blinded.Bytes()}, nil
}

func (bt *BlindedToken) Unblind(cfg *Config, evaluated []byte) (*Token, error) {
	element, err := ristretto255.NewElement().SetCanonicalBytes(evaluated)
	if err != nil {
		return nil, fmt.Errorf("invalid evaluated element: %w", err)
	}
	inv := ristretto255.NewScalar().Invert(bt.blind)
	unblinded := ristretto255.NewElement().ScalarMult(inv, element)
	hash := sha512.New()
	hash.Write(bt.nonce)
	hash.Write(unblinded.Bytes())
	expiry := cfg.ExpiresAt
	if cfg.TokenTTL > 0 {
		expiry = time.Now().Add(cfg.TokenTTL)
	}
	return &Token{
		ConfigID:      cfg.ID,
		Nonce:         bt.nonce,
		SharedSecret:  hash.Sum(nil),
		RemainingUses: cfg.RedemptionLimit,
		ExpiresAt:     expiry,
	}, nil
}

func (t *Token) Usable() bool {
	return t.RemainingUses > 0 && time.Now().Add(time.Minute).Before(t.ExpiresAt)
}

func (t *Token) Headers(projectName string, data []byte) map[string]string {
	mac := hmac.New(sha512.New, t.SharedSecret)
	mac.Write(data)
	return map[string]string{
		"X-ACS-ATTRIBUTE-IDENTIFIER": "",
		"X-ACS-CONFIG-ID":            t.ConfigID,
		"X-ACS-CREDENTIAL":           EncodeBase64(t.Nonce),
		"X-ACS-PROJECT-NAME":         projectName,
		"X-ACS-TAG":                  EncodeBase64(mac.Sum(nil)),
	}
}

func EncodeBase64(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func DecodeBase64(data string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(data, "="))
}
