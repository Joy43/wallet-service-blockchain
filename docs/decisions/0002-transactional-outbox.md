# 0002: Publish `user.registered` through a transactional outbox

## Context
Publishing to NATS after the database commit can fail between the two steps, leaving a user
who exists but whose event is lost. Other services depend on that event.

## Decision
Registration inserts the user and an `auth_outbox` row in one Postgres transaction.
A relay (`worker/outbox_relay.go`) publishes pending rows to NATS using
`FOR UPDATE SKIP LOCKED` (safe with several replicas) and marks them published.
The event id is the JetStream de-duplication id.

## Consequences
- Delivery is at-least-once. Consumers must be idempotent.
- Events can lag by about a second (relay interval).
- Published rows are purged after 7 days.
- Other events (login, logout, failures) still publish directly and may be lost in a broker outage.
