-- +goose Up
-- Lands in schema ai_svc. No foreign keys to other schemas (P5).

-- Per-tenant, per-UTC-day AI usage: the request count is the daily quota
-- counter (reserved atomically before each model call); tokens are added
-- after the call from the provider's reported usage.
CREATE TABLE ai_usages (
    tenant_id     UUID NOT NULL,
    day           DATE NOT NULL,
    requests      BIGINT NOT NULL DEFAULT 0 CHECK (requests >= 0),
    input_tokens  BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, day)
);

-- +goose Down
DROP TABLE ai_usages;
