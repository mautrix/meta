package types

import (
	"encoding/json"
	"strconv"
)

type MercuryUploadResponse struct {
	Raw json.RawMessage `json:"-"`

	ErrorResponse
	Ar      int           `json:"__ar,omitempty"`
	Payload MediaPayloads `json:"payload,omitempty"`
	Hsrp    Hsrp          `json:"hsrp,omitempty"`
	Lid     string        `json:"lid,omitempty"`
}

type StringOrInt int64

const maxSafeIntInFloat64 = 1<<53 - 1
const minSafeIntInFloat64 = -maxSafeIntInFloat64

func (soi *StringOrInt) MarshalJSON() ([]byte, error) {
	if *soi > maxSafeIntInFloat64 || *soi < minSafeIntInFloat64 {
		return json.Marshal(strconv.FormatInt(int64(*soi), 10))
	}
	return json.Marshal(soi)
}

func (soi *StringOrInt) UnmarshalJSON(data []byte) (err error) {
	if data[0] == '"' {
		var str string
		err = json.Unmarshal(data, &str)
		if err != nil {
			return err
		}
		*(*int64)(soi), err = strconv.ParseInt(str, 10, 64)
	} else {
		err = json.Unmarshal(data, (*int64)(soi))
	}
	return
}

type FileMetadata struct {
	FileID       StringOrInt `json:"file_id,omitempty"`
	AudioID      StringOrInt `json:"audio_id,omitempty"`
	VideoID      StringOrInt `json:"video_id,omitempty"`
	ImageID      StringOrInt `json:"image_id,omitempty"`
	GifID        StringOrInt `json:"gif_id,omitempty"`
	Filename     string      `json:"filename,omitempty"`
	Filetype     string      `json:"filetype,omitempty"`
	Src          string      `json:"src,omitempty"`
	ThumbnailSrc string      `json:"thumbnail_src,omitempty"`
}

func (vid *FileMetadata) GetFbId() int64 {
	if vid.VideoID != 0 {
		return int64(vid.VideoID)
	} else if vid.AudioID != 0 {
		return int64(vid.AudioID)
	} else if vid.ImageID != 0 {
		return int64(vid.ImageID)
	} else if vid.GifID != 0 {
		return int64(vid.GifID)
	} else if vid.FileID != 0 {
		return int64(vid.FileID)
	}
	return 0
}

/*
Metadata returns a map (object) for a video like this:

	"metadata": {
		"0": {
			"video_id": 0,
			"filename": "",
			"filetype": "",
			"thumbnail_src": ""
	}

and for an image it returns a slice/array like this:

	"metadata": [{
		"image_id": ,
		"filename": "",
		"filetype": "",
		"src": "",
		"fbid": 0
	}]

So you will have to use type assertion to handle these cases seperately.
*/
type MediaPayloads struct {
	UploadID     any             `json:"uploadID,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	RealMetadata *FileMetadata   `json:"-"`
}

type Hblp struct {
	Consistency Consistency `json:"consistency,omitempty"`
}

type Hsrp struct {
	Hblp Hblp `json:"hblp,omitempty"`
}
