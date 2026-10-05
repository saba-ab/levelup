-- +goose Up
-- Lands in schema badges_svc. Table names match GORM's derivation from the
-- unexported models in internal/repo. tenant_id / player_id / awarded_by
-- are bare uuids: no foreign key leaves this schema (P5).

CREATE TABLE badges (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL,
    slug         TEXT NOT NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    icon_url     TEXT NOT NULL DEFAULT '',
    tier         TEXT NOT NULL,
    category     TEXT NOT NULL,
    points_value BIGINT NOT NULL DEFAULT 0 CHECK (points_value >= 0),
    is_stackable BOOLEAN NOT NULL DEFAULT FALSE,
    -- NULL = unlimited; only meaningful for stackable badges.
    max_awards   INT CHECK (max_awards IS NULL OR max_awards >= 1),
    -- Stored and returned verbatim, never evaluated: rules decide awards.
    requirements JSONB,
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    is_secret    BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order   INT NOT NULL DEFAULT 0,
    version      INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    deleted_at   TIMESTAMPTZ
);

-- Slug unique per tenant among live badges (Laravel: globally unique, B18).
CREATE UNIQUE INDEX idx_badges_tenant_slug ON badges (tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_badges_tenant_page ON badges (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;

CREATE TABLE player_badges (
    id               UUID PRIMARY KEY,
    tenant_id        UUID NOT NULL,
    player_id        UUID NOT NULL,
    badge_id         UUID NOT NULL REFERENCES badges (id),
    -- 0 only for the insert-or-lock placeholder inside an award tx; never
    -- committed (the award that creates the row always applies).
    earned_count     INT NOT NULL DEFAULT 0 CHECK (earned_count >= 0),
    first_awarded_at TIMESTAMPTZ NOT NULL,
    last_awarded_at  TIMESTAMPTZ NOT NULL,
    version          INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL
);

-- The lock target of every award: one holding per (player, badge).
CREATE UNIQUE INDEX idx_player_badges_player_badge ON player_badges (player_id, badge_id);
CREATE INDEX idx_player_badges_tenant_player_page ON player_badges (tenant_id, player_id, created_at DESC, id DESC);
CREATE INDEX idx_player_badges_updated ON player_badges (updated_at);
CREATE INDEX idx_player_badges_tenant ON player_badges (tenant_id);

-- Append-only award ledger: one row per award command, applied or rejected.
-- No FK: rejected rows may name a badge that does not exist, and applied
-- rows outlive a revoked holding.
CREATE TABLE badge_awards (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    player_id       UUID NOT NULL,
    badge_id        UUID NOT NULL,
    -- The holding generation the stack was added to (applied only); a
    -- revoke + re-award creates a new holding id, so reconcile counts stay
    -- exact.
    player_badge_id UUID,
    idempotency_key TEXT NOT NULL,
    source_kind     TEXT NOT NULL DEFAULT '',
    source_id       TEXT NOT NULL DEFAULT '',
    activity_id     TEXT NOT NULL DEFAULT '',
    awarded_by      UUID,
    earned_count    INT NOT NULL DEFAULT 0,
    occurred_at     TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('applied', 'rejected')),
    reason          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL
);

-- Redelivered commands hit this and become no-ops.
CREATE UNIQUE INDEX idx_badge_awards_tenant_key ON badge_awards (tenant_id, idempotency_key);
CREATE INDEX idx_badge_awards_holding ON badge_awards (player_badge_id) WHERE status = 'applied';

CREATE TABLE badge_revocations (
    id                   UUID PRIMARY KEY,
    tenant_id            UUID NOT NULL,
    player_id            UUID NOT NULL,
    badge_id             UUID NOT NULL,
    player_badge_id      UUID NOT NULL,
    earned_count_removed INT NOT NULL,
    revoked_by           UUID,
    revoked_at           TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_badge_revocations_tenant_player ON badge_revocations (tenant_id, player_id);

CREATE TABLE reconcile_markers (
    id       INT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE badge_revocations;
DROP TABLE badge_awards;
DROP TABLE player_badges;
DROP TABLE badges;
