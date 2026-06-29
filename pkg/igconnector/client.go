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

package igconnector

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type IGClient struct {
	Main      *IGConnector
	Client    *instameow.Client
	LoginMeta *metaid.UserLoginMetadata
	UserLogin *bridgev2.UserLogin
	Ghost     *bridgev2.Ghost

	caughtUp     *exsync.Event
	catchingUpTo int64

	stopConnectAttempt   atomic.Pointer[context.CancelFunc]
	mailboxProcessed     atomic.Bool
	waitMailboxProcessed chan struct{}
}

func (ic *IGConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	loginMetadata := login.Metadata.(*metaid.UserLoginMetadata)
	c := &IGClient{
		Main:      ic,
		LoginMeta: loginMetadata,
		UserLogin: login,
		caughtUp:  exsync.NewEvent(),
	}
	c.mailboxProcessed.Store(true)
	login.Client = c
	return nil
}

var (
	_ bridgev2.NetworkAPI                    = (*IGClient)(nil)
	_ bridgev2.CredentialExportingNetworkAPI = (*IGClient)(nil)
)

type respGetProxy struct {
	ProxyURL string `json:"proxy_url"`
}

// TODO this should be moved into mautrix-go

func (ic *IGConnector) getProxy(reason string) (string, error) {
	if ic.Config.GetProxyFrom == "" {
		return ic.Config.Proxy, nil
	}
	parsed, err := url.Parse(ic.Config.GetProxyFrom)
	if err != nil {
		return "", fmt.Errorf("failed to parse address: %w", err)
	}
	q := parsed.Query()
	q.Set("reason", reason)
	parsed.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("User-Agent", mautrix.DefaultUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	} else if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	var respData respGetProxy
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	return respData.ProxyURL, nil
}

func (ic *IGClient) ensureIGClient() {
	if ic.LoginMeta.Cookies != nil && ic.Client == nil && ic.LoginMeta.Platform == types.Instagram {
		ic.LoginMeta.Cookies.Platform = ic.LoginMeta.Platform
		ic.Client = instameow.NewClient(instameow.ClientParams{
			Cookies:      ic.LoginMeta.Cookies,
			Log:          ic.UserLogin.Log.With().Str("component", "instameow").Logger(),
			Settings:     ic.Main.Bridge.GetHTTPClientSettings(),
			EventHandler: ic.handleIGEvent,
		})
	}
}

func (ic *IGClient) ExportCredentials(ctx context.Context) any {
	if ic.Client == nil {
		return nil
	}
	return ic.Client.GetCookies()
}

func (ic *IGClient) Connect(ctx context.Context) {
	if ic.LoginMeta.Platform != types.Instagram {
		ic.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateBadCredentials, Error: MetaNotInstagram})
		return
	}
	ic.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})
	mailboxCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ic.stopConnectAttempt.Store(&cancel)
	ic.ensureIGClient()
	seqID, seqTS, err := ic.Main.DB.GetIGSeqID(ctx, ic.UserLogin.ID)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to get seq ID")
	} else if seqID != 0 {
		if ic.Client != nil && ic.Main.Config.CacheConnectionState {
			zerolog.Ctx(ctx).Debug().
				Int64("seq_id", seqID).
				Time("seq_ts", seqTS).
				Msg("Using saved seq ID")
			ic.Client.SetSeqID(seqID, seqTS)
		}
	} else {
		zerolog.Ctx(ctx).Debug().Msg("No saved seq ID")
	}
	ic.connectWithRetry(mailboxCtx, ctx, 0)
}

const MaxConnectRetries = 10

