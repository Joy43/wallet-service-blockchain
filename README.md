# auth-service

Authentication microservice in Go: register, login, refresh-token rotation, logout,
JWKS for local token verification, and gRPC token validation for other services.

| Concern | Technology |
|---|---|
| Client API | HTTP/JSON (`/v1/auth/*`), Swagger at `/docs` |
| Service-to-service | gRPC `ValidateToken` + standard gRPC health (`proto/auth/v1`) |
| Token verification elsewhere | `GET /.well-known/jwks.json` (Ed25519 public key) |
| Users, audit log, outbox | PostgreSQL (database `auth`, owned only by this service) |
| Sessions, rate limit, profile cache | Redis |
| Events | NATS JetStream (`auth.>`), see [docs/events.md](docs/events.md) |
| Background work | outbox relay, audit-log consumer, retention janitor |

## Quick start

```bash
make keygen                       # prints JWT_PRIVATE_KEY=...
export JWT_PRIVATE_KEY=<value>    # compose refuses to start without it
docker compose up --build

curl -X POST localhost:8081/v1/auth/register -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","password":"password123"}'
curl -X POST localhost:8081/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","password":"password123"}'
curl localhost:8081/v1/auth/me -H "Authorization: Bearer <access_token>"
curl localhost:8081/.well-known/jwks.json
# API docs: http://localhost:8081/docs     ops: /healthz /readyz /metrics
```

## How authentication works

- Login returns a 15-minute Ed25519-signed JWT and a 30-day opaque refresh token.
- A session lives in Redis. Logout deletes it, which ends its access tokens for anyone who checks (gRPC `ValidateToken`).
- Refresh rotates the refresh token. Replaying an old one revokes the whole session.
- Login is rate limited per email. Unknown email and wrong password return the same error.
- Only auth-service holds the private key. Other services verify with the JWKS public key
  (no shared secret) or call `ValidateToken` when they need instant revocation.
- `user.registered` is published through a transactional outbox, so it is never lost.

## Repository layout

```
services/
  auth/                 implemented, its own go.mod, Dockerfile, migrations
    cmd/auth            service entry point
    cmd/keygen          prints a new signing key
    internal/           model, dto, controller, middleware, router, service,
                        repository (postgres, redisstore), events, worker,
                        security, httputil, config
  wallet/ notification/ gateway/      planned (scaffold only)
gen/                    generated gRPC code, its own go.mod (shared contract)
proto/                  .proto contracts (the only code shared between services)
deploy/k8s/base         Kubernetes manifests (kustomize); overlays/ for dev, staging, prod
docs/                   architecture, events, decisions (ADRs), runbooks
.github/workflows       CI per service
```

## Layer rules (inside `services/auth/internal`)

| Layer                 | May import                               | Must NOT import                    |
|-----------------------|------------------------------------------|------------------------------------|
| `model`               | standard library only                    | anything in this repo              |
| `dto`                 | `model`                                  | service, repository                |
| `service`             | `model`                                  | pgx, redis, nats, net/http, dto    |
| `repository`, `events`, `security` | `model`, `service` (errors only) | `controller`, `router`    |
| `controller`          | `model`, `dto`, `httputil`, `middleware` | `repository`, `events`             |
| `middleware`          | `model`, `httputil`                      | `controller`, `service`            |
| `router`              | `controller`, `middleware`               | `repository`                       |
| `cmd/auth`            | everything: it wires the layers          |                                    |

## Commands

```bash
make test                 # unit tests (race detector on)
make run                  # needs .env (see .env.example) and Postgres, Redis, NATS
make proto                # regenerate gen/ (buf + protoc-gen-go + protoc-gen-go-grpc)
make k8s-image && make k8s-secret && make k8s-apply     # Kubernetes (namespace "platform")
```

## Not done yet
- Integration tests against real Postgres / Redis / NATS (only unit tests exist)
- OpenTelemetry tracing, mTLS between services, automated key rotation
- Email verification, password reset, 2FA; no route is role-restricted yet (`middleware.RequireRole` is ready)
- Managed Postgres / Redis / NATS instead of `deploy/k8s/base/infra.yaml`
