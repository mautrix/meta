package connector

import (
	"errors"

	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (m *MetaClient) e2eeEventHandler(rawEvt any) {
	log := m.UserLogin.Log
	switch evt := rawEvt.(type) {
	case *events.FBMessage:
		m.UserLogin.Log.Trace().
			Any("info", evt.Info).
			Any("transport", evt.Transport).
			Any("application", evt.FBApplication).
			Any("ig_transport", evt.IGTransport).
			Any("payload", evt.Message).
			Msg("Received WhatsApp message")
		m.Main.Bridge.QueueRemoteEvent(m.UserLogin, &EnsureWAChatStateEvent{JID: evt.Info.Chat, m: m})
		m.Main.Bridge.QueueRemoteEvent(m.UserLogin, &WAMessageEvent{FBMessage: evt, m: m})
	case *events.Receipt:
		var evtType bridgev2.RemoteEventType
		switch evt.Type {
		case waTypes.ReceiptTypeRead, waTypes.ReceiptTypeReadSelf:
			evtType = bridgev2.RemoteEventReadReceipt
		case waTypes.ReceiptTypeDelivered:
			evtType = bridgev2.RemoteEventDeliveryReceipt
		}
		targets := make([]networkid.MessageID, len(evt.MessageIDs))
		messageSender := *m.WADevice.ID
		if !evt.MessageSender.IsEmpty() {
			messageSender = evt.MessageSender
		}
		for i, id := range evt.MessageIDs {
			targets[i] = metaid.MakeWAMessageID(evt.Chat, messageSender, id)
		}
		m.Main.Bridge.QueueRemoteEvent(m.UserLogin, &simplevent.Receipt{
			EventMeta: simplevent.EventMeta{
				Type:       evtType,
				LogContext: nil,
				PortalKey:  m.makeWAPortalKey(evt.Chat),
				Sender:     m.makeWAEventSender(evt.Sender),
				Timestamp:  evt.Timestamp,
			},
			Targets: targets,
		})
	case *events.OfflineSyncPreview:
		m.connectBackgroundWAEventCount.Store(uint32(evt.Messages))
	case *events.OfflineSyncCompleted:
		m.connectBackgroundWAOfflineSync.Set()
		log.Debug().Int("event_count", evt.Count).Msg("WhatsApp offline sync completed")
	case *events.Connected:
		log.Debug().Msg("Connected to WhatsApp socket")
		m.e2eeConnectWaiter.Set()
		m.waState = status.BridgeState{StateEvent: status.StateConnected}
		m.UserLogin.BridgeState.Send(m.waState)
	case *events.Disconnected:
		log.Debug().Msg("Disconnected from WhatsApp socket")
		m.waState = status.BridgeState{
			StateEvent: status.StateTransientDisconnect,
			Error:      WADisconnected,
		}
		m.UserLogin.BridgeState.Send(m.waState)
	case *events.CATRefreshError:
		if errors.Is(evt.Error, types.ErrPleaseReloadPage) && m.canReconnect() {
			log.Err(evt.Error).Msg("Got CATRefreshError, reloading page")
			go m.FullReconnect()
			return
		}
		m.waState = status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      WACATError,
			Message:    evt.PermanentDisconnectDescription(),
		}
		m.UserLogin.BridgeState.Send(m.waState)
		if m.canReconnect() {
			go m.FullReconnect()
		}
		//go m.sendMarkdownBridgeAlert(context.TODO(), "Error in WhatsApp connection: %s", evt.PermanentDisconnectDescription())
	case events.PermanentDisconnect:
		switch e := evt.(type) {
		case *events.LoggedOut:
			if e.Reason == events.ConnectFailureLoggedOut && !e.OnConnect && m.canReconnect() {
				m.resetWADevice()
				log.Debug().Msg("Doing full reconnect after WhatsApp 401 error")
				go m.FullReconnect()
			}
		case *events.ConnectFailure:
			if e.Reason == events.ConnectFailureNotFound {
				if cli := m.E2EEClient; cli != nil {
					cli.Disconnect()
					err := m.WADevice.Delete(log.WithContext(m.Main.Bridge.BackgroundCtx))
					if err != nil {
						log.Err(err).Msg("Failed to delete WhatsApp device after 415 error")
					}
					m.resetWADevice()
					m.E2EEClient = nil
				}
				log.Debug().Msg("Reconnecting e2ee client after WhatsApp 415 error")
				go m.tryConnectE2EE(true)
			} else if e.Reason == events.ConnectFailureClientUnknown {
				m.resetWADevice()
				log.Debug().Msg("Doing full reconnect after WhatsApp 418 error")
				go m.FullReconnect()
			}
		}

		m.waState = status.BridgeState{
			StateEvent: status.StateUnknownError,
			Error:      WAPermanentError,
			Message:    evt.PermanentDisconnectDescription(),
		}
		m.UserLogin.BridgeState.Send(m.waState)
		//go m.sendMarkdownBridgeAlert(context.TODO(), "Error in WhatsApp connection: %s", evt.PermanentDisconnectDescription())
	default:
		log.Debug().Type("event_type", rawEvt).Msg("Unhandled WhatsApp event")
	}
}
