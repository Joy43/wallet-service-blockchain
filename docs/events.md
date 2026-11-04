# Events published by auth-service

Stream `AUTH`, subjects `auth.<type>`. JSON body, delivered **at least once**:
consumers must de-duplicate by `id`.

```json
{ "id": "…", "type": "user.registered", "user_id": "…", "email": "…", "ip": "…", "occurred_at": "2026-01-01T00:00:00Z" }
```

| Subject | When | Delivery |
|---|---|---|
| `auth.user.registered` | account created | **Outbox**: written in the same DB transaction as the user, so it is never lost |
| `auth.user.logged_in` | successful login | best-effort |
| `auth.login.failed` | wrong password / unknown email | best-effort |
| `auth.user.logged_out` | logout | best-effort |
| `auth.token.reuse_detected` | an already-rotated refresh token was replayed | best-effort |

Only `user.registered` is business-critical (other services create their own data from it),
so only it goes through the outbox. The others feed the audit log and security alerts, where
losing one event during a broker outage is acceptable.

## Consuming
- Create a **durable consumer** with your own name on stream `AUTH` (never share auth's `auth-audit-worker`).
- Ack only after your own work is committed. Make handlers idempotent (key on `id`).
- Treat `email` and `ip` as personal data.
