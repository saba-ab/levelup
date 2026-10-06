-- +goose Up
-- Schema analytics_svc. A pure projection of other modules' facts; tenant_id
-- and player_id are bare uuids (P5). Days are UTC calendar days.

-- Idempotency under at-least-once delivery: one row per applied envelope,
-- inserted in the same transaction as the projection writes.
CREATE TABLE applied_events (
    event_id   TEXT PRIMARY KEY,
    tenant_id  UUID NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX ix_applied_events_applied_at ON applied_events (applied_at);
CREATE INDEX ix_applied_events_tenant ON applied_events (tenant_id);

-- Additive per-day counters. dimension is '' when a metric has no breakdown.
CREATE TABLE daily_counters (
    tenant_id UUID   NOT NULL,
    metric    TEXT   NOT NULL,
    day       DATE   NOT NULL,
    dimension TEXT   NOT NULL DEFAULT '',
    value     BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, metric, day, dimension)
);

CREATE INDEX ix_daily_counters_day ON daily_counters (day);

-- A player is active on a day when they had at least one activity.
CREATE TABLE player_days (
    tenant_id UUID NOT NULL,
    day       DATE NOT NULL,
    player_id UUID NOT NULL,
    PRIMARY KEY (tenant_id, day, player_id)
);

CREATE INDEX ix_player_days_player ON player_days (tenant_id, player_id, day);
CREATE INDEX ix_player_days_day ON player_days (day);

-- Activity days per event type: the funnel source.
CREATE TABLE player_event_days (
    tenant_id  UUID NOT NULL,
    event_type TEXT NOT NULL,
    day        DATE NOT NULL,
    player_id  UUID NOT NULL,
    PRIMARY KEY (tenant_id, event_type, day, player_id)
);

CREATE INDEX ix_player_event_days_player ON player_event_days (tenant_id, event_type, player_id, day);
CREATE INDEX ix_player_event_days_day ON player_event_days (day);

-- First activity day per player (retention cohorts). Upserted with LEAST so
-- reordered deliveries converge on the earliest day.
CREATE TABLE player_first_seen (
    tenant_id UUID NOT NULL,
    player_id UUID NOT NULL,
    first_day DATE NOT NULL,
    PRIMARY KEY (tenant_id, player_id)
);

CREATE INDEX ix_player_first_seen_day ON player_first_seen (tenant_id, first_day);

CREATE TABLE reconcile_markers (
    job      TEXT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE player_first_seen;
DROP TABLE player_event_days;
DROP TABLE player_days;
DROP TABLE daily_counters;
DROP TABLE applied_events;
