# notification service (planned)

Sends email and SMS in reaction to events (for example `auth.user.registered`). Not implemented yet.

| Folder | Will hold |
|---|---|
| `cmd/notification` | entry point |
| `internal/email` | email sending |
| `internal/sms` | SMS sending |
| `internal/platform` | config, logger, NATS connection |
| `test` | tests |

Mostly event-driven: it consumes events and exposes little or no public API.
