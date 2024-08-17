package main

import (
	_ "embed"

	up "go.mau.fi/util/configupgrade"
	"maunium.net/go/mautrix/bridgev2/bridgeconfig"
)

const legacyMigrateRenameTables = `
ALTER TABLE portal RENAME TO portal_old;
ALTER TABLE puppet RENAME TO puppet_old;
ALTER TABLE message RENAME TO message_old;
ALTER TABLE reaction RENAME TO reaction_old;
ALTER TABLE "user" RENAME TO user_old;
ALTER TABLE user_portal RENAME TO user_portal_old;
ALTER TABLE backfill_task RENAME TO backfill_task_old;
`

//go:embed legacymigrate.sql
var legacyMigrateCopyData string

func migrateLegacyConfig(helper up.Helper) {
	helper.Set(up.Str, "mautrix.bridge.e2ee", "encryption", "pickle_key")
	bridgeconfig.CopyToOtherLocation(helper, up.Str, []string{"bridge", "displayname_template"}, []string{"network", "displayname_template"})
	bridgeconfig.CopyToOtherLocation(helper, up.Str, []string{"meta", "mode"}, []string{"network", "mode"})
	bridgeconfig.CopyToOtherLocation(helper, up.Bool, []string{"meta", "ig_e2ee"}, []string{"network", "ig_e2ee"})
	bridgeconfig.CopyToOtherLocation(helper, up.Str, []string{"meta", "proxy"}, []string{"network", "proxy"})
	bridgeconfig.CopyToOtherLocation(helper, up.Str, []string{"meta", "get_proxy_from"}, []string{"network", "get_proxy_from"})
	bridgeconfig.CopyToOtherLocation(helper, up.Int, []string{"meta", "min_full_reconnect_interval_seconds"}, []string{"network", "min_full_reconnect_interval_seconds"})
	bridgeconfig.CopyToOtherLocation(helper, up.Int, []string{"meta", "force_refresh_interval_seconds"}, []string{"network", "force_refresh_interval_seconds"})
	bridgeconfig.CopyToOtherLocation(helper, up.Bool, []string{"bridge", "disable_xma"}, []string{"network", "disable_xma_always"})
}
