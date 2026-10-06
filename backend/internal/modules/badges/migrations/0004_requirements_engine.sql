-- +goose Up
-- Version 4: 2 and 3 are the Go seeds (permissions, default grants).
-- The requirements engine: badges' own per-player projection of other
-- modules' facts, evaluated against badges.requirements. tenant_id and
-- player_id are bare uuids (P5).

CREATE TABLE badge_player_stats (
    tenant_id          UUID NOT NULL,
    player_id          UUID NOT NULL,
    -- points' lifetime_earned (absolute); lifetime_points_at is the ledger
    -- time of the value, so an older event arriving late never wins.
    lifetime_points    BIGINT NOT NULL DEFAULT 0,
    lifetime_points_at TIMESTAMPTZ,
    missions_completed BIGINT NOT NULL DEFAULT 0,
    max_streak         BIGINT NOT NULL DEFAULT 0,
    level              BIGINT NOT NULL DEFAULT 0,
    badges_earned      BIGINT NOT NULL DEFAULT 0,
    updated_at         TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, player_id)
);

CREATE TABLE badge_player_activity_counts (
    tenant_id  UUID NOT NULL,
    player_id  UUID NOT NULL,
    event_type TEXT NOT NULL,
    count      BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, player_id, event_type)
);

-- Dedupe keys of the increments (mission attempt, award, activity), written
-- in the same tx as the increment. Pruned by badges.prune_applied_events.
CREATE TABLE applied_events (
    tenant_id  UUID NOT NULL,
    event_key  TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, event_key)
);
CREATE INDEX idx_applied_events_applied_at ON applied_events (applied_at);

-- Badges the engine evaluates on every fact.
CREATE INDEX idx_badges_auto ON badges (tenant_id, created_at, id)
    WHERE requirements IS NOT NULL AND is_active AND deleted_at IS NULL;

-- GET /badges/stats.
CREATE INDEX idx_badge_awards_tenant_applied ON badge_awards (tenant_id, created_at) WHERE status = 'applied';
CREATE INDEX idx_badge_awards_tenant_badge_applied ON badge_awards (tenant_id, badge_id) WHERE status = 'applied';

-- +goose Down
DROP INDEX idx_badge_awards_tenant_badge_applied;
DROP INDEX idx_badge_awards_tenant_applied;
DROP INDEX idx_badges_auto;
DROP TABLE applied_events;
DROP TABLE badge_player_activity_counts;
DROP TABLE badge_player_stats;
