// Package outbox is the only path from a business transaction to RabbitMQ
// (PRD §4 P3). Publish writes the event into the caller's transaction; the
// dispatcher (dispatcher.go) drains confirmed rows to the broker. A service
// that publishes to the broker directly has re-created the dual-write bug
// this package exists to prevent.
package outbox

import (
	"context"

	"gorm.io/gorm"
)

// Store records events atomically with the state change that caused them.
// tx must be the transaction from postgres.InTx — never a bare handle
// (ADR-0013: transactions are explicit arguments).
type Store interface {
	Publish(ctx context.Context, tx *gorm.DB, topic string, payload any) error
}
