-- +goose Up
CREATE TABLE keys (
    key          TEXT PRIMARY KEY,
    request_hash BYTEA NOT NULL,
    status       INT,
    content_type TEXT,
    response     BYTEA,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_idempotency_created ON keys (created_at);

-- +goose Down
DROP TABLE keys;
