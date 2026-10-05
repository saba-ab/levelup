// Package integration holds cross-cutting tests that exercise several
// platform pieces against real containers (R8, R39, R46).
package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"myapp/internal/platform/postgres"
	"myapp/internal/platform/postgres/pgtest"
)

// Mirror shapes a hostile module might write: a wallet row trying to join
// straight to another module's table — idiomatic GORM, and exactly the
// cross-module coupling §7.1 exists to kill.
type userMirror struct {
	ID    string `gorm:"primaryKey;type:uuid"`
	Email string
}

type walletRow struct {
	ID     string     `gorm:"primaryKey;type:uuid"`
	UserID string     `gorm:"type:uuid"`
	User   userMirror `gorm:"foreignKey:UserID"`
}

func (walletRow) TableName() string { return "walletx_svc.wallet_rows" }

// R8: a cross-module Preload from a wallet-scoped session resolves the other
// module's table INSIDE wallet's schema — which does not exist — and fails
// at runtime in the first integration test rather than silently working
// until extraction day (PRD §7.1 Mechanism 1).
func TestCrossModulePreloadFailsAtRuntime(t *testing.T) {
	dsn := pgtest.DSN(t)
	ctx := context.Background()

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	base, err := db.GormBase(nil)
	require.NoError(t, err)

	// Wallet's own table exists, holding a bare uuid (P5, no FK).
	require.NoError(t, base.Exec(`CREATE SCHEMA IF NOT EXISTS walletx_svc`).Error)
	require.NoError(t, base.Exec(`CREATE TABLE IF NOT EXISTS walletx_svc.wallet_rows (
		id UUID PRIMARY KEY, user_id UUID)`).Error)
	// One row must exist: GORM only issues the preload query when the
	// parent result set is non-empty.
	require.NoError(t, base.Exec(`INSERT INTO walletx_svc.wallet_rows VALUES
		('0198cccc-0000-7000-8000-000000000001', '0198cccc-0000-7000-8000-000000000002')
		ON CONFLICT DO NOTHING`).Error)

	walletDB := postgres.NewModuleDB(base, "walletx")

	var rows []walletRow
	err = walletDB.Preload("User").Find(&rows).Error
	require.Error(t, err, "cross-module Preload must fail: the association resolves inside walletx_svc")
	require.Contains(t, err.Error(), "walletx_svc.user_mirrors")
}
