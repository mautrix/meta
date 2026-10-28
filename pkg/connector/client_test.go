package connector

import (
	"testing"

	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func TestFillBridgeState(t *testing.T) {
	connected := status.BridgeState{StateEvent: status.StateConnected}
	transient := status.BridgeState{StateEvent: status.StateTransientDisconnect, Error: MetaTransientDisconnect}
	waFailed := status.BridgeState{StateEvent: status.StateUnknownError, Error: WAConnectError}
	consent := status.BridgeState{StateEvent: status.StateBadCredentials, Error: FBConsentRequired, UserAction: status.UserActionRestart}
	metaFailed := status.BridgeState{StateEvent: status.StateUnknownError, Error: MetaConnectError}

	for name, tc := range map[string]struct {
		input, meta, wa status.BridgeState
		expected        status.BridgeState
	}{
		"WhatsAppFailureWhileMetaConnected": {
			input:    waFailed,
			meta:     connected,
			wa:       waFailed,
			expected: status.BridgeState{StateEvent: status.StateConnected, Error: WAConnectError},
		},
		"MetaReconnectWhileWhatsAppFailed": {
			input:    connected,
			meta:     connected,
			wa:       waFailed,
			expected: status.BridgeState{StateEvent: status.StateConnected, Error: WAConnectError},
		},
		"WhatsAppFailureWhileMetaReconnecting": {
			input:    waFailed,
			meta:     transient,
			wa:       waFailed,
			expected: transient,
		},
		"MetaFailureWhileWhatsAppFailed": {
			input:    metaFailed,
			meta:     connected,
			wa:       waFailed,
			expected: metaFailed,
		},
		"WhatsAppRecovered": {
			input:    connected,
			meta:     connected,
			wa:       connected,
			expected: connected,
		},
		"WhatsAppConnectedKeepsStoredBadCredentials": {
			input:    connected,
			meta:     consent,
			wa:       connected,
			expected: consent,
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := &MetaClient{
				LoginMeta: &metaid.UserLoginMetadata{Platform: types.Messenger},
				metaState: tc.meta,
				waState:   tc.wa,
			}
			out := m.FillBridgeState(tc.input)
			if out.StateEvent != tc.expected.StateEvent || out.Error != tc.expected.Error || out.UserAction != tc.expected.UserAction {
				t.Errorf("expected %s/%s/%s, got %s/%s/%s",
					tc.expected.StateEvent, tc.expected.Error, tc.expected.UserAction,
					out.StateEvent, out.Error, out.UserAction)
			}
		})
	}
}
