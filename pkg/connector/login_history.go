package connector

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mau.fi/util/l10n"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/status"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type MessengerHistoryLogin struct {
	historyComplete *bridgev2.LoginStep
	historyBackup   *securehistory.Backup
	historyRestored bool
	historyRelogin  bool
}

var historyPINPattern = regexp.MustCompile(`^[0-9]{6}$`)

func historyPINStep(instructions l10n.Message) *bridgev2.LoginStep {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "fi.mau.meta.history_pin",
		Instructions: instructions,
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{{
				Type:      bridgev2.LoginInputFieldTypePassword,
				ID:        "pin",
				Name:      msgFieldPINName.Message(),
				Optional:  true,
				Pattern:   historyPINPattern.String(),
				MinLength: 6,
				MaxLength: 6,
			}},
		},
	}
}

func (m *MessengerHistoryLogin) startHistory(ctx context.Context, complete *bridgev2.LoginStep) (*bridgev2.LoginStep, error) {
	m.historyComplete = complete
	client := complete.CompleteParams.UserLogin.Client.(*MetaClient)
	if client.canBackfillEncrypted() || !client.Main.Config.E2EEBackfill {
		return complete, nil
	}
	var err error
	m.historyBackup, err = client.Client.GetSecureHistoryClient().GetBackup(ctx)
	if errors.Is(err, securehistory.ErrNoPINBackup) {
		client.UserLogin.Log.Info().Msg("Messenger account has no encrypted history backup")
		return complete, nil
	} else if err != nil {
		client.UserLogin.Log.Warn().Err(err).Msg("Failed to check Messenger encrypted history")
		if errors.Is(err, types.ErrPleaseLogIn) || errors.Is(err, httpclient.ErrTokenInvalidated) {
			m.abortLogin(ctx, client)
			return nil, loginerrors.TokenInvalidated
		}
		return historyPINStep(msgStepHistoryPINCheckFailedInstructions.Message()), nil
	}
	client.UserLogin.Log.Info().Msg("Found Messenger encrypted history backup, asking for PIN")
	return historyPINStep(msgStepHistoryPINInstructions.Message()), nil
}

func (m *MessengerHistoryLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if m.historyComplete == nil {
		return nil, errors.New("sign in before restoring Messenger history")
	} else if input["pin"] == "" {
		return m.finishHistory(ctx)
	} else if !historyPINPattern.MatchString(input["pin"]) {
		delete(input, "pin")
		return historyPINStep(msgStepHistoryPINInvalidInstructions.Message()), nil
	}
	pin := []byte(input["pin"])
	delete(input, "pin")
	client := m.historyComplete.CompleteParams.UserLogin.Client.(*MetaClient)
	if err := m.restoreHistory(ctx, client, pin); err != nil {
		client.UserLogin.Log.Warn().Err(err).Msg("Failed to restore Messenger encrypted history")
		if errors.Is(err, types.ErrPleaseLogIn) || errors.Is(err, httpclient.ErrTokenInvalidated) {
			m.abortLogin(ctx, client)
			return nil, loginerrors.TokenInvalidated
		}
		instructions := msgStepHistoryPINRestoreFailedInstructions.Message()
		var recoveryErr *securehistory.RecoveryError
		if errors.As(err, &recoveryErr) && recoveryErr.AttemptsRemaining != nil {
			if *recoveryErr.AttemptsRemaining == 0 {
				return nil, loginerrors.HistoryPINLimit
			}
			instructions = msgStepHistoryPINWrongInstructions.Render(l10n.Params{"attempts": *recoveryErr.AttemptsRemaining})
		} else if device := client.historyDevice(); device != nil && device.ID == "" {
			instructions = msgStepHistoryPINUnconfirmedInstructions.Message()
		}
		return historyPINStep(instructions), nil
	}
	return m.finishHistory(ctx)
}

func (m *MessengerHistoryLogin) abortLogin(ctx context.Context, client *MetaClient) {
	state := status.BridgeState{
		StateEvent: status.StateBadCredentials,
		Error:      MetaCookieRemoved,
	}
	if !m.historyRelogin {
		client.UserLogin.Delete(ctx, state, bridgev2.DeleteOpts{})
		return
	}
	client.Disconnect()
	client.Client = nil
	client.LoginMeta.Cookies = nil
	client.UserLogin.BridgeState.Send(state)
	if err := client.UserLogin.Save(ctx); err != nil {
		client.UserLogin.Log.Err(err).Msg("Failed to save user login after clearing cookies")
	}
}

