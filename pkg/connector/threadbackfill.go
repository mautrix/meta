package connector

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (m *MetaClient) StartThreadBackfill(ctx context.Context) error {
	if m.Main.Config.ThreadBackfill.BatchCount == 0 {
		return nil
	}

	log := m.UserLogin.Log.With().Str("action", "thread_backfill").Logger()
	ctx = log.WithContext(ctx)

	if m.LoginMeta.BackfillCompleted {
		log.Debug().Msg("Thread backfill already completed, skipping")
	} else {
		log.Info().Msg("Starting thread backfill")
		if err := m.runThreadBackfill(ctx); err != nil {
			return err
		}
	}

	if !m.LoginMeta.E2EEBackfillCompleted {
		log.Info().Msg("Starting E2EE thread backfill")
		return m.runE2EEThreadBackfill(ctx)
	}
	return nil
}

func (m *MetaClient) runE2EEThreadBackfill(ctx context.Context) error {
	const syncGroup = 95
	log := zerolog.Ctx(ctx)
	if err := m.Client.WaitForInitialThreadRange(ctx, syncGroup); err != nil {
		return err
	}
	batchCount := 0
	for {
		keyStore := m.Client.GetSyncGroupKeyStore(syncGroup)
		if keyStore == nil || !keyStore.HasMoreBefore {
			log.Info().Int("batches_processed", batchCount).Msg("E2EE thread backfill complete")
			break
		} else if limit := m.Main.Config.ThreadBackfill.BatchCount; limit > 0 && batchCount >= limit {
			log.Info().Int("batches_processed", batchCount).Msg("E2EE thread backfill complete - hit batch count limit")
			break
		}
		prevMinThreadKey := keyStore.MinThreadKey
		gen := m.Client.ThreadRangeGeneration(syncGroup)
		_, tbl, err := m.Client.FetchMoreThreads(ctx, syncGroup)
		if err != nil {
			log.Err(err).Msg("Failed to fetch more E2EE threads")
			return err
		} else if tbl != nil {
			m.parseAndQueueTable(ctx, tbl, false)
		}
		if err = m.Client.WaitForThreadRange(ctx, syncGroup, gen); err != nil {
			return err
		}
		batchCount++
		if keyStore.HasMoreBefore && keyStore.MinThreadKey == prevMinThreadKey {
			log.Info().Int("batches_processed", batchCount).Msg("E2EE thread backfill complete - thread key did not change")
			break
		}
		log.Debug().
			Int("batch", batchCount).
			Int64("min_thread_key", keyStore.MinThreadKey).
			Int64("min_activity_ts", keyStore.MinLastActivityTimestampMs).
			Msg("Processed E2EE thread backfill batch")
		select {
		case <-time.After(m.Main.Config.ThreadBackfill.BatchDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.LoginMeta.E2EEBackfillCompleted = true
	if err := m.UserLogin.Save(ctx); err != nil {
		log.Err(err).Msg("Failed to save E2EE backfill completion state")
	}
	return nil
}

func (m *MetaClient) runThreadBackfill(ctx context.Context) error {
	log := zerolog.Ctx(ctx)
	delay := m.Main.Config.ThreadBackfill.BatchDelay
	batchLimit := m.Main.Config.ThreadBackfill.BatchCount
	batchCount := 0
	var prevMinThreadKey int64

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Fetch next batch of threads, TODO: other SyncGroups?
		keyStore, tbl, err := m.Client.FetchMoreThreads(ctx, 1) // SyncGroup 1
		if err != nil {
			log.Err(err).Msg("Failed to fetch more threads")
			return err
		} else if tbl == nil {
			log.Info().Int("batches_processed", batchCount).Msg("Thread backfill complete - no more threads")
			m.markBackfillComplete(ctx, m.LoginMeta)
			return nil
		}

		batchCount++

		// Process received threads (handled via normal event flow)
		m.parseAndQueueTable(ctx, tbl, false)

		// Check if more threads available - note HasMoreBefore may never become false, so we watch
		// for empty tables as well to identify when we've fully paginated.
		if keyStore == nil || !keyStore.HasMoreBefore {
			log.Info().
				Int("batches_processed", batchCount).
				Any("keystore", keyStore).
				Msg("Thread backfill complete - fully paginated (has no more)")
			m.markBackfillComplete(ctx, m.LoginMeta)
			return nil
		} else if keyStore.MinThreadKey == prevMinThreadKey {
			log.Info().
				Int("batches_processed", batchCount).
				Any("keystore", keyStore).
				Msg("Thread backfill complete - fully paginated (thread key did not change)")
			m.markBackfillComplete(ctx, m.LoginMeta)
			return nil
		} else if batchLimit > 0 && batchCount >= batchLimit {
			log.Info().Int("batched_processed", batchCount).
				Msg("Thread backfill complete - hit batch count limit")
			m.markBackfillComplete(ctx, m.LoginMeta)
			return nil
		}

		prevMinThreadKey = keyStore.MinThreadKey

		log.Debug().
			Int("batch", batchCount).
			Int64("min_thread_key", keyStore.MinThreadKey).
			Int64("min_activity_ts", keyStore.MinLastActivityTimestampMs).
			Msg("Processed thread backfill batch")

		// Rate limiting delay
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (m *MetaClient) markBackfillComplete(ctx context.Context, meta *metaid.UserLoginMetadata) {
	meta.BackfillCompleted = true
	if err := m.UserLogin.Save(ctx); err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to save backfill completion state")
	}
}
