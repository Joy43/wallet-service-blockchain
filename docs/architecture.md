# Architecture

Fill this in as services are added.

## Services

| Service | Status | Owns |
|---|---|---|
| auth | implemented (`services/auth`) | users, sessions |
| wallet | planned (`services/wallet`) | balances, ledger, transfers |
| notification | planned (`services/notification`) | email / SMS delivery |
| gateway | planned (`services/gateway`) | edge routing |

## How services talk
- Immediate answer needed: gRPC (contracts in `proto/`)
- Can happen later: NATS events (shapes in `proto/events/v1`)

## Token verification (no shared secret)
auth-service signs access tokens with an Ed25519 private key it alone holds and publishes the public key
at `GET /.well-known/jwks.json`. Other services verify tokens locally with it. Where a revoked session must
be refused immediately (moving money), call gRPC `ValidateToken` instead. See `docs/decisions/0001-*.md`.

## Events
Contract and delivery guarantees: `docs/events.md`. `user.registered` goes through a transactional outbox.

## Rules
- Each service owns its database. No cross-service database access.
- `proto/` is the only shared code.
- Record every significant decision as a short file in `docs/decisions/`.
