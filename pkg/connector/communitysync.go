package connector

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

const (
	communitySyncMinInterval  = 1 * time.Minute
	communitySyncStaleAfter   = 30 * time.Minute
	communitySyncRequestDelay = 1 * time.Second
	communitySyncRetryPeriod  = 30 * time.Second
)

type communitySyncState struct {
	SequenceID          int64
	LastSyncTimestampMs int64
	Stale               bool
	LastRequested       time.Time
	Pending             bool
	PendingHole         *table.LSVerifyAndUpsertThreadQueueSequenceID
	PendingHolePrev     int64
}

type communitySyncRequest struct {
	ThreadKey int64
	Hole      *table.LSVerifyAndUpsertThreadQueueSequenceID
	PrevSeqID int64
}

const (
	communitySweepRequested = 1 << iota
	communitySweepAll
	communitySweepMarkStale
)

func isSelectiveSyncThread(tt table.ThreadType) bool {
	switch tt {
	case table.COMMUNITY_GROUP, table.COMMUNITY_PRIVATE_HIDDEN_JOINED_THREAD, table.COMMUNITY_BROADCAST_JOINED_THREAD,
		table.COMMUNITY_SUB_THREAD, table.DISCOVERABLE_PUBLIC_BROADCAST_CHAT:
		return true
	default:
		return false
	}
}

func (m *MetaClient) startCommunitySync(ctx context.Context, onlyOutdated bool) {
	if !m.LoginMeta.Platform.IsMessenger() {
		return
	}
	m.communityLoopLock.Lock()
	defer m.communityLoopLock.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	if oldCancel := m.stopCommunitySync.Swap(&cancel); oldCancel != nil {
		(*oldCancel)()
	}
	go m.communitySyncLoop(ctx)
	flags := int32(communitySweepRequested)
	if !onlyOutdated {
		flags |= communitySweepAll
	}
	m.requestCommunitySweep(flags)
}

func (m *MetaClient) stopCommunitySyncLoop() {
	m.communityLoopLock.Lock()
	defer m.communityLoopLock.Unlock()
	if stop := m.stopCommunitySync.Swap(nil); stop != nil {
		(*stop)()
	}
}

func (m *MetaClient) requestCommunitySweep(flags int32) {
	for {
		old := m.communitySweepFlags.Load()
		if m.communitySweepFlags.CompareAndSwap(old, old|flags) {
			break
		}
	}
	select {
	case m.communitySweepWake <- struct{}{}:
	default:
	}
}

func (m *MetaClient) queueCommunitySync(ctx context.Context, req communitySyncRequest) {
	select {
	case m.communitySyncQueue <- req:
	default:
		zerolog.Ctx(ctx).Debug().Int64("thread_id", req.ThreadKey).Msg("Community sync queue is full, deferring request")
		m.deferCommunitySync(req)
	}
}

func (m *MetaClient) handleCommunitySyncTables(ctx context.Context, tbl *table.LSTable) {
	if !m.LoginMeta.Platform.IsMessenger() {
		return
	}
	m.communitySyncLock.Lock()
	for _, upd := range tbl.LSUpdateSelectiveSyncStateV2 {
		st := m.communitySync[upd.ThreadKey]
		st.SequenceID = upd.ThreadQueueSequenceID
		if upd.LastSelectiveSyncTimestampMs != 0 {
			st.LastSyncTimestampMs = upd.LastSelectiveSyncTimestampMs
		}
		m.communitySync[upd.ThreadKey] = st
	}
	for _, upd := range tbl.LSUpdateSelectiveSyncState {
		st := m.communitySync[upd.ThreadKey]
		st.LastSyncTimestampMs = upd.LastSelectiveSyncTimestampMs
		m.communitySync[upd.ThreadKey] = st
	}
	var requests []communitySyncRequest
	for _, upd := range tbl.LSUpdateCommunityThreadStaleState {
		st := m.communitySync[upd.ThreadKey]
		st.Stale = upd.IsStale
		m.communitySync[upd.ThreadKey] = st
		if upd.IsStale {
			requests = append(requests, communitySyncRequest{ThreadKey: upd.ThreadKey})
		}
	}
	for _, upd := range tbl.LSVerifyAndUpsertThreadQueueSequenceID {
		st := m.communitySync[upd.ThreadKey]
		prev := st.SequenceID
		if upd.ThreadQueueSequenceID > prev {
			st.SequenceID = upd.ThreadQueueSequenceID
			m.communitySync[upd.ThreadKey] = st
		}
		if prev != 0 && prev+1 < upd.ThreadQueueSequenceID {
			st.Stale = true
			m.communitySync[upd.ThreadKey] = st
			requests = append(requests, communitySyncRequest{ThreadKey: upd.ThreadKey, Hole: upd, PrevSeqID: prev})
		}
	}
	m.communitySyncLock.Unlock()
	if len(tbl.LSMarkJoinedCommunityThreadsStale) > 0 {
		m.requestCommunitySweep(communitySweepRequested | communitySweepAll | communitySweepMarkStale)
	}
	for _, req := range requests {
		m.queueCommunitySync(ctx, req)
	}
}

