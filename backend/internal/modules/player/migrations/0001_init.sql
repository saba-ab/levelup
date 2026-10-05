-- +goose Up
-- Lands in schema player_svc. Table name matches GORM's derivation from the
-- unexported model struct (player → players).
CREATE TABLE players (
    id           UUID PRIMARY KEY,
    -- Bare uuids, deliberately NO foreign keys to identity_svc (P5).
    tenant_id    UUID NOT NULL,
    external_id  TEXT NOT NULL CHECK (length(external_id) BETWEEN 1 AND 255),
    display_name TEXT NULL CHECK (display_name IS NULL OR length(display_name) <= 255),
    email        TEXT NULL CHECK (email IS NULL OR length(email) <= 255),
    -- Tenant-defined profile, exposed to rule conditions as player.*.
    attributes   JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_by   UUID NULL,
    version      INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    deleted_at   TIMESTAMPTZ NULL
);

-- Live uniqueness only: a soft-deleted player's external_id can be reused
-- (fixes the Laravel 500, doc 03 §10.6). Also the ON CONFLICT arbiter of
-- the race-safe create.
CREATE UNIQUE INDEX ux_players_tenant_external ON players (tenant_id, external_id) WHERE deleted_at IS NULL;

-- Keyset list: (created_at, id) DESC within a tenant.
CREATE INDEX ix_players_tenant_created ON players (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_players_tenant_active ON players (tenant_id, is_active, created_at DESC, id DESC) WHERE deleted_at IS NULL;

-- Case-insensitive prefix search (lower(col) LIKE 'term%').
CREATE INDEX ix_players_search_external ON players (tenant_id, lower(external_id) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX ix_players_search_name ON players (tenant_id, lower(display_name) text_pattern_ops) WHERE deleted_at IS NULL;
CREATE INDEX ix_players_search_email ON players (tenant_id, lower(email) text_pattern_ops) WHERE deleted_at IS NULL;

-- Tenant purge (tenant.deleted.v1) sweeps live and deleted rows alike.
CREATE INDEX ix_players_tenant ON players (tenant_id);

-- +goose Down
DROP TABLE players;
