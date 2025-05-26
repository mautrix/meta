package metaid

import (
	"encoding/binary"
	"fmt"

	"maunium.net/go/mautrix/bridgev2/networkid"
)

type DirectMediaType byte

const (
	DirectMediaTypeMeta     DirectMediaType = 2
	DirectMediaTypeWhatsApp DirectMediaType = 3
)

type MediaInfo struct {
	Type      DirectMediaType
	UserID    networkid.UserLoginID
	MessageID networkid.MessageID
}

func MakeMediaID(mediaType DirectMediaType, userID networkid.UserLoginID, messageID networkid.MessageID) networkid.MediaID {
	mediaID := make([]byte, 1, 10+len(messageID))
	mediaID[0] = byte(mediaType)
	mediaID = binary.BigEndian.AppendUint64(mediaID, uint64(ParseUserLoginID(userID)))
	mediaID = append(mediaID, byte(len(messageID)))
	mediaID = append(mediaID, messageID...)
	return mediaID
}

func ParseMediaID(mediaID networkid.MediaID) (*MediaInfo, error) {
	if len(mediaID) < 10 {
		return nil, fmt.Errorf("media ID too short: %d bytes", len(mediaID))
	}
	mediaType := DirectMediaType(mediaID[0])
	switch mediaType {
	case DirectMediaTypeMeta, DirectMediaTypeWhatsApp:
	default:
		return nil, fmt.Errorf("unrecognized media type %d", mediaType)
	}
	messageIDLength := int(mediaID[9])
	if len(mediaID) != 10+messageIDLength {
		return nil, fmt.Errorf("unexpected media ID length, message ID byte says 10+%d, actual is %d", messageIDLength, len(mediaID))
	}

	return &MediaInfo{
		Type:      mediaType,
		UserID:    MakeUserLoginID(int64(binary.BigEndian.Uint64(mediaID[1:9]))),
		MessageID: networkid.MessageID(mediaID[10 : 10+messageIDLength]),
	}, nil
}
