CREATE TABLE IF NOT EXISTS orders (
    id           TEXT PRIMARY KEY,
    customer_id  TEXT        NOT NULL,
    product_id   TEXT        NOT NULL,
    quantity     INTEGER     NOT NULL CHECK (quantity > 0),
    amount_cents BIGINT      NOT NULL CHECK (amount_cents > 0),
    status       TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders (customer_id);
