-- +goose Up
-- Lands in schema progression_svc. Table names match GORM's derivation from
-- the unexported models in internal/repo (playerProgress → player_progresses).
-- tenant_id, player_id, badge_reward_id, created_by are bare uuids: NO
-- foreign keys into other modules' schemas (P5). The only FKs stay inside
-- this schema.

-- The level ladder. xp_required is a cumulative threshold that strictly
-- increases with level_number (enforced by the service under an advisory
-- lock; the partial unique index keeps numbers unique among live rows).
CREATE TABLE levels (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    level_number    INT NOT NULL CHECK (level_number >= 1),
    name            TEXT NOT NULL,
    description     TEXT,
    xp_required     BIGINT NOT NULL DEFAULT 0 CHECK (xp_required >= 0),
    points_reward   BIGINT NOT NULL DEFAULT 0 CHECK (points_reward >= 0),
    badge_reward_id UUID,
    perks           JSONB,
    icon_url        TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    deleted_at      TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_levels_tenant_number ON levels (tenant_id, level_number) WHERE deleted_at IS NULL;
CREATE INDEX idx_levels_tenant_xp ON levels (tenant_id, xp_required) WHERE deleted_at IS NULL;
CREATE INDEX idx_levels_updated ON levels (updated_at);

-- One row per player: total XP and the level derived from it. Row-locked
-- (FOR UPDATE) and versioned on every grant.
CREATE TABLE player_progresses (
    tenant_id    UUID NOT NULL,
    player_id    UUID NOT NULL,
    total_xp     BIGINT NOT NULL DEFAULT 0 CHECK (total_xp >= 0),
    level_id     UUID REFERENCES levels (id),
    level_number INT NOT NULL DEFAULT 0,
    version      INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, player_id)
);
CREATE INDEX idx_player_progresses_tenant_total ON player_progresses (tenant_id, total_xp DESC);
CREATE INDEX idx_player_progresses_updated ON player_progresses (updated_at);

-- The XP ledger (Laravel had none). The unique key makes every grant
-- command idempotent: a redelivery inserts nothing and publishes nothing.
CREATE TABLE xp_grants (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    player_id       UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    amount          BIGINT NOT NULL CHECK (amount > 0),
    description     TEXT,
    source_kind     TEXT,
    source_id       TEXT,
    activity_id     TEXT,
    occurred_at     TIMESTAMPTZ NOT NULL,
    created_by      UUID,
    created_at      TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX idx_xp_grants_idem ON xp_grants (tenant_id, idempotency_key);
CREATE INDEX idx_xp_grants_player ON xp_grants (tenant_id, player_id, created_at DESC, id DESC);

-- One row per (player, level), ever: level rewards are exactly-once no
-- matter the order or multiplicity of grant delivery.
CREATE TABLE level_rewards (
    id         UUID PRIMARY KEY,
    tenant_id  UUID NOT NULL,
    player_id  UUID NOT NULL,
    level_id   UUID NOT NULL REFERENCES levels (id),
    granted_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX idx_level_rewards_player_level ON level_rewards (tenant_id, player_id, level_id);

-- Rejected grant commands, so a redelivered rejection publishes once.
CREATE TABLE grant_rejections (
    tenant_id       UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    player_id       UUID NOT NULL,
    reason          TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, idempotency_key)
);

CREATE TABLE reconcile_markers (
    id       INT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE grant_rejections;
DROP TABLE level_rewards;
DROP TABLE xp_grants;
DROP TABLE player_progresses;
DROP TABLE levels;
