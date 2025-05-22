package metaid

import (
	"bytes"
	"encoding/binary"
	"io"

	"maunium.net/go/mautrix/bridgev2/networkid"
)

type DirectMediaType byte

const (
	DirectMediaTypeMeta DirectMediaType = iota
	DirectMediaTypeWhatsApp
)

type MediaInfo struct {
	Type      DirectMediaType
	UserID    networkid.UserLoginID
	MessageID networkid.MessageID
}

func MakeMediaID(mediaType DirectMediaType, userID networkid.UserLoginID, messageID networkid.MessageID) networkid.MediaID {
	mediaID := []byte{byte(mediaType)}
	mediaID = binary.AppendVarint(mediaID, ParseUserLoginID(userID))
	mediaID = writeByteSlice(mediaID, []byte(messageID))
	return mediaID
}

func ParseMediaID(mediaID networkid.MediaID) (*MediaInfo, error) {
	buf := bytes.NewReader(mediaID)
	var mediaType DirectMediaType
	if err := binary.Read(buf, binary.BigEndian, &mediaType); err != nil {
		return nil, err
	}

	mediaInfo := &MediaInfo{Type: mediaType}
	uID, err := binary.ReadVarint(buf)
	if err != nil {
		return nil, err
	}
	mediaInfo.UserID = MakeUserLoginID(uID)

	if bs, err := readByteSlice(buf); err != nil {
		return nil, err
	} else {
		mediaInfo.MessageID = networkid.MessageID(string(bs))
	}

	return mediaInfo, nil
}

func writeByteSlice(buf []byte, data []byte) []byte {
	buf = binary.AppendUvarint(buf, uint64(len(data)))
	var err error
	buf, err = binary.Append(buf, binary.BigEndian, data)
	if err != nil {
		panic(err)
	}
	return buf
}

func readByteSlice(buf *bytes.Reader) ([]byte, error) {
	size, err := binary.ReadUvarint(buf)
	if err != nil {
		return nil, err
	}
	bs := make([]byte, size)
	_, err = io.ReadFull(buf, bs)
	if err != nil {
		return nil, err
	}
	return bs, nil
}