func (ic *IGClient) connectWithRetry(retryCtx, ctx context.Context, attempts int) {
	if retryCtx.Err() != nil {
		return
	}
	ic.ensureIGClient()
	cli := ic.Client
	if cli == nil {
		ic.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      MetaNotLoggedIn,
		})
		return
	}
	if ic.Main.Config.ProxyOther && (ic.Main.Config.GetProxyFrom != "" || ic.Main.Config.Proxy != "") {
		cli.GetHTTP().GetNewProxy = ic.Main.getProxy
		if !cli.GetHTTP().UpdateProxy("connect") {
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaProxyUpdateFail,
			})
			return
		}
	}
	if attempts > 0 {
		retryIn := time.Duration(1<<attempts) * time.Second
		zerolog.Ctx(ctx).Debug().Stringer("retry_in", retryIn).Msg("Sleeping before retrying connection")
		select {
		case <-time.After(retryIn):
		case <-retryCtx.Done():
			zerolog.Ctx(ctx).Err(ctx.Err()).Msg("Connection cancelled during sleep")
			return
		}
	} else if state, lastUsed, err := ic.Main.DB.GetReconnectionState(ctx, ic.UserLogin.ID); err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to get reconnection state")
	} else if state == nil {
		zerolog.Ctx(ctx).Debug().Msg("No saved reconnection state")
	} else if !ic.Main.Config.CacheConnectionState {
		zerolog.Ctx(ctx).Debug().Msg("Not using saved reconnection state as it's disabled in the config")
	} else if err = cli.LoadState(state); err != nil {
		zerolog.Ctx(ctx).Err(err).
			Time("last_used", lastUsed).
			Msg("Failed to load reconnection state")
	} else if cli.HasSeqID() {
		zerolog.Ctx(ctx).Debug().
			Time("last_used", lastUsed).
			Msg("Reconnecting with cached state")
		go cli.Connect(ctx)
		return
	}
	var currentUser *types.PolarisViewer
	var mailbox *slidetypes.Mailbox
	var err error
	if cli.HasSeqID() {
		zerolog.Ctx(ctx).Debug().Msg("Seq ID already stored, only reloading index")
		err = cli.ReloadIndex(ctx)
	} else {
		currentUser, mailbox, err = cli.LoadIndex(ctx)
	}
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to load index")
		if errors.Is(err, httpclient.ErrTokenInvalidated) {
			state := status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      MetaCookieRemoved,
			}
			if errors.Is(err, httpclient.ErrTokenInvalidatedRedirect) {
				state.Error = MetaRedirectedToLoginPage
			} else if errors.Is(err, httpclient.ErrUserIDIsZero) {
				state.Error = MetaUserIDIsZero
			}
			ic.UserLogin.BridgeState.Send(state)
			ic.Client = nil
			ic.LoginMeta.Cookies = nil
			err = ic.UserLogin.Save(ctx)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("Failed to save user login after clearing cookies")
			}
		} else if errors.Is(err, httpclient.ErrChallengeRequired) {
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      IGChallengeRequired,
				UserAction: status.UserActionRestart,
			})
		} else if errors.Is(err, httpclient.ErrAccountSuspended) {
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      IGAccountSuspended,
			})
		} else if errors.Is(err, httpclient.ErrCheckpointRequired) {
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      FBCheckpointRequired,
				UserAction: status.UserActionRestart,
			})
		} else if errors.Is(err, httpclient.ErrConsentRequired) {
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      IGConsentRequired,
				UserAction: status.UserActionRestart,
			})
		} else if lsErr := (&types.ErrorResponse{}); errors.As(err, &lsErr) {
			stateEvt := status.StateUnknownError
			if lsErr.ErrorCode == 1357053 {
				stateEvt = status.StateBadCredentials
			} else if attempts < MaxConnectRetries {
				stateEvt = status.StateTransientDisconnect
			}
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: stateEvt,
				Error:      status.BridgeStateErrorCode(fmt.Sprintf("meta-lserror-%d", lsErr.ErrorCode)),
				Message:    lsErr.Error(),
			})
			if stateEvt == status.StateTransientDisconnect {
				ic.connectWithRetry(retryCtx, ctx, attempts+1)
			}
		} else if gqlErr := (&types.GraphQLError{}); errors.As(err, &gqlErr) {
			// TODO determine if this should retry
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaGraphQLError,
				Message:    gqlErr.Message,
			})
		} else {
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateUnknownError,
				Error:      MetaConnectError,
				Info: map[string]any{
					"go_error": err.Error(),
				},
			})
		}
		return
	}
	if retryCtx.Err() != nil {
		zerolog.Ctx(ctx).Err(ctx.Err()).Msg("Connection cancelled")
		return
	}

	if mailbox != nil {
		ic.connectWithMailbox(ctx, retryCtx, currentUser, mailbox)
	}
	if retryCtx.Err() != nil {
		zerolog.Ctx(ctx).Err(ctx.Err()).Msg("Connection cancelled")
		return
	}
	zerolog.Ctx(ctx).Debug().Msg("Processed index, connecting to DGW")
	go ic.Client.Connect(ctx)
}

func (ic *IGClient) connectWithMailbox(ctx, retryCtx context.Context, currentUser *types.PolarisViewer, mailbox *slidetypes.Mailbox) {
	var err error
	ic.Ghost, err = ic.Main.Bridge.GetGhostByID(ctx, networkid.UserID(ic.UserLogin.ID))
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to get own ghost")
		ic.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      MetaConnectError,
		})
		return
	}

	ic.UserLogin.RemoteProfile.Name = currentUser.GetName()
	ic.UserLogin.RemoteProfile.Username = currentUser.GetUsername()
	ic.UserLogin.RemoteName = cmp.Or(ic.UserLogin.RemoteProfile.Name, ic.UserLogin.RemoteProfile.Username)
	// TODO update ghost avatar first?
	ic.UserLogin.RemoteProfile.Avatar = ic.Ghost.AvatarMXC
	meta := ic.UserLogin.Metadata.(*metaid.UserLoginMetadata)
	if meta.IGID == "" {
		meta.IGID = currentUser.ID
		err = ic.UserLogin.Save(ctx)
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Msg("Failed to save user login after updating IGID")
		}
	}

	zerolog.Ctx(ctx).Debug().Msg("Processing inbox")
	ic.processMailbox(ctx, retryCtx, mailbox)
}

func (ic *IGClient) Disconnect() {
	if stopConnectAttempt := ic.stopConnectAttempt.Swap(nil); stopConnectAttempt != nil {
		(*stopConnectAttempt)()
	}
	if cli := ic.Client; cli != nil {
		cli.Disconnect()
		ic.Client = nil
	}
}

func (ic *IGClient) IsLoggedIn() bool {
	return ic.Client.IsAuthenticated()
}

func (ic *IGClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return networkid.UserLoginID(userID) == ic.UserLogin.ID
}

func (ic *IGClient) LogoutRemote(ctx context.Context) {
	// TODO actual logout request?
	ic.Disconnect()
	ic.LoginMeta.Cookies = nil
}

func (ic *IGClient) FullReconnect(seqIDOnly bool) {
	if ic.LoginMeta.Cookies == nil {
		return
	}
	ctx := ic.UserLogin.Log.WithContext(ic.Main.Bridge.BackgroundCtx)
	ic.Disconnect()
	var err error
	if seqIDOnly {
		err = ic.Main.DB.DeleteIGSeqID(ctx, ic.UserLogin.ID)
	} else {
		err = ic.Main.DB.DeleteReconnectionState(ctx, ic.UserLogin.ID)
	}
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to delete reconnection state")
	}
	ic.Connect(ctx)
}
