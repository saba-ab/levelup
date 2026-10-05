package postgres

import (
	"context"

	"gorm.io/gorm"

	"myapp/internal/shared/errs"
)

// InTx runs fn in one database transaction (ADR-0013): the tx handle is an
// explicit argument everywhere, never smuggled through context. Nesting is
// rejected outright — GORM would silently downgrade the inner call to a
// savepoint, and a "transaction" that can partially survive its parent's
// rollback is exactly the ambiguity the ban on mixing styles exists to kill.
func InTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return errs.New(errs.Internal, "nested InTx call — restructure the caller to pass the outer tx")
	}
	return db.WithContext(ctx).Transaction(fn)
}
