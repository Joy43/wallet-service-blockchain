CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'user',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Written by the background worker from auth.> events. event_id makes
-- redelivered messages idempotent.
CREATE TABLE IF NOT EXISTS auth_audit_log (
    event_id    TEXT PRIMARY KEY,
    type        TEXT        NOT NULL,
    user_id     TEXT        NOT NULL DEFAULT '',
    email       TEXT        NOT NULL DEFAULT '',
    ip          TEXT        NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_auth_audit_log_occurred_at ON auth_audit_log (occurred_at);
