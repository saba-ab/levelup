// Package idempotency makes non-GET requests safely retryable (R11):
// Idempotency-Key + request hash + stored response. Postgres, not Redis —
// double-charging on a Redis restart is not an acceptable trade.
package idempotency

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"levelup/internal/shared/errs"
)

type State int

const (
	// Claimed: this request owns the key — run the handler and Complete.
	Claimed State = iota
	// Replay: an identical request already completed — serve the stored response.
	Replay
	// Mismatch: same key, DIFFERENT body — a client bug (R11: 422).
	Mismatch
	// InFlight: a concurrent identical request holds the claim.
	InFlight
)

type Stored struct {
	Status      int
	ContentType string
	Body        []byte
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Claim is claim-first (INSERT … ON CONFLICT DO NOTHING): exactly one of N
// concurrent identical requests wins and executes; the rest see InFlight or,
// later, Replay.
func (s *Store) Claim(ctx context.Context, key string, hash []byte) (State, Stored, error) {
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO idempotency_svc.keys (key, request_hash) VALUES ($1, $2)
		 ON CONFLICT (key) DO NOTHING`, key, hash)
	if err != nil {
		return 0, Stored{}, errs.Wrap(errs.Unavailable, "idempotency claim", err)
	}
	if tag.RowsAffected() == 1 {
		return Claimed, Stored{}, nil
	}

	var existingHash []byte
	var status *int
	var contentType *string
	var body []byte
	err = s.pool.QueryRow(ctx,
		`SELECT request_hash, status, content_type, response
		 FROM idempotency_svc.keys WHERE key = $1`, key).
		Scan(&existingHash, &status, &contentType, &body)
	if err != nil {
		return 0, Stored{}, errs.Wrap(errs.Unavailable, "idempotency lookup", err)
	}
	if string(existingHash) != string(hash) {
		return Mismatch, Stored{}, nil
	}
	if status == nil {
		return InFlight, Stored{}, nil
	}
	ct := ""
	if contentType != nil {
		ct = *contentType
	}
	return Replay, Stored{Status: *status, ContentType: ct, Body: body}, nil
}

// Complete stores the winning request's response for replays.
func (s *Store) Complete(ctx context.Context, key string, res Stored) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE idempotency_svc.keys
		 SET status = $2, content_type = $3, response = $4, completed_at = now()
		 WHERE key = $1`, key, res.Status, res.ContentType, res.Body)
	if err != nil {
		return errs.Wrap(errs.Unavailable, "idempotency complete", err)
	}
	return nil
}

// Release frees a claim whose handler never produced a response (panic,
// timeout): better a retried request than a key wedged in InFlight forever.
func (s *Store) Release(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM idempotency_svc.keys WHERE key = $1 AND status IS NULL`, key)
	return err
}

// Prune drops entries older than the retention window.
func (s *Store) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM idempotency_svc.keys WHERE created_at < now() - $1::interval`,
		olderThan.String())
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "idempotency prune", err)
	}
	return tag.RowsAffected(), nil
}
