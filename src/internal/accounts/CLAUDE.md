# accounts — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Module-specific notes:
- Provider-agnostic: reaches the bank only through the injected `banking.BankProvider` seam and `banking.ErrReauthRequired`. It must never import `plaid` (enforced by `architecture/isolation_test.go`).
- `repo.go` is the only file that touches `core/db/sqlc`; its methods take and return this package's domain types (`Connection`, `Account`), never `sqlc.*`.
- Access tokens are encrypted at rest via `core/cryptox` under the config encryption key; the plaintext is never persisted unencrypted, never sent to the client, and is never stored on the `Connection` entity. It may be handed to the in-process `transactions` sync via `ConnectionsToSync` (decrypted inside this service, the field's owner).
- Seeding (`SeedAccountKind` / `SeedCountsAsSavings`) runs only when an account first appears. Sync refreshes balance + last-synced and discovers new accounts, but never reseeds an existing account's kind/counts-as-savings, nor duplicates it.
- The overview totals are computed over active (non-hidden/non-closed) accounts only; an unknown balance is excluded, never counted as zero.
- The overview at `/` surfaces the whole connection lifecycle — connect, disconnect (confirmation-gated, deletes the bank's accounts + connection), reconnect (update mode, clears `needs_reconnect`) — each swapping the shared overview region. Recoverable failures render inline (connect error in the control; reconnect error beside the row); never redirect.
- Disconnect is a server action in both bank modes (no provider modal); reconnect mirrors connect's dual mode (fake posts directly, real runs the Alpine relink-token interceptor). Failures during reconnect leave the connection `needs_reconnect`.
- e2e (`BANK_PROVIDER=fake`) covers connect/disconnect/reconnect against the real server. The fake holds one connection with all three accounts; reconnect e2e connects via the fake then flips `state` to `needs_reconnect` in SQLite so the stored token stays decryptable.
