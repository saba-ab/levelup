-- +goose Up
-- Schema leaderboards_svc. Postgres is the source of truth for scores; the
-- redis-core sorted sets are a derived read model rebuilt from these tables
-- (00-target-architecture D11). player_id / program_id / tenant_id are bare
-- uuids: no foreign keys into other modules' schemas (P5).

CREATE TABLE leaderboards (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    legacy_id       BIGINT UNIQUE,
    slug            TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    type            TEXT NOT NULL CHECK (type IN ('points', 'badges', 'missions', 'xp')),
    metric          TEXT NOT NULL CHECK (metric IN ('earned', 'net', 'balance', 'count')),
    reset_frequency TEXT NOT NULL CHECK (reset_frequency IN ('never', 'daily', 'weekly', 'monthly')),
    -- NULL = every player of the tenant; otherwise only enrolled players count.
    program_id      UUID,
    max_entries     INT NOT NULL DEFAULT 100 CHECK (max_entries BETWEEN 1 AND 1000),
    is_active       BOOLEAN NOT NULL DEFAULT true,
    version         INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    deleted_at      TIMESTAMPTZ,
    CHECK (metric <> 'balance' OR reset_frequency = 'never')
);

-- Slugs are unique per tenant among live boards (fixes L2's global unique).
CREATE UNIQUE INDEX ux_leaderboards_tenant_slug ON leaderboards (tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX ix_leaderboards_tenant_type ON leaderboards (tenant_id, type) WHERE deleted_at IS NULL AND is_active;
CREATE INDEX ix_leaderboards_tenant_created ON leaderboards (tenant_id, created_at DESC, id DESC);

-- One row per (board, period) that ever received a score: the list of
-- read-model keys (no KEYS/SCAN needed) and the rollover work queue.
CREATE TABLE leaderboard_periods (
    leaderboard_id UUID NOT NULL REFERENCES leaderboards (id) ON DELETE CASCADE,
    period_start   TIMESTAMPTZ NOT NULL,
    tenant_id      UUID NOT NULL,
    period_end     TIMESTAMPTZ,          -- NULL for never-resetting boards
    closed_at      TIMESTAMPTZ,          -- set once, by leaderboards.rollover
    PRIMARY KEY (leaderboard_id, period_start)
);

CREATE INDEX ix_leaderboard_periods_due ON leaderboard_periods (period_end)
    WHERE closed_at IS NULL AND period_end IS NOT NULL;
CREATE INDEX ix_leaderboard_periods_tenant ON leaderboard_periods (tenant_id);

CREATE TABLE leaderboard_scores (
    leaderboard_id UUID NOT NULL,
    period_start   TIMESTAMPTZ NOT NULL,
    player_id      UUID NOT NULL,
    tenant_id      UUID NOT NULL,
    score          BIGINT NOT NULL DEFAULT 0,
    -- Time of the newest fact applied; guards balance metrics so an older
    -- balance_after delivered late cannot overwrite a newer one.
    last_event_at  TIMESTAMPTZ,
    updated_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (leaderboard_id, period_start, player_id),
    FOREIGN KEY (leaderboard_id, period_start)
        REFERENCES leaderboard_periods (leaderboard_id, period_start) ON DELETE CASCADE
);

-- Ranking order: score desc, then player id asc for deterministic ties.
CREATE INDEX ix_leaderboard_scores_rank ON leaderboard_scores (leaderboard_id, period_start, score DESC, player_id);
CREATE INDEX ix_leaderboard_scores_player ON leaderboard_scores (tenant_id, player_id);

-- Final standings of closed periods.
CREATE TABLE leaderboard_snapshots (
    leaderboard_id UUID NOT NULL,
    period_start   TIMESTAMPTZ NOT NULL,
    player_id      UUID NOT NULL,
    tenant_id      UUID NOT NULL,
    rank           BIGINT NOT NULL,
    score          BIGINT NOT NULL,
    closed_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (leaderboard_id, period_start, player_id)
);

CREATE INDEX ix_leaderboard_snapshots_rank ON leaderboard_snapshots (leaderboard_id, period_start, rank);
CREATE INDEX ix_leaderboard_snapshots_tenant ON leaderboard_snapshots (tenant_id);

-- Idempotency of score increments under at-least-once delivery: one row per
-- (event, board), inserted in the same transaction as the score upsert.
CREATE TABLE applied_events (
    event_id       TEXT NOT NULL,
    leaderboard_id UUID NOT NULL,
    applied_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (event_id, leaderboard_id)
);

CREATE INDEX ix_applied_events_applied_at ON applied_events (applied_at);

-- Projection of program.player_enrolled/unenrolled.v1 (last writer by event
-- time wins, so reordering cannot resurrect a stale state).
CREATE TABLE program_members (
    program_id UUID NOT NULL,
    player_id  UUID NOT NULL,
    tenant_id  UUID NOT NULL,
    enrolled   BOOLEAN NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (program_id, player_id)
);

CREATE INDEX ix_program_members_tenant ON program_members (tenant_id);

-- Projection of player.deactivated/activated/deleted.v1. Hidden players
-- keep their scores but are excluded from every ranking.
CREATE TABLE hidden_players (
    tenant_id   UUID NOT NULL,
    player_id   UUID NOT NULL,
    deactivated BOOLEAN NOT NULL DEFAULT false,
    deleted     BOOLEAN NOT NULL DEFAULT false,
    changed_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, player_id)
);

CREATE TABLE reconcile_markers (
    job      TEXT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE hidden_players;
DROP TABLE program_members;
DROP TABLE applied_events;
DROP TABLE leaderboard_snapshots;
DROP TABLE leaderboard_scores;
DROP TABLE leaderboard_periods;
DROP TABLE leaderboards;
