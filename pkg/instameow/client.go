// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package instameow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exhttp"
	"go.mau.fi/util/exsync"

	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/data/endpoints"
	"go.mau.fi/mautrix-meta/pkg/messagix/dgw"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type Client struct {
	http    *httpclient.HTTPClient
	configs *httpclient.Configs
	cookies *cookies.Cookies
	log     *zerolog.Logger

	socket        *dgw.Socket
	cancelSocket  atomic.Pointer[context.CancelFunc]
	socketStopped *exsync.Event
	connected     *exsync.Event
	socketRetries int

	eventHandler EventHandler

	seqID   int64
	seqIDTS time.Time
}

type EventHandler func(context.Context, slidetypes.ClientEvent) error

var _ httpclient.Client = (*Client)(nil)

type ClientParams struct {
	Cookies      *cookies.Cookies
	Log          zerolog.Logger
	Settings     exhttp.ClientSettings
	SeqID        int64
	SeqIDTS      time.Time
	EventHandler EventHandler
}

func NewClient(params ClientParams) *Client {
	c := &Client{
		cookies:       params.Cookies,
		log:           &params.Log,
		seqID:         params.SeqID,
		seqIDTS:       params.SeqIDTS,
		socketStopped: exsync.NewEvent(),
		connected:     exsync.NewEvent(),
	}
	c.SetEventHandler(params.EventHandler)
	c.configs = httpclient.NewConfigs(c)
	c.http = httpclient.NewHTTPClient(c, c.configs, params.Settings)
	c.socketStopped.Set()
	return c
}

func (c *Client) SetEventHandler(handler func(context.Context, slidetypes.ClientEvent) error) {
	if handler == nil {
		handler = func(context.Context, slidetypes.ClientEvent) error {
			return fmt.Errorf("event handler disabled")
		}
	}
	c.eventHandler = handler
}

func (c *Client) LoadIndex(ctx context.Context) (*types.PolarisViewer, *slidetypes.Mailbox, error) {
	if c == nil {
		return nil, nil, ErrClientIsNil
	} else if !c.cookies.IsLoggedIn() {
		return nil, nil, httpclient.ErrTokenInvalidated
	}

	moduleLoader := httpclient.NewModuleParser(c, c.http, c.configs)
	err := moduleLoader.Load(ctx, c.GetEndpoint("messages"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load inbox: %w", err)
	}
	err = c.configs.Setup(c.IsAuthenticated())
	if err != nil {
		return nil, nil, err
	}
	c.socket = dgw.NewSocket(c.getSocketOptions())
	mailbox, err := c.GetMailbox(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get mailbox: %w", err)
	}
	c.seqID = mailbox.Mailbox.UQSeqID
	c.seqIDTS = time.Now()
	return &c.configs.BrowserConfigTable.PolarisViewer, mailbox.Mailbox, err
}

func (c *Client) GetOwnFBID() int64 {
	if c.configs.BrowserConfigTable.CurrentUserInitialData.IGUserEIMU == "" || c.configs.BrowserConfigTable.PolarisViewer.Data.Fbid != c.configs.BrowserConfigTable.CurrentUserInitialData.NonFacebookUserID {
		return 0
	}
	fbid, _ := strconv.ParseInt(c.configs.BrowserConfigTable.CurrentUserInitialData.IGUserEIMU, 10, 64)
	return fbid
}

func (c *Client) GetCookies() *cookies.Cookies {
	return c.cookies
}

func (c *Client) GetEndpoint(name string) string {
	return endpoints.InstagramEndpoints[name]
}

func (c *Client) GetPlatform() types.Platform {
	return types.Instagram
}

func (c *Client) IsAuthenticated() bool {
	return c.cookies.IsLoggedIn() && c.configs.BrowserConfigTable.PolarisViewer.ID != ""
}

func (c *Client) GetLogger() *zerolog.Logger {
	return c.log
}

func (c *Client) SetLogger(logger zerolog.Logger) {
	c.log = &logger
}

func (c *Client) GetHTTP() *httpclient.HTTPClient {
	return c.http
}

type dumpedState struct {
	Configs   *httpclient.Configs
	SeqID     int64
	SeqIDTS   time.Time
	Timestamp time.Time
}

const MaxCachedStateAge = 24 * time.Hour

var ErrCachedStateTooOld = errors.New("cached state is too old")
var ErrClientIsNil = errors.New("client is nil")

func (c *Client) LoadState(state json.RawMessage) error {
	if c == nil {
		return ErrClientIsNil
	}
	var dumped dumpedState
	if err := json.Unmarshal(state, &dumped); err != nil {
		return err
	} else if !dumped.Timestamp.IsZero() && time.Since(dumped.Timestamp) > MaxCachedStateAge {
		return fmt.Errorf("%w (created at %s)", ErrCachedStateTooOld, dumped.Timestamp)
	}

	c.configs = dumped.Configs
	c.seqID = dumped.SeqID
	c.seqIDTS = dumped.SeqIDTS
	c.socket = dgw.NewSocket(c.getSocketOptions())
	return nil
}

func (c *Client) DumpState() (json.RawMessage, error) {
	if c == nil || c.configs == nil || c.seqID == 0 {
		return nil, nil
	}
	return json.Marshal(&dumpedState{
		Configs:   c.configs,
		SeqID:     c.seqID,
		SeqIDTS:   c.seqIDTS,
		Timestamp: time.Now(),
	})
}
