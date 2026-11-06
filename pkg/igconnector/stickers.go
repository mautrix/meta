package igconnector

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/msgconv/mediadl"
)

const maxStickerSize = 10 * 1024 * 1024
const stickerCacheTime = 5 * time.Minute
const stickerErrorCacheTime = 30 * time.Second

const stickerPackURLPrefix = "instagram://sticker-pack/"

var _ bridgev2.StickerImportingNetworkAPI = (*IGClient)(nil)

type cachedStickerPack struct {
	pack      *bridgev2.ImportedImagePack
	fetchedAt time.Time
}

func (ic *IGClient) stickerPackMetadata(ctx context.Context, section *slidetypes.StickerTraySection) event.ImagePackMetadata {
	return event.ImagePackMetadata{
		DisplayName: strings.TrimSpace(section.Title),
		AvatarURL:   ic.stickerPackIcon(ctx, section),
		Usage:       []event.ImagePackUsage{event.ImagePackUsageSticker},
		BridgedPack: &event.BridgedStickerPack{Network: "instagram", URL: stickerPackURLPrefix + section.ID},
	}
}

func (ic *IGClient) stickerPackIcon(ctx context.Context, section *slidetypes.StickerTraySection) id.ContentURIString {
	parsed, err := url.Parse(section.IconV2.LightSelected)
	if section.IconV2.LightSelected == "" || err != nil {
		return ""
	}
	// Icon URLs have rotating signatures and hosts, so the cache is keyed by the CDN path.
	key := database.Key(fmt.Sprintf("instagram.sticker_pack_icon.%x", sha256.Sum256([]byte(parsed.Path))))
	if mxc := ic.Main.Bridge.DB.KV.Get(ctx, key); mxc != "" {
		return id.ContentURIString(mxc)
	}
	mxc, _, _, err := ic.uploadSticker(ctx, section.IconV2.LightSelected, section.ID)
	if err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Str("pack_id", section.ID).Msg("Failed to upload Instagram sticker pack icon")
		return ""
	}
	ic.Main.Bridge.DB.KV.Set(ctx, key, string(mxc))
	return mxc
}

