package main

import (
	_ "embed"
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
