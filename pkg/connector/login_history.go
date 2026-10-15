package connector

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/loginerrors"
	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type MessengerHistoryLogin struct {
	historyComplete *bridgev2.LoginStep
	historyBackup   *securehistory.Backup
}

var historyPINPattern = regexp.MustCompile(`^[0-9]{6}$`)

func historyPINStep(message string) *bridgev2.LoginStep {
	instructions := "Enter your Messenger secure storage PIN to restore encrypted chat history. Leave it empty to continue without restoring history."
	if message != "" {
		instructions = message + "\n\n" + instructions
	}
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "fi.mau.meta.history_pin",
		Instructions: instructions,
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{{
				Type:      bridgev2.LoginInputFieldTypePassword,
				ID:        "pin",
				Name:      "Secure storage PIN",
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
	if !client.LoginMeta.Platform.IsMessenger() {
		return complete, nil
	}
	if client.canBackfillEncrypted() {
		return m.finishHistory(ctx)
	}
	var err error
	m.historyBackup, err = client.Client.GetSecureHistoryClient().GetBackup(ctx)
	if errors.Is(err, securehistory.ErrNoPINBackup) {
		return complete, nil
	} else if err != nil {
		client.UserLogin.Log.Warn().Err(err).Msg("Failed to check Messenger encrypted history")
		if errors.Is(err, types.ErrPleaseLogIn) || errors.Is(err, httpclient.ErrTokenInvalidated) {
			return nil, loginerrors.TokenInvalidated
		}
		return historyPINStep("Couldn't check your saved history. Enter your PIN to try again, or leave it empty to continue."), nil
	}
	return historyPINStep(""), nil
}

func (m *MessengerHistoryLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	if m.historyComplete == nil {
		return nil, errors.New("sign in before restoring Messenger history")
	} else if input["pin"] == "" {
		return m.finishHistory(ctx)
	} else if !historyPINPattern.MatchString(input["pin"]) {
		delete(input, "pin")
		return historyPINStep("Enter your six-digit PIN, or leave it empty to continue without restoring history."), nil
	}
	pin := []byte(input["pin"])
	delete(input, "pin")
	client := m.historyComplete.CompleteParams.UserLogin.Client.(*MetaClient)
	if err := m.restoreHistory(ctx, client, pin); err != nil {
		client.UserLogin.Log.Warn().Err(err).Msg("Failed to restore Messenger encrypted history")
		if errors.Is(err, types.ErrPleaseLogIn) || errors.Is(err, httpclient.ErrTokenInvalidated) {
			return nil, loginerrors.TokenInvalidated
		}
		message := "Couldn't restore encrypted history. You can leave the PIN empty to finish signing in."
		var recoveryErr *securehistory.RecoveryError
		if errors.As(err, &recoveryErr) && recoveryErr.AttemptsRemaining != nil {
			if *recoveryErr.AttemptsRemaining == 0 {
				return nil, loginerrors.HistoryPINLimit
			}
			message = fmt.Sprintf("Couldn't verify your PIN. Messenger reports %d attempts remaining.", *recoveryErr.AttemptsRemaining)
		} else if device := client.historyDevice(); device != nil && device.ID == "" {
			message = "Messenger hasn't confirmed history setup. Leave the PIN empty to finish signing in."
		}
		return historyPINStep(message), nil
	}
	return m.finishHistory(ctx)
}

func (m *MessengerHistoryLogin) finishHistory(ctx context.Context) (*bridgev2.LoginStep, error) {
	client := m.historyComplete.CompleteParams.UserLogin.Client.(*MetaClient)
	if client.canBackfillEncrypted() {
		client.backfillLock.Lock()
		err := client.UserLogin.Save(ctx)
		client.backfillLock.Unlock()
		if err != nil {
			client.UserLogin.Log.Warn().Err(err).Msg("Failed to save Messenger history enrollment")
			return historyPINStep("Couldn't save history setup. Leave the PIN empty to retry saving it."), nil
		}
		if err := client.queueEncryptedBackfill(ctx); err != nil {
			client.UserLogin.Log.Warn().Err(err).Msg("Failed to queue Messenger encrypted history")
			return historyPINStep("Couldn't queue your saved history. Leave the PIN empty to try again."), nil
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
	if err := client.e2eeConnectWaiter.WaitTimeoutCtx(ctx, ConnectWaitTimeout); err != nil {
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
	client.backfillLock.Lock()
	client.LoginMeta.HistoryDevice = device
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = client.UserLogin.Save(saveCtx)
	client.backfillLock.Unlock()
	return err
}
