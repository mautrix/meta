package connector

import (
	"context"
	"time"

	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (m *MetaClient) resetMessengerBackfill(ctx context.Context, portalKey networkid.PortalKey) (bool, error) {
	task, err := m.Main.Bridge.DB.BackfillTask.GetNextForPortal(ctx, portalKey, true)
	if err != nil {
		return false, err
	} else if task != nil && task.UserLoginID != "" && task.UserLoginID != m.UserLogin.ID {
		return false, nil
	}
	if task == nil {
		task = &database.BackfillTask{PortalKey: portalKey, BatchCount: -1}
	}
	task.UserLoginID = m.UserLogin.ID
	task.Cursor = ""
	task.IsDone = false
	task.QueueDone = false
	task.DispatchedAt = time.Time{}
	task.CompletedAt = time.Time{}
	task.NextDispatchMinTS = time.Now()
	err = m.Main.Bridge.DB.BackfillTask.Upsert(ctx, task)
	return err == nil, err
}

func (m *MetaClient) storeThreadCutover(ctx context.Context, cutover *table.LSCreateOpenToE2EEThreadLink) error {
	var reopened bool
	err := m.Main.DB.DoTxn(ctx, nil, func(ctx context.Context) error {
		previous, err := m.Main.DB.GetOpenThreadKey(ctx, m.UserLogin.ID, cutover.ArmadilloThreadID)
		if err != nil {
			return err
		}
		if err = m.Main.DB.PutThreadCutover(ctx, m.UserLogin.ID, cutover.ArmadilloThreadID, cutover.OpenThreadID, cutover.ShowOpenMessageHistory); err != nil {
			return err
		} else if !cutover.ShowOpenMessageHistory || previous == cutover.OpenThreadID {
			return nil
		}
		portal, err := m.Main.Bridge.DB.Portal.GetByIDWithUncertainReceiver(ctx, m.makeFBPortalKey(cutover.ArmadilloThreadID, table.UNKNOWN_THREAD_TYPE))
		if err != nil {
			return err
		} else if portal == nil || portal.MXID == "" || !portal.Metadata.(*metaid.PortalMetadata).ThreadType.IsWhatsApp() {
			return nil
		}
		reopened, err = m.resetMessengerBackfill(ctx, portal.PortalKey)
		return err
	})
	if err == nil && reopened {
		m.Main.Bridge.WakeupBackfillQueue()
	}
	return err
}