func (ic *IGClient) getStickerPacks(ctx context.Context) ([]*slidetypes.StickerTraySection, error) {
	if ic.Client == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	ic.stickerLock.Lock()
	defer ic.stickerLock.Unlock()
	cacheTime := stickerCacheTime
	if ic.stickerTrayErr != nil {
		cacheTime = stickerErrorCacheTime
	}
	if time.Since(ic.stickerTrayFetchedAt) < cacheTime {
		return ic.stickerTray, ic.stickerTrayErr
	}
	var packs []*slidetypes.StickerTraySection
	resp, err := ic.Client.GetStickerTray(ctx)
	if err != nil {
		err = wrapInstagramRespError(fmt.Errorf("failed to get sticker tray: %w", err))
	} else {
		for _, edge := range resp.Data.Sections.Edges {
			if edge.Node != nil && edge.Node.Type == "FIRST_PARTY_STICKER" && edge.Node.ID != "" {
				packs = append(packs, edge.Node)
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ic.stickerTray, ic.stickerTrayErr, ic.stickerTrayFetchedAt = packs, err, time.Now()
	return packs, err
}

func (ic *IGClient) ListImagePacks(ctx context.Context) ([]*event.ImagePackMetadata, error) {
	if !ic.LoginMeta.Platform.IsInstagram() {
		return nil, nil
	}
	packs, err := ic.getStickerPacks(ctx)
	if err != nil {
		return nil, err
	}
	metadata := make([]*event.ImagePackMetadata, 0, len(packs))
	for _, section := range packs {
		meta := ic.stickerPackMetadata(ctx, section)
		metadata = append(metadata, &meta)
	}
	return metadata, nil
}

func (ic *IGClient) uploadSticker(ctx context.Context, mediaURL, name string) (id.ContentURIString, string, int, error) {
	_, reader, err := mediadl.DownloadMedia(ctx, "image/*", mediaURL, maxStickerSize)
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
	mxc, _, err := ic.Main.Bridge.Bot.UploadMedia(ctx, "", data, name, mimeType)
	return mxc, mimeType, len(data), err
}

func (ic *IGClient) DownloadImagePack(ctx context.Context, packURL string) (*bridgev2.ImportedImagePack, error) {
	if !ic.LoginMeta.Platform.IsInstagram() {
		return nil, mautrix.MNotFound.WithMessage("Unsupported sticker pack")
	}
	packID, ok := strings.CutPrefix(packURL, stickerPackURLPrefix)
	if !ok || packID == "" {
		return nil, mautrix.MNotFound.WithMessage("Unsupported sticker pack")
	}
	packs, err := ic.getStickerPacks(ctx)
	if err != nil {
		return nil, err
	}
	for _, section := range packs {
		if section.ID == packID {
			return ic.downloadStickerPack(ctx, section)
		}
	}
	return nil, mautrix.MNotFound.WithMessage("Sticker pack is not available on this account")
}

// Relayed stickers and stickers that aren't in a pack of this account's tray are sent as images.
func (ic *IGClient) getTrayStickerID(ctx context.Context, msg *bridgev2.MatrixMessage) string {
	info := msg.Content.Info
	if msg.Event.Type != event.EventSticker || msg.OrigSender != nil || !ic.LoginMeta.Platform.IsInstagram() ||
		info == nil || info.BridgedSticker == nil || info.BridgedSticker.Network != "instagram" || info.BridgedSticker.ID == "" {
		return ""
	}
	packID, ok := strings.CutPrefix(info.BridgedSticker.PackURL, stickerPackURLPrefix)
	if !ok || packID == "" {
		return ""
	}
	packs, err := ic.getStickerPacks(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to check sticker tray, sending sticker as image")
		return ""
	}
	for _, section := range packs {
		if section.ID != packID {
			continue
		}
		for _, edge := range section.Stickers.Edges {
			if edge.Node != nil && edge.Node.ID == info.BridgedSticker.ID {
				return edge.Node.ID
			}
		}
	}
	return ""
}

func (ic *IGClient) downloadStickerPack(ctx context.Context, section *slidetypes.StickerTraySection) (*bridgev2.ImportedImagePack, error) {
	// Preview and import both download the pack, so a recent result is reused.
	ic.stickerPackLock.Lock(section.ID)
	defer ic.stickerPackLock.Unlock(section.ID)
	if cached, ok := ic.stickerPacks.Get(section.ID); ok && time.Since(cached.fetchedAt) < stickerCacheTime {
		return cached.pack, nil
	}
	metadata := ic.stickerPackMetadata(ctx, section)
	pack := &bridgev2.ImportedImagePack{
		Shortcode: "instagram_" + section.ID,
		Content:   &event.ImagePackEventContent{Metadata: metadata, Images: make(map[string]*event.ImagePackImage)},
	}
	for _, edge := range section.Stickers.Edges {
		sticker := edge.Node
		if sticker == nil || sticker.ID == "" || sticker.AnimatedInfo == nil || sticker.AnimatedInfo.WebP == nil {
			continue
		}
		webp := sticker.AnimatedInfo.WebP
		mxc, mimeType, size, err := ic.uploadSticker(ctx, webp.URL, sticker.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to import sticker %s: %w", sticker.ID, err)
		}
		pack.Content.Images[sticker.ID] = &event.ImagePackImage{
			URL: mxc, Body: sticker.AltText,
			Info: &event.FileInfo{MimeType: mimeType, Size: size, Width: webp.Width, Height: webp.Height, BridgedSticker: &event.BridgedSticker{
				Network: "instagram", ID: sticker.ID, PackURL: metadata.BridgedPack.URL,
			}},
		}
	}
	if len(pack.Content.Images) == 0 {
		return nil, mautrix.MNotFound.WithMessage("Sticker pack is empty")
	}
	ic.stickerPacks.Set(section.ID, cachedStickerPack{pack: pack, fetchedAt: time.Now()})
	return pack, nil
}
