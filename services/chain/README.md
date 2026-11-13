# chain service (planned)

Talks to the Hedera network on behalf of the wallet: watches deposits, sends withdrawals.
Not implemented yet.

## Why it is a separate service
It is the **only** service that holds blockchain keys. auth and wallet never see them, so a
bug or breach there cannot move funds. wallet asks chain to act (gRPC or events).

## Configuration
Copy `.env.example` to `.env` (git-ignored). See the comments in that file.
- Missing key means the service refuses to start.
- Never log the key, never return it in an error or API response.
- One key per environment. Testnet keys are never reused on mainnet.
- Real-money environments: key in KMS/HSM or a secret manager, not an env file.

## Folders
| Folder | Will hold |
|---|---|
| `cmd/chain` | entry point |
| `internal/hedera` | SDK calls (balance, transfer, token service, consensus service) |
| `internal/platform` | config, logger, connections |
| `test` | tests (use testnet or a fake client, never mainnet) |
