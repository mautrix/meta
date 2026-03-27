package connector

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
)

func (m *MetaClient) syncInstagramMessageRequests(ctx context.Context) error {
	if !m.LoginMeta.Platform.IsInstagram() {
		return nil
	} else if m.Client == nil || m.Client.Instagram == nil {
		return fmt.Errorf("instagram client is not available")
	}

	threads, err := m.Client.Instagram.FetchMessageRequests(ctx)
	if err != nil {
		return err
	}

	log := zerolog.Ctx(ctx)
	log.Debug().Int("thread_count", len(threads)).Msg("Queueing Instagram message requests")
	for _, thread := range threads {
		m.UserLogin.QueueRemoteEvent(&VerifyThreadExistsEvent{
			LSVerifyThreadExists: thread,
			m:                    m,
		})
	}

	return nil
}

func (m *MetaClient) startMessageRequestSync(ctx context.Context) {
	if !m.LoginMeta.Platform.IsInstagram() {
		return
	}
	go func() {
		if err := m.syncInstagramMessageRequests(ctx); err != nil {
			m.UserLogin.Log.Err(err).Msg("Instagram message request sync failed")
		}
	}()
}
