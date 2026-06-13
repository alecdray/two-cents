# accounts — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Module-specific notes:
- Provider-agnostic: reaches the bank only through the injected `banking.BankProvider` seam and `banking.ErrReauthRequired`. It must never import `plaid` (enforced by `architecture/isolation_test.go`).
- `repo.go` is the only file that touches `core/db/sqlc`; its methods take and return this package's domain types (`Connection`, `Account`), never `sqlc.*`.
- Access tokens are encrypted at rest via `core/cryptox` under the config encryption key; the plaintext never leaves the service and is never stored on the `Connection` entity.
- Seeding (`SeedAccountKind` / `SeedCountsAsSavings`) runs only when an account first appears. Sync refreshes balance + last-synced and discovers new accounts, but never reseeds an existing account's kind/counts-as-savings, nor duplicates it.
- The overview totals are computed over active (non-hidden/non-closed) accounts only; an unknown balance is excluded, never counted as zero.
