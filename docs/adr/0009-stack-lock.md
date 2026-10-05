# ADR-0009: The stack is locked

**Status:** Accepted · 2026-09-01

## Context
Boring by default (PRD §4 P6). Every addition is a future migration.

## Decision
chi v5 · GORM+pgx · google/wire · goose v3 · validator/v10 · caarlos0/env/v11 · zap ·
golang-jwt/v5 · casbin v2 (+gorm-adapter, redis-watcher) · go-redis v9 + redis_rate ·
amqp091-go · robfig/cron v3 · OTel + prometheus client · swaggo/swag ·
testify/mockery/testcontainers-go/k6 · golangci-lint · go-task · air (PRD §7).
Additions require an ADR. Additions made so far: sony/gobreaker (ADR-0014),
golang.org/x/tools (arch tests, test-only).

## Consequences
Where a chosen tool makes a §4 principle easy to violate (GORM above all), containment
is mandatory and mechanical: per-module TablePrefix, unexported models, AutoMigrate
banned by arch test (PRD §7.1).
