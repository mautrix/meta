package messagix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"

	"github.com/google/go-querystring/query"
	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/messagix/methods"
	"go.mau.fi/mautrix-meta/messagix/types"
)

type MercuryUploadMedia struct {
	Filename  string
	MimeType  string
	MediaData []byte

	IsVoiceClip  bool
	WaveformData *WaveformData
}

type WaveformData struct {
	Amplitudes        []float64 `json:"amplitudes"`
	SamplingFrequency int       `json:"sampling_frequency"`
}

func (c *Client) SendMercuryUploadRequest(ctx context.Context, media *MercuryUploadMedia) (*types.MercuryUploadResponse, error) {
	urlQueries := c.NewHttpQuery()
	queryValues, err := query.Values(urlQueries)
	if err != nil {
		return nil, fmt.Errorf("failed to convert HttpQuery into query.Values for mercury upload: %v", err)
	}

	payloadQuery := queryValues.Encode()
	url := c.getEndpoint("media_upload") + payloadQuery
	payload, contentType, err := c.NewMercuryMediaPayload(media)
	if err != nil {
		return nil, err
	}
	h := c.buildHeaders(true)
	h.Set("accept", "*/*")
	h.Set("content-type", contentType)
	h.Set("origin", c.getEndpoint("base_url"))
	h.Set("referer", c.getEndpoint("messages"))
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-site", "same-origin") // header is required

	_, respBody, err := c.MakeRequest(url, "POST", h, payload, types.NONE)
	if err != nil {
		return nil, fmt.Errorf("failed to send MercuryUploadRequest: %v", err)
	}

	resp, err := c.parseMercuryResponse(ctx, respBody)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

var antiJSPrefix = []byte("for (;;);")

func (c *Client) parseMercuryResponse(ctx context.Context, respBody []byte) (*types.MercuryUploadResponse, error) {
	jsonData := bytes.TrimPrefix(respBody, antiJSPrefix)

	logEvt := zerolog.Ctx(ctx).Trace()
	if json.Valid(jsonData) {
		logEvt.RawJSON("response_body", jsonData).Msg("Mercury upload response")
	} else {
		logEvt.Bytes("response_body", respBody).Msg("Mercury upload response (invalid JSON)")
	}

	var mercuryResponse *types.MercuryUploadResponse
	if err := json.Unmarshal(jsonData, &mercuryResponse); err != nil {
		return nil, fmt.Errorf("failed to parse mercury response: %v", err)
	} else if mercuryResponse.ErrorCode != 0 {
		return nil, fmt.Errorf("error in mercury upload: %w", &mercuryResponse.ErrorResponse)
	}
	mercuryResponse.Raw = jsonData

	err := c.parseMetadata(mercuryResponse)
	if err != nil {
		zerolog.Ctx(ctx).Debug().RawJSON("response_body", jsonData).Msg("Mercury upload response with unrecognized data")
		return nil, err
	}

	return mercuryResponse, nil
}

func (c *Client) parseMetadata(response *types.MercuryUploadResponse) error {
	if len(response.Payload.Metadata) == 0 {
		return fmt.Errorf("no metadata in upload response")
	}

	switch response.Payload.Metadata[0] {
	case '[':
		var realMetadata []types.ImageMetadata
		err := json.Unmarshal(response.Payload.Metadata, &realMetadata)
		if err != nil {
			return fmt.Errorf("failed to unmarshal image metadata in upload response: %v", err)
		}
		response.Payload.RealMetadata = &realMetadata[0]
	case '{':
		var realMetadata map[string]types.VideoMetadata
		err := json.Unmarshal(response.Payload.Metadata, &realMetadata)
		if err != nil {
			return fmt.Errorf("failed to unmarshal video metadata in upload response: %v", err)
		}
		realMetaEntry := realMetadata["0"]
		response.Payload.RealMetadata = &realMetaEntry
	default:
		return fmt.Errorf("unexpected metadata in upload response")
	}

	return nil
}

// returns payloadBytes, multipart content-type header
func (c *Client) NewMercuryMediaPayload(media *MercuryUploadMedia) ([]byte, string, error) {
	var mercuryPayload bytes.Buffer
	writer := multipart.NewWriter(&mercuryPayload)

	err := writer.SetBoundary("----WebKitFormBoundary" + methods.RandStr(16))
	if err != nil {
		return nil, "", fmt.Errorf("messagix-mercury: Failed to set boundary (%v)", err)
	}

	if media.IsVoiceClip {
		err = writer.WriteField("voice_clip", "true")
		if err != nil {
			return nil, "", fmt.Errorf("messagix-mercury: Failed to write voice_clip field (%v)", err)
		}

		if media.WaveformData != nil {
			waveformBytes, err := json.Marshal(media.WaveformData)
			if err != nil {
				return nil, "", fmt.Errorf("messagix-mercury: Failed to marshal waveform (%v)", err)
			}

			err = writer.WriteField("voice_clip_waveform_data", string(waveformBytes))
			if err != nil {
				return nil, "", fmt.Errorf("messagix-mercury: Failed to write waveform field (%v)", err)
			}
		}
	}

	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="farr"; filename="%s"`, media.Filename))
	partHeader.Set("Content-Type", media.MimeType)

	mediaPart, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, "", fmt.Errorf("messagix-mercury: Failed to create multipart writer (%v)", err)
	}

	_, err = mediaPart.Write(media.MediaData)
	if err != nil {
		return nil, "", fmt.Errorf("messagix-mercury: Failed to write data to multipart section (%v)", err)
	}

	err = writer.Close()
	if err != nil {
		return nil, "", fmt.Errorf("messagix-mercury: Failed to close multipart writer (%v)", err)
	}

	return mercuryPayload.Bytes(), writer.FormDataContentType(), nil
}
