-- +goose Up
-- Lands in schema identity_svc. Table names match GORM's derivation from
-- the unexported models in internal/repo (tenant → tenants, userRole →
-- user_roles). Within-module FKs only; nothing references another schema.

CREATE TABLE tenants (
    id            UUID PRIMARY KEY,
    legacy_id     BIGINT UNIQUE,                       -- Laravel tenants.id (data migration)
    name          TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 255),
    slug          TEXT NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    owner_user_id UUID NULL,                           -- FK added below (circular with users)
    active        BOOLEAN NOT NULL DEFAULT TRUE,
    timezone      TEXT NOT NULL DEFAULT 'UTC',
    settings      JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(settings) = 'object'),
    version       INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    deleted_at    TIMESTAMPTZ NULL                     -- soft delete; modules purge on tenant.deleted.v1
);

-- Slugs are globally unique among live tenants.
CREATE UNIQUE INDEX tenants_slug_key ON tenants (slug) WHERE deleted_at IS NULL;
CREATE INDEX tenants_list_idx ON tenants (created_at DESC, id DESC) WHERE deleted_at IS NULL;

CREATE TABLE users (
    id                  UUID PRIMARY KEY,
    legacy_id           BIGINT UNIQUE,                 -- Laravel users.id (data migration)
    tenant_id           UUID NULL REFERENCES tenants (id) ON DELETE RESTRICT, -- NULL = platform user
    name                TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 255),
    email               TEXT NOT NULL CHECK (email = lower(email)),
    password_hash       TEXT NOT NULL,                 -- bcrypt; legacy $2y$ hashes copied verbatim
    active              BOOLEAN NOT NULL DEFAULT TRUE,
    email_verified_at   TIMESTAMPTZ NULL,
    password_changed_at TIMESTAMPTZ NULL,
    version             INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);

-- Case-insensitive and global across tenants: one user, one tenant.
CREATE UNIQUE INDEX users_email_key ON users (lower(email));
CREATE INDEX users_tenant_list_idx ON users (tenant_id, created_at DESC, id DESC);

ALTER TABLE tenants
    ADD CONSTRAINT tenants_owner_user_fk FOREIGN KEY (owner_user_id)
    REFERENCES users (id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE roles (
    id    BIGINT PRIMARY KEY,                          -- contract ids; casbin subject role:{id}
    key   TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('tenant', 'platform'))
);

-- Mirrors identity/contracts Role* constants and internal/domain/role.go.
-- Ids are part of the contract: never renumber.
INSERT INTO roles (id, key, label, scope) VALUES
    (1, 'platform_admin',  'Platform Admin',  'platform'),
    (2, 'owner',           'Owner',           'tenant'),
    (3, 'super_admin',     'Super Admin',     'tenant'),
    (4, 'admin',           'Admin',           'tenant'),
    (5, 'program_manager', 'Program Manager', 'tenant'),
    (6, 'developer',       'Developer',       'tenant')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE user_roles (
    user_id    UUID   NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id    BIGINT NOT NULL REFERENCES roles (id),
    granted_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, role_id)
);

-- +goose Down
DROP TABLE user_roles;
DROP TABLE roles;
ALTER TABLE tenants DROP CONSTRAINT tenants_owner_user_fk;
DROP TABLE users;
DROP TABLE tenants;
