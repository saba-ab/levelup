-- +goose Up
-- Lands in schema streaks_svc. No foreign keys to other modules' schemas
-- (P5): tenant_id and player_id are bare uuids.

-- Period starts (period_start, last_period_start, run_started_at, run_floor)
-- are the CIVIL start date of a bucket in the tenant's timezone, stored as
-- midnight UTC of that date (domain.Period.Start).

CREATE TABLE streaks (
    id                UUID PRIMARY KEY,
    tenant_id         UUID NOT NULL,
    slug              TEXT NOT NULL,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    activity_key      TEXT NOT NULL,
    period            TEXT NOT NULL CHECK (period IN ('daily', 'weekly', 'monthly')),
    grace_periods     INT NOT NULL DEFAULT 0 CHECK (grace_periods >= 0),
    points_per_period BIGINT NOT NULL DEFAULT 0 CHECK (points_per_period >= 0),
    milestones        JSONB NOT NULL DEFAULT '[]'::jsonb, -- [{"count": 7, "bonus_points": 100}]
    is_active         BOOLEAN NOT NULL DEFAULT true,
    version           INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,
    deleted_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX ux_streaks_tenant_slug ON streaks (tenant_id, slug) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX ux_streaks_tenant_activity_key ON streaks (tenant_id, activity_key) WHERE deleted_at IS NULL;
CREATE INDEX ix_streaks_tenant_created ON streaks (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;

CREATE TABLE player_streaks (
    id                    UUID PRIMARY KEY,
    tenant_id             UUID NOT NULL,
    player_id             UUID NOT NULL,
    streak_id             UUID NOT NULL REFERENCES streaks (id) ON DELETE CASCADE,
    current_count         INT NOT NULL DEFAULT 0,
    longest_count         INT NOT NULL DEFAULT 0,
    last_period_start     TIMESTAMPTZ,
    run_started_at        TIMESTAMPTZ,
    run_floor             TIMESTAMPTZ, -- admin reset: buckets at or before it never count again
    broken_at             TIMESTAMPTZ,
    broken_run_started_at TIMESTAMPTZ, -- the run streaks.broken.v1 was last published for
    version               INT NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL,
    CONSTRAINT ux_player_streaks_player_streak UNIQUE (player_id, streak_id)
);

CREATE INDEX ix_player_streaks_tenant_player ON player_streaks (tenant_id, player_id);
-- Break sweep: live runs per streak ordered by their newest bucket.
CREATE INDEX ix_player_streaks_sweep ON player_streaks (streak_id, last_period_start) WHERE current_count > 0;

-- One row per recorded period: the unique key is what makes a second record
-- in the same period a no-op.
CREATE TABLE streak_periods (
    id               UUID PRIMARY KEY,
    tenant_id        UUID NOT NULL,
    player_streak_id UUID NOT NULL REFERENCES player_streaks (id) ON DELETE CASCADE,
    period_start     TIMESTAMPTZ NOT NULL,
    idempotency_key  TEXT NOT NULL,
    recorded_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT ux_streak_periods_bucket UNIQUE (player_streak_id, period_start)
);

CREATE INDEX ix_streak_periods_tenant ON streak_periods (tenant_id);

-- A milestone pays once per run; a run is identified by its first bucket.
CREATE TABLE streak_milestone_awards (
    id               UUID PRIMARY KEY,
    tenant_id        UUID NOT NULL,
    player_streak_id UUID NOT NULL REFERENCES player_streaks (id) ON DELETE CASCADE,
    milestone        INT NOT NULL,
    bonus_points     BIGINT NOT NULL DEFAULT 0,
    run_started_at   TIMESTAMPTZ NOT NULL,
    awarded_at       TIMESTAMPTZ NOT NULL,
    CONSTRAINT ux_streak_milestone_awards_run UNIQUE (player_streak_id, milestone, run_started_at)
);

CREATE INDEX ix_streak_milestone_awards_tenant ON streak_milestone_awards (tenant_id);

-- Idempotency of record commands (HTTP and job.streaks.record).
CREATE TABLE record_requests (
    tenant_id       UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    player_id       TEXT NOT NULL,
    streak_id       TEXT NOT NULL DEFAULT '',
    activity_key    TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL CHECK (status IN ('applied', 'rejected')),
    reason          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, idempotency_key)
);

CREATE INDEX ix_record_requests_created ON record_requests (created_at);

CREATE TABLE reconcile_markers (
    job      TEXT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE record_requests;
DROP TABLE streak_milestone_awards;
DROP TABLE streak_periods;
DROP TABLE player_streaks;
DROP TABLE streaks;
