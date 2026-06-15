# transactions — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the Transaction rows and the per-connection sync cursors; hosts the recurring bank-sync task. Domain authority: [`docs/domain/README.md`](../../../docs/domain/README.md) §Transactions and §Sync orchestration.

Module-specific notes:
- Provider-agnostic: reaches the bank only through the injected `banking.BankProvider` seam and `banking.ErrReauthRequired`. It must never import `plaid` (enforced by `architecture/isolation_test.go`).
- **One-way dependencies:** `transactions` imports `accounts.Service` and `categorization.Service`; neither calls back into Transactions. `SyncTransactions` calls `accounts.SyncAccounts` FIRST (balances + connection health) before writing any transaction rows, then pulls per connection — keeping the module graph an acyclic DAG.
- **Categorization (this module is the only writer).** After upserting a pull's `added`/`modified` rows, the sync auto-categorizes each new-or-still-uncategorized, non-overridden row via `categorization.Resolve` and writes `classification`/`category_id` (the bank-sync upsert deliberately never touches those columns, so an existing row's categorization is preserved). `ReCategorize` records a manual override (`categorization_overridden = 1`, sticky across re-sync; a Spending choice requires a Category, Income/Transfer/needs-review clear it). `ApplyCategorization(substrings)` re-resolves matching non-overridden rows and is driven by the server-wired re-categorize seam on a Rule change. Categorization resolution runs on the global handle after the upsert transaction commits.
- It gets the connections to pull from `accounts.ConnectionsToSync`, which carries each connection's decrypted access token and its provider→internal account-id map. Decryption happens inside `accounts` (the field's owner); transactions resolves each pulled row's account via that map.
- `repo.go` is the only file that touches `core/db/sqlc`; its methods take and return this package's domain types (`Transaction`, `RecentTransaction`), never `sqlc.*`.
- Rows are keyed by the **stable provider transaction id** (PK). `added`/`modified` upsert in place; `removed` ids delete by id — so a re-sync over unchanged data is idempotent (no duplicates). Amounts are stored signed as-is from the seam (outflow positive, inflow negative).
- Per-connection failure isolation: a pull that returns `banking.ErrReauthRequired` is skipped (its cursor left unchanged) and the pass continues; the overall sync does not error. Mirrors `accounts.syncConnection`.
- Each connection's resume cursor lives in `transaction_sync_state` (PK `connection_id`). A fresh connection starts from the empty cursor (full backfill) and the returned cursor is persisted in the same transaction as the row writes, so a partial apply never leaves the cursor ahead of the data.
- The bank's two-level category strings are still stored verbatim (`category_primary` / `category_detailed`) as the input to resolution; the resolved internal facet lives in the separate `classification` / `category_id` / `categorization_overridden` columns.
- **Out of scope here:** transfer subtype/destination pairing and the `transferDestinationOverridden` facet — a later slice that needs the Accounts read.
- The recurring sync lives in `task.go` (implements `core/task.Task`) and is registered at the composition root (`server/services.go`).
