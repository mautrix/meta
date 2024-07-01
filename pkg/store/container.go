package store

import (
	"context"

	"go.mau.fi/util/dbutil"

	"go.mau.fi/mautrix-meta/pkg/store/upgrades"
)

type Container struct {
	db *dbutil.Database
}

func NewStore(db *dbutil.Database, log dbutil.DatabaseLogger) *Container {
	return &Container{db: db.Child("meta_version", upgrades.Table, log)}
}

func (c *Container) Upgrade(ctx context.Context) error {
	return c.db.Upgrade(ctx)
}

func newMetaSession(qh *dbutil.QueryHelper[*MetaSession]) *MetaSession {
	return &MetaSession{}
}

func (c *Container) GetSessionQuery() *MetaSessionQuery {
	return &MetaSessionQuery{dbutil.MakeQueryHelper(c.db, newMetaSession)}
}
