-- +goose Up
-- API keys for tenant backends (ADR-0017). Only a SHA-256 of the full key is
-- stored; prefix is the clear lookup handle shown in the UI.
CREATE TABLE api_keys (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name         TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    prefix       TEXT NOT NULL UNIQUE,
    secret_hash  TEXT NOT NULL,
    role_ids     BIGINT[] NOT NULL,
    created_by   UUID,
    created_at   TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_tenant ON api_keys (tenant_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE api_keys;
