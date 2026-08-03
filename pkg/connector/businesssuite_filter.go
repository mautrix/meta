package connector

import (
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

const businessSuiteInstagramSyncGroup int64 = 127

// rememberBusinessSuiteThreadChannels records the channel identity carried by
// Business Suite's unified snapshot. The sync group alone is not sufficient:
// Meta can return Instagram threads in group 127 and marks them separately with
// deleteThenInsertIgThreadInfo.
func (m *MetaClient) rememberBusinessSuiteThreadChannels(tbl *table.LSTable) {
	if m.LoginMeta == nil || m.LoginMeta.Platform != types.BusinessSuite || tbl == nil {
		return
	}
	for _, thread := range tbl.LSDeleteThenInsertThread {
		if thread.SyncGroup == businessSuiteInstagramSyncGroup {
			m.businessSuiteInstagramThreads.Store(thread.ThreadKey, struct{}{})
		}
	}
	for _, thread := range tbl.LSUpdateOrInsertThread {
		if thread.SyncGroup == businessSuiteInstagramSyncGroup {
			m.businessSuiteInstagramThreads.Store(thread.ThreadKey, struct{}{})
		}
	}
	for _, info := range tbl.LSDeleteThenInsertIgThreadInfo {
		m.businessSuiteInstagramThreads.Store(info.ThreadKey, struct{}{})
	}
}

func (m *MetaClient) shouldIgnoreBusinessSuiteThread(threadKey int64) bool {
	if m.LoginMeta == nil || m.LoginMeta.Platform != types.BusinessSuite {
		return false
	}
	_, ignored := m.businessSuiteInstagramThreads.Load(threadKey)
	return ignored
}
