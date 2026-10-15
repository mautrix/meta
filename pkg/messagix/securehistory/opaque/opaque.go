package opaque

import (
	"bytes"
	"crypto/hmac"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/bytemare/cryptotools/group/ciphersuite"
	"github.com/bytemare/cryptotools/hash"
	"github.com/bytemare/cryptotools/mhf"
	bytemare "github.com/bytemare/opaque"
	"github.com/bytemare/opaque/ake"
	"github.com/bytemare/opaque/core/envelope"
	"github.com/bytemare/opaque/message"
	"github.com/bytemare/voprf"
)

var ErrInvalidPIN = errors.New("incorrect recovery PIN")

type Login struct {
	client *bytemare.Client
}

type Result struct {
	Message    []byte
	SessionKey []byte
	ExportKey  []byte
}

func Begin(password []byte) (*Login, []byte, error) {
	if len(password) == 0 {
		return nil, nil, errors.New("empty recovery PIN")
	}
	hardener := mhf.PBKDF2Sha512.Get()
	hardener.Parameterize(100_000)
	login := &Login{
		client: bytemare.NewClient(voprf.RistrettoSha512, hash.SHA512, hash.SHA512, hash.SHA512,
			hardener, envelope.Base, ciphersuite.Ristretto255Sha512),
	}
	login.client.Ake.Initialize(nil, nil, 32)
	return login, login.client.AuthenticationStart(bytes.Clone(password), nil).Serialize(), nil
}

func (l *Login) Finish(response []byte) (*Result, error) {
	if l.client == nil {
		return nil, errors.New("invalid recovery login state")
	}
	defer l.Close()
	if len(response) < 323 || len(response)-323 > 255*64 || response[64] != byte(envelope.Base) || int(binary.BigEndian.Uint16(response[257:259])) != len(response)-323 {
		return nil, errors.New("invalid OPAQUE response")
	}
	ke2, err := message.DeserializeKE2(response, 32, 32, 64, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid OPAQUE response: %w", err)
	}
	c := l.client
	for _, encoded := range [][]byte{ke2.Data, ke2.Pks, ke2.EpkS} {
		point, err := c.Ake.Group.NewElement().Decode(encoded)
		if err != nil || point.IsIdentity() {
			return nil, errors.New("invalid OPAQUE response point")
		}
	}
	if hmac.Equal(ke2.Data, c.Ke1.Data) {
		return nil, errors.New("reflected OPAQUE evaluation")
	}
	output, err := c.Core.Oprf.Finalize(&voprf.Evaluation{Elements: [][]byte{ke2.Data}})
	if err != nil {
		return nil, fmt.Errorf("failed to finalize OPAQUE evaluation: %w", err)
	}
	hardened := c.Core.Harden(output, []byte("OPAQUE_PBKDF2_SALT"), 64)
	credentials, exportKey, err := c.Core.RecoverSecretNew(nil, nil, ke2.Pks, hardened, ke2.Envelope)
	if errors.Is(err, envelope.ErrEnvelopeInvalidTag) {
		return nil, ErrInvalidPIN
	} else if err != nil {
		return nil, fmt.Errorf("failed to recover OPAQUE credentials: %w", err)
	}
	secret, err := c.Ake.Group.NewScalar().Decode(credentials.Sku)
	if err != nil {
		return nil, fmt.Errorf("invalid OPAQUE client key: %w", err)
	}
	public := c.Ake.Group.Base().Mult(secret)
	ke3, _, err := c.Ake.Finalize(public.Bytes(), credentials.Sku, ke2.Pks, ke2.Pks, c.Ke1, ke2)
	if errors.Is(err, ake.ErrAkeInvalidServerMac) {
		return nil, ErrInvalidPIN
	} else if err != nil {
		return nil, fmt.Errorf("failed to finalize OPAQUE login: %w", err)
	}
	return &Result{
		Message:    ke3.Serialize(),
		SessionKey: c.SessionKey(),
		ExportKey:  exportKey,
	}, nil
}

func (l *Login) Close() {
	l.client = nil
}
