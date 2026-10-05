// Package postgres owns the connection story (PRD §7.1, §15): one shared
// pgx pool pair (writer/reader), a PgBouncer-safe GORM base on top of the
// writer, per-module GORM sessions with schema-pinned table prefixes, and
// the goose migration runner.
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"levelup/internal/shared/errs"
)

// Platform packages take primitives, never config.Config — the composition
// root adapts. This keeps platform importable from module internals without
// cycles and enforces "infrastructure knows no application shape".
type DB struct {
	writer *pgxpool.Pool
	reader *pgxpool.Pool
}

// NewFromDSNs builds the writer/reader pair. Empty readerDSN → reads fall
// back to the writer pool.
func NewFromDSNs(ctx context.Context, writerDSN, readerDSN string, maxConns int32) (*DB, func(), error) {
	writer, err := newPool(ctx, writerDSN, maxConns)
	if err != nil {
		return nil, nil, errs.Wrap(errs.Unavailable, "postgres writer", err)
	}

	reader := writer
	if readerDSN != "" && readerDSN != writerDSN {
		reader, err = newPool(ctx, readerDSN, maxConns)
		if err != nil {
			writer.Close()
			return nil, nil, errs.Wrap(errs.Unavailable, "postgres reader", err)
		}
	}

	db := &DB{writer: writer, reader: reader}
	cleanup := func() {
		if db.reader != db.writer {
			db.reader.Close()
		}
		db.writer.Close()
	}
	return db, cleanup, nil
}

// Writer is the pool for transactions and anything that mutates.
func (d *DB) Writer() *pgxpool.Pool { return d.writer }

// Reader serves read-only paths; behind a replica when one is configured.
func (d *DB) Reader() *pgxpool.Pool { return d.reader }

func newPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	// Pool math (PRD §15): pods × MaxConns must stay under the PgBouncer
	// pool, which must stay under Postgres max_connections.
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	// MANDATORY behind PgBouncer transaction pooling: no prepared-statement
	// cache, plain exec protocol (PRD §15).
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
