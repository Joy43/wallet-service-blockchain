# gateway (planned)

Single entry point for clients. Add it once there are three or more services. Not implemented yet.

| Folder | Will hold |
|---|---|
| `cmd/gateway` | entry point |
| `internal/routing` | which URL goes to which service |
| `internal/middleware` | rate limiting, CORS, token check at the edge |
| `internal/platform` | config, logger |
| `test` | tests |

Contains no business logic.