func (m *MetaClient) communitySyncLoop(ctx context.Context) {
	log := m.UserLogin.Log.With().Str("action", "community selective sync").Logger()
	ctx = log.WithContext(ctx)
	retry := time.NewTicker(communitySyncRetryPeriod)
	defer retry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-retry.C:
			m.retryPendingCommunitySyncs(ctx)
		case req := <-m.communitySyncQueue:
			m.handleCommunitySyncRequest(ctx, req)
		case <-m.communitySweepWake:
			if flags := m.communitySweepFlags.Swap(0); flags&communitySweepRequested != 0 {
				m.selectiveSyncAllThreads(ctx, flags&communitySweepAll == 0, flags&communitySweepMarkStale != 0)
			}
		}
	}
}

func (m *MetaClient) handleCommunitySyncRequest(ctx context.Context, req communitySyncRequest) {
	if req.Hole != nil {
		m.requestCommunityThreadHole(ctx, req)
	} else {
		m.selectiveSyncThread(ctx, req.ThreadKey, false)
	}
}

func (m *MetaClient) drainCommunitySyncQueue(ctx context.Context) {
	for {
		select {
		case req := <-m.communitySyncQueue:
			m.handleCommunitySyncRequest(ctx, req)
		default:
			return
		}
	}
}

func (m *MetaClient) isBridgedSelectiveSyncThread(ctx context.Context, threadKey int64) bool {
	portal, err := m.Main.Bridge.GetExistingPortalByKey(ctx, m.makeFBPortalKey(threadKey, table.COMMUNITY_GROUP))
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Int64("thread_id", threadKey).Msg("Failed to get portal")
		return false
	}
	return portal != nil && portal.MXID != "" && isSelectiveSyncThread(portal.Metadata.(*metaid.PortalMetadata).ThreadType)
}

func (m *MetaClient) retryPendingCommunitySyncs(ctx context.Context) {
	var reqs []communitySyncRequest
	m.communitySyncLock.Lock()
	for threadKey, st := range m.communitySync {
		if !st.Pending || time.Since(st.LastRequested) < communitySyncMinInterval {
			continue
		}
		reqs = append(reqs, communitySyncRequest{ThreadKey: threadKey, Hole: st.PendingHole, PrevSeqID: st.PendingHolePrev})
		st.Pending = false
		st.PendingHole = nil
		st.PendingHolePrev = 0
		m.communitySync[threadKey] = st
	}
	m.communitySyncLock.Unlock()
	for _, req := range reqs {
		m.handleCommunitySyncRequest(ctx, req)
	}
}

func (m *MetaClient) deferCommunitySync(req communitySyncRequest) {
	m.communitySyncLock.Lock()
	defer m.communitySyncLock.Unlock()
	st := m.communitySync[req.ThreadKey]
	st.Pending = true
	if req.Hole != nil {
		if st.PendingHole == nil {
			st.PendingHole = req.Hole
			st.PendingHolePrev = req.PrevSeqID
		} else {
			st.PendingHolePrev = min(st.PendingHolePrev, req.PrevSeqID)
			if req.Hole.ThreadQueueSequenceID > st.PendingHole.ThreadQueueSequenceID {
				st.PendingHole = req.Hole
			}
		}
	}
	m.communitySync[req.ThreadKey] = st
}

func (m *MetaClient) requestCommunityThreadHole(ctx context.Context, req communitySyncRequest) {
	if !m.isBridgedSelectiveSyncThread(ctx, req.ThreadKey) {
		return
	} else if !m.claimCommunitySync(req.ThreadKey) {
		m.deferCommunitySync(req)
		return
	}
	log := zerolog.Ctx(ctx).With().
		Int64("thread_id", req.ThreadKey).
		Int64("previous_tq_seq_id", req.PrevSeqID).
		Int64("current_tq_seq_id", req.Hole.ThreadQueueSequenceID).
		Logger()
	resp, err := m.Client.ExecuteTasks(ctx, &socket.CommunityThreadHoleDetection{
		ThreadID:        req.ThreadKey,
		PreviousTQSeqID: req.PrevSeqID,
		CurrentTQSeqID:  req.Hole.ThreadQueueSequenceID,
		DeltaType:       req.Hole.DeltaType,
	})
	if err != nil {
		log.Err(err).Msg("Failed to request missing community thread messages")
		return
	}
	log.Debug().Strs("response_table_fields", resp.NonNilFields()).Msg("Requested missing community thread messages")
	m.queueCommunitySyncResponse(ctx, resp)
}

