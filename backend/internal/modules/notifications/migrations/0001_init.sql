-- +goose Up
-- Lands in schema notifications_svc. Table names match GORM's derivation
-- from the unexported models in internal/repo. tenant_id / player_id /
-- created_by are bare uuids: no foreign key leaves this schema (P5).

CREATE TABLE notification_templates (
    id             UUID PRIMARY KEY,
    tenant_id      UUID NOT NULL,
    name           TEXT NOT NULL,
    trigger        TEXT NOT NULL CHECK (trigger IN (
                       'badges.awarded', 'progression.level_reached', 'missions.completed',
                       'streaks.milestone_reached', 'streaks.broken', 'rewards.claimed', 'points.credited')),
    channels       TEXT[] NOT NULL CHECK (cardinality(channels) >= 1 AND channels <@ ARRAY['in_app', 'email']::TEXT[]),
    -- Go text/template sources, validated (parse + dry run) on save.
    title_template TEXT NOT NULL,
    body_template  TEXT NOT NULL DEFAULT '',
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_by     UUID,
    version        INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    deleted_at     TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_notification_templates_tenant_name ON notification_templates (tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_notification_templates_tenant_page ON notification_templates (tenant_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_notification_templates_trigger ON notification_templates (tenant_id, trigger) WHERE deleted_at IS NULL AND is_active;

-- One row per (template, triggering event, channel). Title and body are
-- rendered at fan-out; the recipient email is read at send time and never
-- stored.
CREATE TABLE notifications (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL,
    template_id  UUID NOT NULL REFERENCES notification_templates (id),
    player_id    UUID NOT NULL,
    -- bus envelope event_id of the triggering fact.
    event_id     TEXT NOT NULL,
    trigger      TEXT NOT NULL,
    channel      TEXT NOT NULL CHECK (channel IN ('in_app', 'email')),
    status       TEXT NOT NULL CHECK (status IN ('pending', 'delivered', 'failed', 'skipped')),
    title        TEXT NOT NULL,
    body         TEXT NOT NULL,
    reason       TEXT NOT NULL DEFAULT '',
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT NOT NULL DEFAULT '',
    lease_until  TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    read_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);

-- Redelivered facts hit this and insert nothing.
CREATE UNIQUE INDEX idx_notifications_template_event_channel ON notifications (template_id, event_id, channel);
CREATE INDEX idx_notifications_tenant_page ON notifications (tenant_id, created_at DESC, id DESC);
CREATE INDEX idx_notifications_feed ON notifications (tenant_id, player_id, created_at DESC, id DESC) WHERE channel = 'in_app';
CREATE INDEX idx_notifications_unread ON notifications (tenant_id, player_id) WHERE channel = 'in_app' AND status = 'delivered' AND read_at IS NULL;
CREATE INDEX idx_notifications_pending ON notifications (created_at) WHERE status = 'pending';

CREATE TABLE channel_settings (
    tenant_id       UUID PRIMARY KEY,
    email_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    email_from_name TEXT NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE channel_settings;
DROP TABLE notifications;
DROP TABLE notification_templates;
