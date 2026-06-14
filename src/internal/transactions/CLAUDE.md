# transactions — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the Transaction rows and the per-connection sync cursors; hosts the recurring bank-sync task. Domain authority: [`docs/domain/README.md`](../../../docs/domain/README.md) §Transactions and §Sync orchestration.

Module-specific notes:
- Provider-agnostic: reaches the bank only through the injected `banking.BankProvider` seam and `banking.ErrReauthRequired`. It must never import `plaid` (enforced by `architecture/isolation_test.go`).
- **One-way dependency:** `transactions` imports `accounts.Service`; Accounts never calls Transactions. `SyncTransactions` calls `accounts.SyncAccounts` FIRST (balances + connection health) before writing any transaction rows, then pulls per connection — keeping the module graph an acyclic DAG.
- It gets the connections to pull from `accounts.ConnectionsToSync`, which carries each connection's decrypted access token and its provider→internal account-id map. Decryption happens inside `accounts` (the field's owner); transactions resolves each pulled row's account via that map.
- `repo.go` is the only file that touches `core/db/sqlc`; its methods take and return this package's domain types (`Transaction`, `RecentTransaction`), never `sqlc.*`.
- Rows are keyed by the **stable provider transaction id** (PK). `added`/`modified` upsert in place; `removed` ids delete by id — so a re-sync over unchanged data is idempotent (no duplicates). Amounts are stored signed as-is from the seam (outflow positive, inflow negative).
- Per-connection failure isolation: a pull that returns `banking.ErrReauthRequired` is skipped (its cursor left unchanged) and the pass continues; the overall sync does not error. Mirrors `accounts.syncConnection`.
- Each connection's resume cursor lives in `transaction_sync_state` (PK `connection_id`). A fresh connection starts from the empty cursor (full backfill) and the returned cursor is persisted in the same transaction as the row writes, so a partial apply never leaves the cursor ahead of the data.
- **Out of scope here:** categorization (Classification/Category), manual overrides, transfer pairing — the Categorization domain does not exist yet. The bank's two-level category strings are stored verbatim (`category_primary` / `category_detailed`); no taxonomy mapping.
- The recurring sync lives in `task.go` (implements `core/task.Task`) and is registered at the composition root (`server/services.go`).
