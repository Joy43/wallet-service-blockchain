-- Transactional outbox: rows are written in the SAME transaction as the business change,
-- then a relay publishes them to NATS. Guarantees at-least-once delivery.
CREATE TABLE IF NOT EXISTS auth_outbox (
    id           TEXT PRIMARY KEY,           -- event id; also used as the JetStream de-dup id
    subject      TEXT        NOT NULL,
    payload      TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_auth_outbox_pending ON auth_outbox (created_at) WHERE published_at IS NULL;
