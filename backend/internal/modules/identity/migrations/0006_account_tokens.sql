-- +goose Up
-- Emailed single-use tokens: password resets, email verifications and
-- invitations. Only a SHA-256 of each token is stored (token_hash); the
-- plaintext exists only in the email.

CREATE TABLE password_resets (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email      TEXT NOT NULL,                 -- the address the link went to
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_password_resets_user ON password_resets (user_id);

CREATE TABLE email_verifications (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email      TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_email_verifications_user ON email_verifications (user_id);

CREATE TABLE invitations (
    id               UUID PRIMARY KEY,
    tenant_id        UUID NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email            TEXT NOT NULL CHECK (email = lower(email)),
    name             TEXT NOT NULL DEFAULT '',
    role_ids         BIGINT[] NOT NULL,
    token_hash       TEXT NOT NULL UNIQUE,
    invited_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    accepted_at      TIMESTAMPTZ,
    accepted_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    revoked_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL
);

-- At most one open invitation per address and tenant; a re-invite revokes
-- the previous one in the same transaction.
CREATE UNIQUE INDEX invitations_open_key ON invitations (tenant_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE INDEX idx_invitations_tenant ON invitations (tenant_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE invitations;
DROP TABLE email_verifications;
DROP TABLE password_resets;
