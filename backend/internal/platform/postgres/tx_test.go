package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
)

type txItem struct {
	ID   int64 `gorm:"primaryKey"`
	Name string
}

func (txItem) TableName() string { return "tx_svc.items" }

func setupTxDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := pgtest.DSN(t)
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "tx", migrationsFS(t)))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	gdb, err := db.GormBase(nil)
	require.NoError(t, err)
	return gdb
}

func TestInTxCommits(t *testing.T) {
	gdb := setupTxDB(t)

	err := postgres.InTx(context.Background(), gdb, func(tx *gorm.DB) error {
		return tx.Create(&txItem{Name: "kept"}).Error
	})
	require.NoError(t, err)

	var n int64
	require.NoError(t, gdb.Model(&txItem{}).Where("name = ?", "kept").Count(&n).Error)
	require.EqualValues(t, 1, n)
}

func TestInTxRollsBackOnError(t *testing.T) {
	gdb := setupTxDB(t)
	boom := errors.New("boom")

	err := postgres.InTx(context.Background(), gdb, func(tx *gorm.DB) error {
		if err := tx.Create(&txItem{Name: "doomed"}).Error; err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)

	var n int64
	require.NoError(t, gdb.Model(&txItem{}).Where("name = ?", "doomed").Count(&n).Error)
	require.Zero(t, n, "rollback must leave no row (R4 precondition)")
}

func TestInTxRejectsNesting(t *testing.T) {
	gdb := setupTxDB(t)

	err := postgres.InTx(context.Background(), gdb, func(tx *gorm.DB) error {
		return postgres.InTx(context.Background(), tx, func(*gorm.DB) error { return nil })
	})
	require.Error(t, err, "nested InTx means a hidden savepoint — banned (ADR-0013)")
	require.Contains(t, err.Error(), "nested")
}
