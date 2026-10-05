-- +goose Up
-- Lands in schema eventcatalog_svc. Table names match GORM's derivation
-- from the repo models (eventCategory → event_categories, eventType →
-- event_types). tenant_id NULL marks a platform-global row: the only tables
-- where that is allowed (ADR-0015). tenant_id is a bare uuid, no FK (P5).

CREATE TABLE event_categories (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NULL,
    slug        TEXT NOT NULL CHECK (length(slug) <= 100 AND slug ~ '^[a-z0-9]+([_-][a-z0-9]+)*$'),
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
    description TEXT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    -- Laravel bigint id, kept for the data migration (ADR-0016).
    legacy_id   BIGINT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

-- Slug unique per tenant, and unique among globals (Laravel had one global
-- UNIQUE(slug) across all tenants). A tenant slug may equal a global one:
-- the tenant row shadows the global row.
CREATE UNIQUE INDEX ux_event_categories_tenant_slug ON event_categories (tenant_id, slug) WHERE tenant_id IS NOT NULL;
CREATE UNIQUE INDEX ux_event_categories_global_slug ON event_categories (slug) WHERE tenant_id IS NULL;

CREATE TABLE event_types (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NULL,
    -- Intra-module FK: allowed (only cross-schema REFERENCES are banned).
    category_id     UUID NULL REFERENCES event_categories (id) ON DELETE SET NULL,
    -- 100 = width of rules.trigger_event, which references the slug.
    slug            TEXT NOT NULL CHECK (length(slug) <= 100 AND slug ~ '^[a-z0-9]+([_-][a-z0-9]+)*$'),
    name            TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
    description     TEXT NULL,
    -- Optional JSON Schema subset for the activity properties. Replaces the
    -- "metadata" column Laravel's code wrote but never created.
    property_schema JSONB NULL CHECK (property_schema IS NULL OR jsonb_typeof(property_schema) = 'object'),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    legacy_id       BIGINT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    deleted_at      TIMESTAMPTZ NULL
);

-- Live slugs unique per tenant and among globals; soft-deleted rows free
-- their slug (Laravel's index included trashed rows → 500 on re-create).
CREATE UNIQUE INDEX ux_event_types_tenant_slug ON event_types (tenant_id, slug)
    WHERE tenant_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX ux_event_types_global_slug ON event_types (slug)
    WHERE tenant_id IS NULL AND deleted_at IS NULL;

-- Keyset pages (created_at, id) DESC per tenant, globals included.
CREATE INDEX ix_event_types_tenant_created ON event_types (tenant_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX ix_event_types_category ON event_types (category_id) WHERE category_id IS NOT NULL;

-- +goose Down
DROP TABLE event_types;
DROP TABLE event_categories;
