package securehistory

import (
	"context"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"errors"

	"github.com/google/uuid"
	"github.com/gtank/ristretto255"
	"go.mau.fi/libsignal/ecc"
	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow/util/keys"
)

type Enrollment struct {
	Device *Device
	Info   *DeviceInfo
}

type addDeviceResponse struct {
	DeviceID  string `json:"device_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"is_retryable"`
}

type EnrollmentError struct {
	Code string
}

func (e *EnrollmentError) Error() string {
	return "Messenger backup enrollment rejected: " + e.Code
}

type DeviceInfo struct {
	BackupID                string `json:"backup_id"`
	BaseEpochID             string `json:"base_epoch_id"`
	DeviceEpochHMAC         []byte `json:"device_epoch_hmac"`
	DevicePublicKey         []byte `json:"device_public_key"`
	DeviceRegistrationID    string `json:"device_registration_id"`
	EpochStoragePublicKey   []byte `json:"epoch_storage_pubkey"`
	EpochStorageSignature   []byte `json:"epoch_storage_pubkey_sig"`
	EpochAuthPublicKey      []byte `json:"epoch_auth_pubkey"`
	EpochAuthSignature      []byte `json:"epoch_auth_pubkey_sig"`
	EpochRootKeyFingerprint []byte `json:"epoch_root_key_fingerprint"`
	OCMFRotationToken       []byte `json:"ocmf_rotation_token"`
	VirtualDeviceID         []byte `json:"virtual_device_id"`
	ArmadilloPublicKey      []byte `json:"armadillo_public_key"`
	ArmadilloKeySignature   []byte `json:"armadillo_key_signature"`
	LabyrinthKeySignature   []byte `json:"labyrinth_key_signature"`
	OccamOnlyEligible       bool   `json:"occam_only_eligible"`
	TraceID                 string `json:"trace_id"`
}

func (d *VirtualDevice) PrepareDevice(backupID, registrationID string, armadillo *keys.KeyPair, epochs map[string]Epoch) (*Enrollment, error) {
	if d.Info == nil || d.Info.Secrets == nil || backupID == "" || registrationID == "" || armadillo == nil || armadillo.Priv == nil || armadillo.Pub == nil {
		return nil, errors.New("missing Messenger backup or connected device identity")
	}
	epoch, ok := epochs[d.Info.LastEpochID]
	if !ok || !validEpochAnonID(epoch.AnonID) || len(epoch.RootKey) != 32 {
		return nil, errors.New("missing latest Messenger backup epoch")
	}
	state := &Device{BackupID: backupID, Epochs: epochs}
	var err error
	state.MailboxRootKey, err = d.decryptSecret(d.Info.Secrets.MailboxRootKey, "mailbox_root_key")
	if err != nil {
		return nil, err
	}
	ocmf, err := d.decryptSecret(d.Info.Secrets.OCMFClientState, "ocmf_client_state")
	if err != nil {
		return nil, err
	}
	var token []byte
	state.OCMFClientState, token, err = evolveOCMF(ocmf)
	if err != nil {
		return nil, err
	}
	deviceKey := keys.NewKeyPair()
	storageKey := keys.NewKeyPair()
	authKey := keys.NewKeyPair()
	publicKey := append([]byte{255, 1}, deviceKey.Pub[:]...)
	state.StoragePublicKey = storageKey.Pub[:]
	state.StoragePrivateKey = storageKey.Priv[:]
	deviceEpochKey, err := deriveEpochKey(epoch, "epoch_devices", 32)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, deviceEpochKey)
	mac.Write(publicKey)
	fingerprint, err := hkdf.Key(sha256.New, epoch.RootKey, nil, "meb/debug/fingerprint/MEBEpochRootKey", 18)
	if err != nil {
		return nil, err
	}
	labyrinthSignature := ecc.CalculateSignature(ecc.NewDjbECPrivateKey(*armadillo.Priv), publicKey)
	return &Enrollment{Device: state, Info: &DeviceInfo{
		BackupID:                backupID,
		BaseEpochID:             epoch.ID,
		DeviceEpochHMAC:         mac.Sum(nil),
		DevicePublicKey:         publicKey,
		DeviceRegistrationID:    registrationID,
		EpochStoragePublicKey:   state.StoragePublicKey,
		EpochStorageSignature:   signDeviceKey(deviceKey, '0', state.StoragePublicKey),
		EpochAuthPublicKey:      authKey.Pub[:],
		EpochAuthSignature:      signDeviceKey(deviceKey, '1', authKey.Pub[:]),
		EpochRootKeyFingerprint: fingerprint,
		OCMFRotationToken:       token,
		VirtualDeviceID:         d.ID,
		ArmadilloPublicKey:      armadillo.Pub[:],
		ArmadilloKeySignature:   signDeviceKey(deviceKey, '3', armadillo.Pub[:]),
		LabyrinthKeySignature:   labyrinthSignature[:],
		OccamOnlyEligible:       true,
		TraceID:                 uuid.NewString(),
	}}, nil
}

func signDeviceKey(key *keys.KeyPair, domain byte, message []byte) []byte {
	signature := ecc.CalculateSignature(ecc.NewDjbECPrivateKey(*key.Priv), append([]byte{domain}, message...))
	return append([]byte{255, 3}, signature[:]...)
}

func evolveOCMF(state []byte) ([]byte, []byte, error) {
	current, err := ristretto255.NewScalar().SetCanonicalBytes(state)
	if err != nil || current.Equal(ristretto255.NewScalar()) == 1 {
		return nil, nil, errors.New("invalid Messenger backup OCMF state")
	}
	factor := ristretto255.NewScalar()
	for factor.Equal(ristretto255.NewScalar()) == 1 {
		_, err = factor.SetUniformBytes(random.Bytes(64))
		if err != nil {
			return nil, nil, err
		}
	}
	current.Multiply(current, factor)
	factor.Invert(factor)
	return current.Bytes(), factor.Bytes(), nil
}

func (c *Client) AddDevice(ctx context.Context, enrollment *Enrollment) (*Device, error) {
	if enrollment == nil || enrollment.Device == nil || enrollment.Info == nil || enrollment.Device.ID != "" {
		return nil, errors.New("invalid Messenger backup enrollment")
	}
	response, err := request[addDeviceResponse](ctx, c.HTTP, "EBAddDeviceMutation", "xfb_eb_add_device", map[string]any{
		"input": map[string]any{
			"device_info":      enrollment.Info,
			"family_device_id": "",
			"ls_device_id":     "",
			"request_uuid":     enrollment.Info.TraceID,
		},
	})
	if err != nil {
		return nil, err
	} else if response == nil {
		return nil, errors.New("backup enrollment returned no result")
	} else if response.Code != "" {
		return nil, &EnrollmentError{Code: response.Code}
	} else if response.DeviceID == "" {
		return nil, errors.New("backup enrollment returned no device ID")
	}
	device := *enrollment.Device
	device.ID = response.DeviceID
	return &device, nil
}
