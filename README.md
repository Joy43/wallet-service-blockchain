# order-service

Production-style Go microservice skeleton (hexagonal architecture).

## Quick start

```bash
go mod tidy          # downloads deps, creates go.sum
docker compose up --build

# create an order
curl -X POST localhost:8080/v1/orders \
  -H 'Content-Type: application/json' \
  -d '{"customer_id":"c-1","product_id":"p-1","quantity":2,"amount_cents":1999}'

# fetch it
curl localhost:8080/v1/orders/<id>

# ops
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/metrics
```

## Layer rules

| Layer                  | May import                         | Must NOT import           |
|------------------------|------------------------------------|---------------------------|
| `internal/domain`      | standard library only              | anything in this repo     |
| `internal/service`     | `domain`                           | pgx, nats, net/http       |
| `internal/repository`  | `domain`, pgx                      | `service`, `transport`    |
| `internal/events`      | `domain`, nats                     | `service`, `transport`    |
| `internal/transport`   | `domain`, net/http (or gRPC)       | `repository`, `events`    |
| `cmd/server`           | everything — it wires the layers   | —                         |

## Next steps for production
- Transactional outbox instead of publish-after-commit
- OpenTelemetry tracing (otelhttp middleware + trace context in NATS headers)
- gRPC adapter in `internal/transport/grpcapi` using `proto/`
- Event consumer in `cmd/worker` for `inventory.reserved` / `payment.failed`
