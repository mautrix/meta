package types

import "github.com/google/uuid"

type NativeSession struct {
	AccessToken    string    `json:"access_token"`
	AppID          string    `json:"app_id"`
	DeviceID       uuid.UUID `json:"device_id"`
	FamilyDeviceID uuid.UUID `json:"family_device_id"`
}
