-- +goose Up
-- Lands in schema missions_svc. Table names match GORM's derivation from the
-- unexported models in internal/repo (mission → missions, missionAttempt →
-- mission_attempts, progressEvent → progress_events, reconcileMarker →
-- reconcile_markers). tenant_id / player_id / badge_reward_id are bare uuids:
-- no foreign keys to other modules' schemas (P5).

CREATE TABLE missions (
    id                          UUID PRIMARY KEY,
    tenant_id                   UUID NOT NULL,
    slug                        TEXT NOT NULL,
    name                        TEXT NOT NULL,
    description                 TEXT NOT NULL DEFAULT '',
    type                        TEXT NOT NULL CHECK (type IN ('one_time', 'daily', 'weekly', 'repeating')),
    status                      TEXT NOT NULL DEFAULT 'draft'
                                CHECK (status IN ('draft', 'active', 'paused', 'expired', 'archived')),
    -- Units of progress an attempt must accumulate to complete.
    target                      BIGINT NOT NULL CHECK (target > 0),
    -- Opaque to missions: documents which activities count, e.g.
    -- {"event_type": "purchase_completed", "conditions": [{"field": "amount", "op": ">=", "value": 100}]}.
    -- The rules module evaluates it and sends job.missions.progress.
    criteria                    JSONB NOT NULL DEFAULT '{}'::jsonb,
    points_reward               BIGINT NOT NULL DEFAULT 0 CHECK (points_reward >= 0),
    xp_reward                   BIGINT NOT NULL DEFAULT 0 CHECK (xp_reward >= 0),
    badge_reward_id             UUID,
    -- NULL = unlimited completions per player; one_time ⇒ exactly 1.
    max_completions_per_player  INT CHECK (max_completions_per_player IS NULL OR max_completions_per_player >= 1),
    starts_at                   TIMESTAMPTZ,
    ends_at                     TIMESTAMPTZ,
    version                     INT NOT NULL DEFAULT 0,
    created_at                  TIMESTAMPTZ NOT NULL,
    updated_at                  TIMESTAMPTZ NOT NULL,
    deleted_at                  TIMESTAMPTZ,
    CONSTRAINT missions_one_time_once CHECK (type <> 'one_time' OR max_completions_per_player = 1),
    CONSTRAINT missions_window CHECK (starts_at IS NULL OR ends_at IS NULL OR ends_at > starts_at)
);

CREATE UNIQUE INDEX ux_missions_tenant_slug ON missions (tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX ix_missions_tenant_page ON missions (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
-- The expire sweep's scan.
CREATE INDEX ix_missions_due ON missions (ends_at) WHERE deleted_at IS NULL AND status IN ('active', 'paused');

-- One row per attempt. period_key (UTC):
--   daily  → 'YYYY-MM-DD' (calendar day, UTC)
--   weekly → 'YYYY-Www'   (ISO-8601 week, starts Monday 00:00 UTC)
--   one_time, repeating → 'all'
-- period_ends_at is the exclusive end of the period (NULL for 'all'); the
-- sweep expires open attempts past it.
CREATE TABLE mission_attempts (
    id             UUID PRIMARY KEY,
    tenant_id      UUID NOT NULL,
    mission_id     UUID NOT NULL REFERENCES missions (id),
    player_id      UUID NOT NULL,
    status         TEXT NOT NULL CHECK (status IN ('in_progress', 'completed', 'expired', 'abandoned')),
    progress       BIGINT NOT NULL DEFAULT 0,
    -- Snapshot of missions.target when the attempt started.
    target         BIGINT NOT NULL CHECK (target > 0),
    period_key     TEXT NOT NULL,
    period_ends_at TIMESTAMPTZ,
    started_at     TIMESTAMPTZ NOT NULL,
    completed_at   TIMESTAMPTZ,
    version        INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    CONSTRAINT mission_attempts_progress CHECK (progress >= 0 AND progress <= target),
    CONSTRAINT mission_attempts_completed CHECK ((status = 'completed') = (completed_at IS NOT NULL))
);

-- At most one open attempt per (mission, player, period): find-or-start is
-- INSERT ... ON CONFLICT DO NOTHING against this index.
CREATE UNIQUE INDEX ux_mission_attempts_open
    ON mission_attempts (tenant_id, mission_id, player_id, period_key) WHERE status = 'in_progress';
CREATE INDEX ix_mission_attempts_player ON mission_attempts (tenant_id, player_id, created_at DESC, id DESC);
CREATE INDEX ix_mission_attempts_mission ON mission_attempts (tenant_id, mission_id, created_at DESC, id DESC);
CREATE INDEX ix_mission_attempts_completed
    ON mission_attempts (tenant_id, mission_id, player_id, period_key) WHERE status = 'completed';
CREATE INDEX ix_mission_attempts_open_period ON mission_attempts (period_ends_at) WHERE status = 'in_progress';

-- Idempotency ledger for progress commands (job and HTTP). A redelivered
-- key hits the unique index and changes nothing.
CREATE TABLE progress_events (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    mission_id      UUID NOT NULL,
    player_id       UUID NOT NULL,
    attempt_id      UUID REFERENCES mission_attempts (id),
    increment       BIGINT NOT NULL CHECK (increment > 0),
    status          TEXT NOT NULL CHECK (status IN ('applied', 'rejected')),
    reason          TEXT NOT NULL DEFAULT '',
    source_kind     TEXT NOT NULL DEFAULT '',
    source_id       TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT ux_progress_events_key UNIQUE (tenant_id, idempotency_key)
);

CREATE TABLE reconcile_markers (
    job      TEXT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE progress_events;
DROP TABLE mission_attempts;
DROP TABLE missions;
