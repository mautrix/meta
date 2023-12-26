package types

type MercuryUploadResponse struct {
	Ar      int     `json:"__ar,omitempty"`
	Error            int    `json:"error,omitempty"`
	ErrorSummary     string `json:"errorSummary,omitempty"`
	ErrorDescription string `json:"errorDescription,omitempty"`
	Payload MediaPayloads `json:"payload,omitempty"`
	Hsrp    Hsrp    `json:"hsrp,omitempty"`
	Lid     string  `json:"lid,omitempty"`
}

type MediaMetadata interface {
	GetFbId() int64
}

type ImageMetadata struct {
	ImageID  int64  `json:"image_id,omitempty"`
	Filename string `json:"filename,omitempty"`
	Filetype string `json:"filetype,omitempty"`
	Src      string `json:"src,omitempty"`
	Fbid     int64  `json:"fbid,omitempty"`
}

func (img *ImageMetadata) GetFbId() int64 {
	return img.Fbid
}

type VideoMetadata struct {
	VideoID      int64  `json:"video_id,omitempty"`
	Filename     string `json:"filename,omitempty"`
	Filetype     string `json:"filetype,omitempty"`
	ThumbnailSrc string `json:"thumbnail_src,omitempty"`
}

func (vid *VideoMetadata) GetFbId() int64 {
	return vid.VideoID
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
	UploadID any        `json:"uploadID,omitempty"`
	Metadata interface{} `json:"metadata,omitempty"`
}

type Hblp struct {
	Consistency Consistency `json:"consistency,omitempty"`
}

type Hsrp struct {
	Hblp Hblp `json:"hblp,omitempty"`
}