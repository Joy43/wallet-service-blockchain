# wallet service (planned)

Owns balances, transfers and the ledger. Not implemented yet.

| Folder | Will hold |
|---|---|
| `cmd/wallet` | entry point (wiring only) |
| `internal/account` | wallet accounts, balance queries |
| `internal/ledger` | append-only record of every transaction |
| `internal/transfer` | moving money between accounts |
| `internal/chain` | blockchain integration (only if the product needs it) |
| `internal/platform` | config, logger, DB / Redis / NATS connections |
| `internal/middleware` | token validation (calls auth over gRPC), request ID, metrics |
| `migrations` | this service's own database schema (`wallet_db`) |
| `api` | OpenAPI description |
| `test/integration`, `test/contract` | real-dependency tests, proto compatibility tests |

Each feature folder keeps its own model, service, repository, controller and dto together.
No other service reads this service's database: use gRPC or events.
