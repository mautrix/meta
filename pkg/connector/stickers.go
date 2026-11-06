package connector

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

const seenStickerPackURL = "facebook://seen-stickers"
const stickerPackURLPrefix = "facebook://sticker-pack/"
const maxStickerSize = 10 * 1024 * 1024
const stickerDisplayBox = 96
const stickerCacheTime = 5 * time.Minute
const stickerErrorCacheTime = 30 * time.Second

var _ bridgev2.StickerImportingNetworkAPI = (*MetaClient)(nil)

type cachedStickerPack struct {
	pack      *bridgev2.ImportedImagePack
	fetchedAt time.Time
}

func seenStickerPackMetadata() event.ImagePackMetadata {
	return event.ImagePackMetadata{
		DisplayName: "Seen Facebook stickers",
		Usage:       []event.ImagePackUsage{event.ImagePackUsageSticker},
		BridgedPack: &event.BridgedStickerPack{Network: "facebook", URL: seenStickerPackURL},
	}
}

func stickerPackMetadata(pack messagix.StickerPack) event.ImagePackMetadata {
	return event.ImagePackMetadata{
		DisplayName: pack.Name,
		Usage:       []event.ImagePackUsage{event.ImagePackUsageSticker},
		BridgedPack: &event.BridgedStickerPack{Network: "facebook", URL: stickerPackURLPrefix + pack.ID},
	}
}

func (m *MetaClient) getStickerPacks(ctx context.Context) ([]messagix.StickerPack, error) {
	if m.Client == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	m.stickerLock.Lock()
	defer m.stickerLock.Unlock()
	cacheTime := stickerCacheTime
	if m.stickerTrayErr != nil {
		cacheTime = stickerErrorCacheTime
	}
	if time.Since(m.stickerTrayFetchedAt) < cacheTime {
		return m.stickerTray, m.stickerTrayErr
	}
	packs, err := m.Client.FetchStickerTray(ctx)
	if err != nil && ctx.Err() != nil {
		return nil, err
	} else if err != nil {
		err = fmt.Errorf("failed to get sticker tray: %w", err)
	}
	m.stickerTray, m.stickerTrayErr, m.stickerTrayFetchedAt = packs, err, time.Now()
	return packs, err
}

func stickerDisplaySize(width, height int) (int, int) {
	if width <= 0 || height <= 0 {
		return stickerDisplayBox, stickerDisplayBox
	} else if width >= height {
		return stickerDisplayBox, max(1, height*stickerDisplayBox/width)
	}
	return max(1, width*stickerDisplayBox/height), stickerDisplayBox
}

func (m *MetaClient) ListImagePacks(ctx context.Context) ([]*event.ImagePackMetadata, error) {
	if !m.LoginMeta.Platform.IsMessenger() {
		return nil, nil
	}
	stickers, err := m.Main.DB.GetSeenStickers(ctx, m.UserLogin.ID)
	if err != nil {
		return nil, err
	}
	packs, err := m.getStickerPacks(ctx)
	if err != nil && len(stickers) == 0 {
		return nil, err
	} else if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to list Facebook sticker packs")
	}
	metadata := make([]*event.ImagePackMetadata, 0, len(packs)+1)
	for _, pack := range packs {
		meta := stickerPackMetadata(pack)
		metadata = append(metadata, &meta)
	}
	if len(stickers) > 0 {
		meta := seenStickerPackMetadata()
		metadata = append(metadata, &meta)
	}
	return metadata, nil
}

func (m *MetaClient) DownloadImagePack(ctx context.Context, url string) (*bridgev2.ImportedImagePack, error) {
	if !m.LoginMeta.Platform.IsMessenger() {
		return nil, mautrix.MNotFound.WithMessage("Unsupported sticker pack")
	} else if url == seenStickerPackURL {
		return m.downloadSeenStickerPack(ctx)
	}
	packID, ok := strings.CutPrefix(url, stickerPackURLPrefix)
	if !ok || packID == "" {
		return nil, mautrix.MNotFound.WithMessage("Unsupported sticker pack")
	}
	packs, err := m.getStickerPacks(ctx)
	if err != nil {
		return nil, err
	}
	for _, pack := range packs {
		if pack.ID == packID {
			return m.downloadStickerPack(ctx, pack)
		}
	}
	return nil, mautrix.MNotFound.WithMessage("Sticker pack is not available on this account")
}

func (m *MetaClient) uploadPackSticker(ctx context.Context, url, name string) (id.ContentURIString, string, int, error) {
	_, reader, err := mediadl.DownloadMedia(ctx, "image/*", url, maxStickerSize)
	if err != nil {
		return "", "", 0, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxStickerSize+1))
	reader.Close()
	if err != nil {
		return "", "", 0, err
	} else if len(data) > maxStickerSize {
		return "", "", 0, mediadl.ErrTooLargeFile
	}
	mimeType := mimetype.Detect(data).String()
	if !strings.HasPrefix(mimeType, "image/") {
		return "", "", 0, fmt.Errorf("unsupported sticker media type %s", mimeType)
	}
	mxc, _, err := m.Main.Bridge.Bot.UploadMedia(ctx, "", data, name, mimeType)
	return mxc, mimeType, len(data), err
}

