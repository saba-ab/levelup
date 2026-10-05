-- +goose Up
CREATE TABLE outbox_events (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id     UUID NOT NULL UNIQUE,
    topic        TEXT NOT NULL,
    payload      JSONB NOT NULL,
    headers      JSONB NOT NULL DEFAULT '{}',
    occurred_at  TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ
);

-- The dispatcher's working set: only unpublished rows, in insert order.
CREATE INDEX idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;

-- Pruning path: published rows are kept 7 days for replay (PRD §7.9.2).
CREATE INDEX idx_outbox_published_at ON outbox_events (published_at) WHERE published_at IS NOT NULL;

-- +goose Down
DROP TABLE outbox_events;
