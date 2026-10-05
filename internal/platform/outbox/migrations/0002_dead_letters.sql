-- +goose Up

-- R23: per-row publish bookkeeping. A row the broker nacks is retried with
-- backoff instead of blocking every row behind it; after the attempt cap it
-- moves to dead_letters below.
ALTER TABLE outbox_events
    ADD COLUMN attempts        INT         NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Parked rows from both sides of the broker. Dispatcher rows are moved here
-- from outbox_events and can be replayed back; consumer rows mirror the
-- broker DLQ for inspection and are replayed from the broker (task
-- rabbit:replay). Unreplayed rows are never pruned.
CREATE TABLE dead_letters (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source      TEXT        NOT NULL CHECK (source IN ('dispatcher', 'consumer')),
    event_id    TEXT,
    topic       TEXT        NOT NULL,
    queue       TEXT,
    payload     JSONB       NOT NULL,
    headers     JSONB       NOT NULL DEFAULT '{}',
    error       TEXT        NOT NULL,
    attempts    INT         NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    dead_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    replayed_at TIMESTAMPTZ
);

CREATE INDEX idx_dead_letters_open ON dead_letters (dead_at DESC) WHERE replayed_at IS NULL;
CREATE INDEX idx_dead_letters_replayed ON dead_letters (replayed_at) WHERE replayed_at IS NOT NULL;

-- +goose Down
DROP TABLE dead_letters;
ALTER TABLE outbox_events DROP COLUMN next_attempt_at, DROP COLUMN attempts;