func (m *MetaClient) selectiveSyncAllThreads(ctx context.Context, onlyOutdated, markStale bool) {
	log := zerolog.Ctx(ctx)
	userPortals, err := m.Main.Bridge.DB.UserPortal.GetAllForLogin(ctx, m.UserLogin.UserLogin)
	if err != nil {
		log.Err(err).Msg("Failed to get portals")
		return
	}
	cutoff := time.Now().Add(-communitySyncStaleAfter).UnixMilli()
	var count int
	for _, up := range userPortals {
		portal, err := m.Main.Bridge.GetExistingPortalByKey(ctx, up.Portal)
		if err != nil {
			log.Err(err).Msg("Failed to get portal")
			continue
		} else if portal == nil || portal.MXID == "" || !isSelectiveSyncThread(portal.Metadata.(*metaid.PortalMetadata).ThreadType) {
			continue
		}
		threadKey := metaid.ParseFBPortalID(portal.ID)
		m.communitySyncLock.Lock()
		st := m.communitySync[threadKey]
		if markStale {
			st.Stale = true
			m.communitySync[threadKey] = st
		}
		m.communitySyncLock.Unlock()
		if onlyOutdated && !st.Stale && st.LastSyncTimestampMs > cutoff {
			continue
		}
		m.drainCommunitySyncQueue(ctx)
		if m.selectiveSyncThread(ctx, threadKey, true) {
			count++
			select {
			case <-time.After(communitySyncRequestDelay):
			case <-ctx.Done():
				return
			}
		}
	}
	if count > 0 {
		log.Debug().Int("thread_count", count).Msg("Selective synced community threads")
	}
}

func (m *MetaClient) selectiveSyncThread(ctx context.Context, threadKey int64, portalChecked bool) bool {
	if threadKey == 0 || (!portalChecked && !m.isBridgedSelectiveSyncThread(ctx, threadKey)) {
		return false
	}
	if !m.claimCommunitySync(threadKey) {
		m.deferCommunitySync(communitySyncRequest{ThreadKey: threadKey})
		return false
	}
	m.communitySyncLock.Lock()
	st := m.communitySync[threadKey]
	m.communitySyncLock.Unlock()
	log := zerolog.Ctx(ctx).With().
		Int64("thread_id", threadKey).
		Bool("is_stale", st.Stale).
		Int64("client_seq_id", st.SequenceID).
		Logger()
	task := &socket.ReportThreadViewOpenTask{
		ThreadKey:   threadKey,
		IsStale:     st.Stale,
		ClientSeqID: st.SequenceID,
	}
	if st.LastSyncTimestampMs != 0 {
		task.ClientLastSyncTimestampMs = &st.LastSyncTimestampMs
	}
	resp, err := m.Client.ExecuteTasks(ctx, task)
	if err != nil {
		log.Err(err).Msg("Failed to selective sync community thread")
		return true
	}
	log.Debug().Strs("response_table_fields", resp.NonNilFields()).Msg("Selective synced community thread")
	staleAgain := false
	for _, upd := range resp.LSUpdateCommunityThreadStaleState {
		if upd.ThreadKey == threadKey {
			staleAgain = upd.IsStale
		}
	}
	if m.queueCommunitySyncResponse(ctx, resp) {
		m.communitySyncLock.Lock()
		st = m.communitySync[threadKey]
		st.LastSyncTimestampMs = max(st.LastSyncTimestampMs, time.Now().UnixMilli())
		st.Stale = staleAgain
		m.communitySync[threadKey] = st
		m.communitySyncLock.Unlock()
	}
	return true
}

func (m *MetaClient) claimCommunitySync(threadKey int64) bool {
	m.backfillLock.Lock()
	_, collecting := m.backfillCollectors[threadKey]
	m.backfillLock.Unlock()
	if collecting {
		return false
	}
	m.communitySyncLock.Lock()
	defer m.communitySyncLock.Unlock()
	st := m.communitySync[threadKey]
	if time.Since(st.LastRequested) < communitySyncMinInterval {
		return false
	}
	st.LastRequested = time.Now()
	m.communitySync[threadKey] = st
	return true
}

func (m *MetaClient) queueCommunitySyncResponse(ctx context.Context, resp *table.LSTable) bool {
	if resp == nil || ctx.Err() != nil {
		return false
	}
	resp.LSMarkJoinedCommunityThreadsStale = nil
	m.parseAndQueueTable(ctx, resp, false)
	return true
}
