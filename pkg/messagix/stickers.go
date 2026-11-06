package messagix

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	maxStickerTrayPages = 5
	maxStickerPackPages = 10
	// The picker query returns thumbnail-sized renditions unless asked for a larger box.
	stickerMediaSize = 512
)

type StickerPack struct {
	ID   string
	Name string
}

type Sticker struct {
	ID     string
	Label  string
	URL    string
	Width  int
	Height int
}

type stickerPageInfo struct {
	EndCursor   string `json:"end_cursor"`
	HasNextPage bool   `json:"has_next_page"`
}

type stickerTrayPacks struct {
	Edges []struct {
		Node *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"node"`
	} `json:"edges"`
	PageInfo stickerPageInfo `json:"page_info"`
}

type stickerStore struct {
	ID        string            `json:"id"`
	TrayPacks *stickerTrayPacks `json:"tray_packs"`
}

type stickerImage struct {
	URI    string `json:"uri"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type stickersInPack struct {
	ID       string `json:"id"`
	Stickers *struct {
		Edges []struct {
			Node *struct {
				ID                 string        `json:"id"`
				Label              string        `json:"label"`
				AccessibilityLabel string        `json:"accessibility_label"`
				Image              *stickerImage `json:"image"`
				AnimatedImage      *stickerImage `json:"animated_image"`
				Pack               *struct {
					ID string `json:"id"`
				} `json:"pack"`
			} `json:"node"`
		} `json:"edges"`
		PageInfo stickerPageInfo `json:"page_info"`
	} `json:"stickers"`
}

func isStickerFBID(id string) bool {
	parsed, err := strconv.ParseInt(id, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == id
}

func isStickerMediaURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func nextStickerCursor(info stickerPageInfo, seen map[string]struct{}) (string, bool, error) {
	if !info.HasNextPage {
		return "", false, nil
	} else if info.EndCursor == "" {
		return "", false, fmt.Errorf("response has next page without cursor")
	} else if _, ok := seen[info.EndCursor]; ok {
		return "", false, fmt.Errorf("response repeated page cursor")
	}
	seen[info.EndCursor] = struct{}{}
	return info.EndCursor, true, nil
}

func (c *Client) FetchStickerTray(ctx context.Context) ([]StickerPack, error) {
	if c == nil {
		return nil, ErrClientIsNil
	}
	var card struct {
		PickerPlugins *struct {
			StickerPicker *struct {
				StickerStore *stickerStore `json:"sticker_store"`
			} `json:"sticker_picker"`
		} `json:"picker_plugins"`
	}
	err := c.makeGraphQLRequest(ctx, "CometStickerPickerCardQuery", map[string]any{
		"feedbackID": nil, "scale": 1, "stickerInterface": "MESSAGES",
	}, &card)
	if err != nil {
		return nil, err
	} else if card.PickerPlugins == nil || card.PickerPlugins.StickerPicker == nil ||
		card.PickerPlugins.StickerPicker.StickerStore == nil {
		return nil, fmt.Errorf("response didn't contain sticker store")
	}
	store := card.PickerPlugins.StickerPicker.StickerStore
	storeID := store.ID
	var packs []StickerPack
	seenPacks := make(map[string]struct{})
	seenCursors := make(map[string]struct{})
	for page := 1; ; page++ {
		if store.TrayPacks == nil {
			return nil, fmt.Errorf("response didn't contain sticker tray")
		}
		for _, edge := range store.TrayPacks.Edges {
			if edge.Node == nil || !isStickerFBID(edge.Node.ID) {
				continue
			} else if _, ok := seenPacks[edge.Node.ID]; ok {
				continue
			}
			name := strings.TrimSpace(edge.Node.Name)
			if name == "" {
				name = "Facebook stickers"
			}
			seenPacks[edge.Node.ID] = struct{}{}
			packs = append(packs, StickerPack{ID: edge.Node.ID, Name: name})
		}
		cursor, hasNext, err := nextStickerCursor(store.TrayPacks.PageInfo, seenCursors)
		if err != nil {
			return nil, err
		} else if !hasNext || page >= maxStickerTrayPages {
			return packs, nil
		} else if storeID == "" {
			return nil, fmt.Errorf("response didn't contain sticker store ID")
		}
		var next struct {
			Store *stickerStore `json:"fetch__StickerStore"`
		}
		err = c.makeGraphQLRequest(ctx, "useCometStickerPickerTrayPacksPaginationQuery", map[string]any{
			"after_cursor": cursor, "count": 20, "id": storeID, "scale": 1,
			"stickerInterface": "MESSAGES", "thumbnailHeight": 20, "thumbnailWidth": 20,
		}, &next)
		if err != nil {
			return nil, err
		} else if next.Store == nil {
			return nil, fmt.Errorf("response didn't contain sticker store")
		}
		store = next.Store
	}
}

func (c *Client) FetchStickerPack(ctx context.Context, packID string) ([]Sticker, error) {
	if c == nil {
		return nil, ErrClientIsNil
	} else if !isStickerFBID(packID) {
		return nil, fmt.Errorf("invalid sticker pack ID")
	}
	var root struct {
		Pack *stickersInPack `json:"sticker_pack"`
	}
	err := c.makeGraphQLRequest(ctx, "CometStickerPickerPackContentRootQuery", map[string]any{
		"packID": packID, "scale": 1, "stickerHeight": stickerMediaSize, "stickerWidth": stickerMediaSize,
	}, &root)
	if err != nil {
		return nil, err
	}
	pack := root.Pack
	var stickers []Sticker
	seenStickers := make(map[string]struct{})
	seenCursors := make(map[string]struct{})
	for page := 1; ; page++ {
		if pack == nil || pack.ID != packID || pack.Stickers == nil {
			return nil, fmt.Errorf("response didn't contain requested sticker pack")
		}
		for _, edge := range pack.Stickers.Edges {
			node := edge.Node
			if node == nil || !isStickerFBID(node.ID) || (node.Pack != nil && node.Pack.ID != packID) {
				continue
			} else if _, ok := seenStickers[node.ID]; ok {
				continue
			}
			mediaURL := ""
			if node.AnimatedImage != nil && isStickerMediaURL(node.AnimatedImage.URI) {
				mediaURL = node.AnimatedImage.URI
			} else if node.Image != nil && isStickerMediaURL(node.Image.URI) {
				mediaURL = node.Image.URI
			} else {
				continue
			}
			label := node.Label
			if label == "" {
				label = node.AccessibilityLabel
			}
			seenStickers[node.ID] = struct{}{}
			sticker := Sticker{ID: node.ID, Label: label, URL: mediaURL}
			if node.Image != nil {
				sticker.Width, sticker.Height = node.Image.Width, node.Image.Height
			}
			stickers = append(stickers, sticker)
		}
		cursor, hasNext, err := nextStickerCursor(pack.Stickers.PageInfo, seenCursors)
		if err != nil {
			return nil, err
		} else if !hasNext || page >= maxStickerPackPages {
			return stickers, nil
		}
		var next struct {
			Pack *stickersInPack `json:"node"`
		}
		err = c.makeGraphQLRequest(ctx, "CometStickerPickerPackContentPaginationQuery", map[string]any{
			"after": cursor, "first": 40, "id": packID, "scale": 1, "sticker_height": stickerMediaSize, "sticker_width": stickerMediaSize,
		}, &next)
		if err != nil {
			return nil, err
		}
		pack = next.Pack
	}
}
