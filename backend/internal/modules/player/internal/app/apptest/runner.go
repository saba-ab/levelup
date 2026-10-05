package apptest

import (
	"context"

	"gorm.io/gorm"
)

// Runner is a pass-through transaction runner that marks the TxState open
// for the duration of fn, so fakes can prove what happened inside the tx.
func Runner(state *TxState) func(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return func(_ context.Context, fn func(tx *gorm.DB) error) error {
		state.Set(true)
		defer state.Set(false)
		return fn(nil)
	}
}
