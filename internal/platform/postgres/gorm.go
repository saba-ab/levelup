package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// GormBase opens GORM over the shared writer pool. PgBouncer-safe by
// construction (PRD §15): no statement cache, no implicit per-write
// transaction. log may be nil (tests).
func (d *DB) GormBase(log *zap.Logger) (*gorm.DB, error) {
	sqlDB := stdlib.OpenDBFromPool(d.writer)
	return gorm.Open(gormpg.New(gormpg.Config{Conn: sqlDB}), &gorm.Config{
		PrepareStmt:            false, // "prepared statement already exists" under PgBouncer otherwise
		SkipDefaultTransaction: true,  // transactions are explicit (ADR-0013)
		Logger:                 newGormLogger(log),
	})
}

// NewModuleDB mints a module-scoped session (PRD §7.1 Mechanism 1): every
// table name resolves as <module>_svc.<table>, so a cross-module Preload
// resolves to a table that does not exist and fails in the first integration
// test instead of silently working until extraction day.
func NewModuleDB(base *gorm.DB, module string) *gorm.DB {
	db, err := gorm.Open(gormpg.New(gormpg.Config{
		Conn: base.ConnPool, // shared *sql.DB — one pool, many sessions
	}), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   module + "_svc.",
			SingularTable: false,
		},
		PrepareStmt:            false,
		SkipDefaultTransaction: true,
		Logger:                 base.Logger,
	})
	if err != nil {
		// base was already opened over a live pool; re-opening over the same
		// ConnPool cannot fail at runtime. Treat it as programmer error.
		panic(err)
	}
	return db
}

// newGormLogger adapts GORM's logger to zap so slow queries (>200ms, R14)
// land in the same stream as everything else.
func newGormLogger(log *zap.Logger) gormlogger.Interface {
	if log == nil {
		return gormlogger.Discard
	}
	return &zapGormLogger{log: log, slow: 200 * time.Millisecond}
}

type zapGormLogger struct {
	log  *zap.Logger
	slow time.Duration
}

func (l *zapGormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }

func (l *zapGormLogger) Info(_ context.Context, msg string, args ...any) {
	l.log.Sugar().Infof(msg, args...)
}

func (l *zapGormLogger) Warn(_ context.Context, msg string, args ...any) {
	l.log.Sugar().Warnf(msg, args...)
}

func (l *zapGormLogger) Error(_ context.Context, msg string, args ...any) {
	l.log.Sugar().Errorf(msg, args...)
}

func (l *zapGormLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		l.log.Error("query failed", zap.String("sql", sql), zap.Int64("rows", rows), zap.Error(err))
	case elapsed > l.slow:
		sql, rows := fc()
		l.log.Warn("slow query", zap.String("sql", sql), zap.Int64("rows", rows),
			zap.Duration("elapsed", elapsed))
	}
}
