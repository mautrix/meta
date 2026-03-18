package connector

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

func (m *MetaClient) StartThreadBackfill(ctx context.Context) error {
	log := m.UserLogin.Log.With().Str("action", "thread_backfill").Logger()
	ctx = log.WithContext(ctx)

	if m.Main.Config.ThreadBackfill.BatchCount == 0 {
		log.Debug().Msg("Thread backfill disabled")
		return nil
	}

	if m.LoginMeta.BackfillCompleted {
		log.Debug().Msg("Thread backfill already completed, skipping")
		return nil
	}
	log.Info().Msg("Starting thread backfill")

	// First run backfill for group 1 = Facebook chats
	if err := m.runThreadBackfillForSyncGroup(ctx, 1); err != nil {
		return err
	}
	// Then backfill any other sync groups with more threads available
	additionalGroups := m.Client.GetSyncGroupsWithMoreThreads()
	for syncGroup := range additionalGroups {
		if err := m.runThreadBackfillForSyncGroup(ctx, syncGroup); err != nil {
			return err
		}
	}

	// Only once all sync groups have been paginated we mark backfill as complete
	m.LoginMeta.BackfillCompleted = true
	if err := m.UserLogin.Save(ctx); err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("Failed to save backfill completion state")
	}
	return nil
}

func (m *MetaClient) runThreadBackfillForSyncGroup(ctx context.Context, syncGroup int64) error {
	log := zerolog.Ctx(ctx).With().Str("action", "thread_backfill").Int64("sync_group", syncGroup).Logger()

	delay := m.Main.Config.ThreadBackfill.BatchDelay
	batchLimit := m.Main.Config.ThreadBackfill.BatchCount
	batchCount := 0

	var prevMinThreadKey int64

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		keyStore, tbl, err := m.Client.FetchMoreThreads(ctx, syncGroup)
		if err != nil {
			log.Err(err).Msg("Failed to fetch more threads")
			return err
		} else if tbl == nil {
			log.Info().Int("batches_processed", batchCount).Msg("Thread backfill complete - no more threads")
			return nil
		}

		batchCount++

		// Process received threads (handled via normal event flow)
		m.parseAndQueueTable(ctx, tbl, false)

		// Check if more threads available - note HasMoreBefore may never become false, so we watch
		// for empty tables as well to identify when we've fully paginated.
		if keyStore == nil || !keyStore.HasMoreBefore {
			log.Info().Int("batches_processed", batchCount).Any("keystore", keyStore).Msg("Thread backfill complete - fully paginated (has no more)")
			return nil
		} else if keyStore.MinThreadKey == prevMinThreadKey {
			log.Info().Int("batches_processed", batchCount).Any("keystore", keyStore).Msg("Thread backfill complete - fully paginated (thread key did not change)")
			return nil
		} else if batchLimit > 0 && batchCount >= batchLimit {
			log.Info().Int("batched_processed", batchCount).Msg("Thread backfill complete - hit batch count limit")
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