func (m *MessengerHistoryLogin) finishHistory(ctx context.Context) (*bridgev2.LoginStep, error) {
	client := m.historyComplete.CompleteParams.UserLogin.Client.(*MetaClient)
	if client.canBackfillEncrypted() {
		client.backfillLock.Lock()
		err := client.UserLogin.Save(ctx)
		client.backfillLock.Unlock()
		if err != nil {
			client.UserLogin.Log.Warn().Err(err).Msg("Failed to save Messenger history enrollment")
			return historyPINStep(msgStepHistoryPINSaveFailedInstructions.Message()), nil
		}
		if m.historyRestored {
			if err := client.queueEncryptedBackfill(ctx); err != nil {
				client.UserLogin.Log.Warn().Err(err).Msg("Failed to queue Messenger encrypted history")
				return historyPINStep(msgStepHistoryPINQueueFailedInstructions.Message()), nil
			}
			m.historyRestored = false
		}
	}
	return m.historyComplete, nil
}

func (m *MessengerHistoryLogin) restoreHistory(ctx context.Context, client *MetaClient, pin []byte) error {
	if device := client.historyDevice(); device != nil {
		if device.ID == "" {
			return errors.New("history enrollment is unconfirmed")
		}
		return nil
	}
	if err := waitForHistoryE2EE(ctx, client); err != nil {
		return err
	}
	history := client.Client.GetSecureHistoryClient()
	if m.historyBackup == nil {
		var err error
		m.historyBackup, err = history.GetBackup(ctx)
		if err != nil {
			return err
		}
	}
	recovery, err := history.RecoverPIN(ctx, m.historyBackup, pin)
	if err != nil {
		return err
	}
	virtual, err := history.GetVirtualDevice(ctx, m.historyBackup, metaid.ParseUserLoginID(client.UserLogin.ID), recovery.Code)
	if err != nil {
		return err
	}
	epochs, err := virtual.DeriveEpochs()
	if err != nil {
		return err
	}
	enrollment, err := virtual.PrepareDevice(m.historyBackup.ID, history.RegistrationID, client.WADevice.IdentityKey, epochs)
	if err != nil {
		return err
	}
	client.backfillLock.Lock()
	client.LoginMeta.HistoryDevice = enrollment.Device
	if err = client.UserLogin.Save(ctx); err != nil {
		client.LoginMeta.HistoryDevice = nil
		client.backfillLock.Unlock()
		return err
	}
	client.backfillLock.Unlock()
	device, err := history.AddDevice(ctx, enrollment)
	if err != nil {
		var rejected *securehistory.EnrollmentError
		if errors.As(err, &rejected) {
			client.backfillLock.Lock()
			client.LoginMeta.HistoryDevice = nil
			saveErr := client.UserLogin.Save(ctx)
			client.backfillLock.Unlock()
			return errors.Join(err, saveErr)
		}
		return err
	}
	client.UserLogin.Log.Info().Str("history_device_id", device.ID).Msg("Enrolled Messenger encrypted history device")
	client.backfillLock.Lock()
	client.LoginMeta.HistoryDevice = device
	m.historyRestored = true
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = client.UserLogin.Save(saveCtx)
	client.backfillLock.Unlock()
	return err
}

func waitForHistoryE2EE(ctx context.Context, client *MetaClient) error {
	deadline := time.Now().Add(ConnectWaitTimeout)
	for {
		if !client.IsLoggedIn() {
			return fmt.Errorf("%w: logged out while waiting for encrypted messaging", httpclient.ErrTokenInvalidated)
		}
		wait := min(time.Until(deadline), time.Second)
		if wait <= 0 {
			return errors.New("timed out waiting for encrypted messaging to connect")
		}
		err := client.e2eeConnectWaiter.WaitTimeoutCtx(ctx, wait)
		if err == nil && client.Client != nil {
			return nil
		} else if err == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		} else if ctx.Err() != nil {
			return err
		}
	}
}
