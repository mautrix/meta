package metaid

import (
	"encoding/binary"
	"fmt"

	"maunium.net/go/mautrix/bridgev2/networkid"
)

type DirectMediaType byte

const (
	// deprecated due to bug around handling multiple media
	// attachments in a single message
	DirectMediaTypeMetaV1     DirectMediaType = 2
	DirectMediaTypeWhatsAppV1 DirectMediaType = 3

	// recommended for current use
	DirectMediaTypeMetaV2     DirectMediaType = 4
	DirectMediaTypeWhatsAppV2 DirectMediaType = 5
)

func (t DirectMediaType) isSupported() bool {
	switch t {
	case
		DirectMediaTypeMetaV1, DirectMediaTypeWhatsAppV1,
		DirectMediaTypeMetaV2, DirectMediaTypeWhatsAppV2:
		return true
	}
	return false
}

func (t DirectMediaType) includesPartID() bool {
	switch t {
	case DirectMediaTypeMetaV1, DirectMediaTypeWhatsAppV1:
		return false
	}
	return true
}

type MediaInfo struct {
	Type      DirectMediaType
	UserID    networkid.UserLoginID
	MessageID networkid.MessageID
	PartID    networkid.PartID
}

func MakeMediaID(mediaType DirectMediaType, userID networkid.UserLoginID, messageID networkid.MessageID, partID networkid.PartID) networkid.MediaID {
	mediaID := make([]byte, 1, 11+len(messageID)+len(partID))
	mediaID[0] = byte(mediaType)
	mediaID = binary.BigEndian.AppendUint64(mediaID, uint64(ParseUserLoginID(userID)))
	mediaID = append(mediaID, byte(len(messageID)))
	mediaID = append(mediaID, messageID...)
	if mediaType.includesPartID() {
		mediaID = append(mediaID, byte(len(partID)))
		mediaID = append(mediaID, partID...)
	}
	return mediaID
}

func ParseMediaID(mediaID networkid.MediaID) (*MediaInfo, error) {
	var info MediaInfo

	ptr := 0
	read := func(size int, what string) ([]byte, error) {
		if len(mediaID) < ptr+size {
			return nil, fmt.Errorf("media ID too short (%d bytes) to read %d byte %s starting at byte %d", len(mediaID), size, what, ptr)
		}
		b := mediaID[ptr : ptr+size]
		ptr += size
		return b, nil
	}
	readOne := func(what string) (byte, error) {
		b, err := read(1, what)
		if err != nil {
			return 0, err
		}
		return b[0], nil
	}

	c, err := readOne("media type")
	if err != nil {
		return nil, err
	}
	info.Type = DirectMediaType(c)

	if !info.Type.isSupported() {
		return nil, fmt.Errorf("unrecognized media type %d", info.Type)
	}

	b, err := read(8, "user id")
	if err != nil {
		return nil, err
	}
	info.UserID = MakeUserLoginID(int64(binary.BigEndian.Uint64(b)))

	c, err = readOne("message id length")
	if err != nil {
		return nil, err
	}
	messageIDLength := int(c)

	b, err = read(messageIDLength, "message id")
	if err != nil {
		return nil, err
	}
	info.MessageID = networkid.MessageID(b)

	if info.Type.includesPartID() {
		c, err = readOne("part id length")
		if err != nil {
			return nil, err
		}
		partIDLength := int(c)

		b, err = read(partIDLength, "part id")
		if err != nil {
			return nil, err
		}

		info.PartID = networkid.PartID(b)
	}

	return &info, nil
}