func (m *MetaClient) downloadStickerPack(ctx context.Context, pack messagix.StickerPack) (*bridgev2.ImportedImagePack, error) {
	// Previews and imports both download the pack, so reuse recent results instead of refetching and reuploading.
	m.stickerPackLock.Lock(pack.ID)
	defer m.stickerPackLock.Unlock(pack.ID)
	m.stickerLock.Lock()
	cached, ok := m.stickerPacks[pack.ID]
	if ok && time.Since(cached.fetchedAt) < stickerCacheTime {
		m.stickerLock.Unlock()
		return cached.pack, nil
	}
	stickers, err := m.Client.FetchStickerPack(ctx, pack.ID)
	m.stickerLock.Unlock()
	if err != nil {
		return nil, fmt.Errorf("failed to get sticker pack: %w", err)
	}
	metadata := stickerPackMetadata(pack)
	imported := &bridgev2.ImportedImagePack{
		Shortcode: "facebook_" + pack.ID,
		Content:   &event.ImagePackEventContent{Metadata: metadata, Images: make(map[string]*event.ImagePackImage)},
	}
	for _, sticker := range stickers {
		mxc, mimeType, size, err := m.uploadPackSticker(ctx, sticker.URL, sticker.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to import sticker %s: %w", sticker.ID, err)
		}
		width, height := stickerDisplaySize(sticker.Width, sticker.Height)
		imported.Content.Images[sticker.ID] = &event.ImagePackImage{
			URL: mxc, Body: sticker.Label,
			Info: &event.FileInfo{MimeType: mimeType, Size: size, Width: width, Height: height, BridgedSticker: &event.BridgedSticker{
				Network: "facebook", ID: sticker.ID, PackURL: metadata.BridgedPack.URL,
			}},
		}
	}
	if len(imported.Content.Images) == 0 {
		return nil, mautrix.MNotFound.WithMessage("Sticker pack is empty")
	}
	m.stickerLock.Lock()
	if m.stickerPacks == nil {
		m.stickerPacks = make(map[string]cachedStickerPack)
	}
	m.stickerPacks[pack.ID] = cachedStickerPack{pack: imported, fetchedAt: time.Now()}
	m.stickerLock.Unlock()
	return imported, nil
}

func (m *MetaClient) downloadSeenStickerPack(ctx context.Context) (*bridgev2.ImportedImagePack, error) {
	stickers, err := m.Main.DB.GetSeenStickers(ctx, m.UserLogin.ID)
	if err != nil {
		return nil, err
	}
	pack := &bridgev2.ImportedImagePack{Content: &event.ImagePackEventContent{
		Metadata: seenStickerPackMetadata(), Images: make(map[string]*event.ImagePackImage),
	}}
	for _, sticker := range stickers {
		if sticker.MXC == "" {
			size, reader, err := mediadl.DownloadMedia(ctx, sticker.MIME, sticker.CDNURL, maxStickerSize)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				m.UserLogin.Log.Warn().Int64("sticker_id", sticker.ID).Msg("Skipping unavailable sticker media")
				continue
			}
			mxc, _, err := m.Main.Bridge.Bot.UploadMediaStream(ctx, "", size, false, func(dest io.Writer) (*bridgev2.FileStreamResult, error) {
				_, err := io.Copy(dest, reader)
				return &bridgev2.FileStreamResult{MimeType: sticker.MIME, FileName: strconv.FormatInt(sticker.ID, 10)}, err
			})
			reader.Close()
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				m.UserLogin.Log.Warn().Int64("sticker_id", sticker.ID).Msg("Skipping sticker upload failure")
				continue
			}
			err = m.Main.DB.SetSeenStickerMXC(ctx, m.UserLogin.ID, sticker.ID, mxc, sticker.MIME)
			if err != nil {
				return nil, err
			}
			sticker.MXC = mxc
		}
		stickerID := strconv.FormatInt(sticker.ID, 10)
		width, height := stickerDisplaySize(sticker.Width, sticker.Height)
		pack.Content.Images[stickerID] = &event.ImagePackImage{
			URL: sticker.MXC, Body: sticker.Body,
			Info: &event.FileInfo{MimeType: sticker.MIME, Width: width, Height: height, BridgedSticker: &event.BridgedSticker{
				Network: "facebook", ID: stickerID, PackURL: seenStickerPackURL,
			}},
		}
	}
	return pack, nil
}
