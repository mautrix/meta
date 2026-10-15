package connector

import (
	"context"
	"time"

	"maunium.net/go/mautrix/bridgev2/database"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

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
		task, err := m.Main.Bridge.DB.BackfillTask.GetNextForPortal(ctx, portal.PortalKey, true)
		if err != nil {
			return err
		} else if task != nil && task.UserLoginID != "" && task.UserLoginID != m.UserLogin.ID {
			return nil
		}
		reopened = true
		return m.Main.Bridge.DB.BackfillTask.Upsert(ctx, &database.BackfillTask{
			PortalKey:         portal.PortalKey,
			UserLoginID:       m.UserLogin.ID,
			NextDispatchMinTS: time.Now(),
		})
	})
	if err == nil && reopened {
		m.Main.Bridge.WakeupBackfillQueue()
	}
	return err
}
