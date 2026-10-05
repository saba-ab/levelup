-- +goose Up
-- Lands in schema program_svc. Table names match GORM's derivation from the
-- unexported models (program → programs, enrollment → enrollments).
CREATE TABLE programs (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL, -- bare uuid, NO foreign key to identity_svc (P5)
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
    slug        TEXT NOT NULL CHECK (length(slug) BETWEEN 1 AND 120),
    description TEXT NULL CHECK (description IS NULL OR length(description) <= 1000),
    status      TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'paused', 'ended')),
    starts_at   TIMESTAMPTZ NULL,
    ends_at     TIMESTAMPTZ NULL,
    settings    JSONB NOT NULL DEFAULT '{}'::jsonb,
    mechanics   JSONB NOT NULL DEFAULT '{}'::jsonb,
    legacy_id   BIGINT NULL UNIQUE, -- Laravel id, for the data migration (ADR-0016)
    version     INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,
    deleted_at  TIMESTAMPTZ NULL,
    CONSTRAINT ck_programs_window CHECK (starts_at IS NULL OR ends_at IS NULL OR ends_at >= starts_at)
);

-- Slug is unique per tenant among live rows; a soft-deleted program frees it.
CREATE UNIQUE INDEX ux_programs_tenant_slug ON programs (tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX ix_programs_tenant_created ON programs (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX ix_programs_tenant_status ON programs (tenant_id, status) WHERE deleted_at IS NULL;
-- The program.auto_end sweep.
CREATE INDEX ix_programs_running_ends_at ON programs (ends_at)
    WHERE deleted_at IS NULL AND status IN ('active', 'paused') AND ends_at IS NOT NULL;

-- Membership. program_id is an intra-module FK (allowed); player_id is a
-- BARE uuid, deliberately NO foreign key to player_svc (P5).
CREATE TABLE enrollments (
    program_id  UUID NOT NULL REFERENCES programs (id) ON DELETE CASCADE,
    player_id   UUID NOT NULL,
    tenant_id   UUID NOT NULL,
    enrolled_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (program_id, player_id)
);

CREATE INDEX ix_enrollments_program_cursor ON enrollments (program_id, enrolled_at DESC, player_id DESC);
CREATE INDEX ix_enrollments_tenant_player ON enrollments (tenant_id, player_id);

-- +goose Down
DROP TABLE enrollments;
DROP TABLE programs;
