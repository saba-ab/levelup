-- +goose Up
-- Lands in schema webhooks_svc. No foreign keys to other modules' schemas
-- (P5): tenant_id is a bare uuid.

CREATE TABLE endpoints (
    id                   UUID PRIMARY KEY,
    tenant_id            UUID NOT NULL,
    url                  TEXT NOT NULL,
    description          TEXT NOT NULL DEFAULT '',
    event_types          TEXT[] NOT NULL CHECK (cardinality(event_types) > 0),
    -- Signing secret, stored as-is; only returned on create and rotate.
    secret               TEXT NOT NULL,
    is_active            BOOLEAN NOT NULL DEFAULT true,
    -- Consecutive FAILED deliveries (not attempts); reset by any success.
    consecutive_failures INT NOT NULL DEFAULT 0,
    disabled_reason      TEXT NOT NULL DEFAULT '',
    disabled_at          TIMESTAMPTZ,
    version              INT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL,
    deleted_at           TIMESTAMPTZ
);

CREATE INDEX ix_endpoints_tenant_created ON endpoints (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
-- Fan-out lookup: a tenant's live active endpoints.
CREATE INDEX ix_endpoints_tenant_active ON endpoints (tenant_id) WHERE deleted_at IS NULL AND is_active;

CREATE TABLE deliveries (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    endpoint_id     UUID NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    -- The source envelope's event_id (a fresh id for webhook.test).
    event_id        TEXT NOT NULL,
    event           TEXT NOT NULL,
    -- Exact request body, so a redelivery resends the same bytes.
    payload         TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed')),
    attempts        INT NOT NULL DEFAULT 0,
    cycle_attempts  INT NOT NULL DEFAULT 0,
    response_status INT,
    response_body   TEXT NOT NULL DEFAULT '',
    latency_ms      BIGINT,
    last_error      TEXT NOT NULL DEFAULT '',
    lease_until     TIMESTAMPTZ,
    enqueued_at     TIMESTAMPTZ NOT NULL,
    last_attempt_at TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    -- A redelivered or reordered fact never fans out twice.
    CONSTRAINT ux_deliveries_endpoint_event UNIQUE (endpoint_id, event_id)
);

CREATE INDEX ix_deliveries_tenant_created ON deliveries (tenant_id, created_at DESC, id DESC);
CREATE INDEX ix_deliveries_endpoint_created ON deliveries (endpoint_id, created_at DESC, id DESC);
-- Retry sweep: pending rows by last activity.
CREATE INDEX ix_deliveries_pending ON deliveries (enqueued_at) WHERE status = 'pending';

CREATE TABLE reconcile_markers (
    job      TEXT PRIMARY KEY,
    last_run TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE reconcile_markers;
DROP TABLE deliveries;
DROP TABLE endpoints;
