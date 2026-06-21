# transactions

Owns the **Transaction** rows — a single money movement on one Account, as
reported by the bank — and the per-connection sync cursors. It hosts the
recurring bank-sync task that drives a full refresh on a schedule.

See the domain model: [`docs/domain/README.md`](../../../docs/domain/README.md)
(Transactions and Sync orchestration sections).

## Boundaries

The module is provider-agnostic. It depends on the `banking.BankProvider` seam
(injected) and `banking.ErrReauthRequired`, the `accounts.Service` (injected),
and `core/*` — never on a concrete provider client such as `plaid`. The provider
isolation test in `src/internal/architecture` fails if that boundary is crossed.

**Dependency direction is one-way:** `transactions` imports `accounts`; Accounts
never calls Transactions. This keeps the module graph an acyclic DAG.

## Entities

- **Transaction** — keyed by the bank's stable provider transaction id (the
  primary key). Carries the owning account's internal id, the transaction date,
  the signed amount (outflow positive, inflow negative — stored as-is from the
  seam), the cleaned merchant and the raw counterparty, the bank's two-level
  category (stored verbatim), and a `pending` / `posted` status.

This module is the **sole writer** of a Transaction's Classification + Category
and its transfer destination + subtype, though the *decisions* come from
[Categorization](../categorization/README.md): the sync calls
`categorization.Resolve` per new/uncategorized row, and `ReCategorize` /
`MarkTransferDestination` record the user's sticky manual overrides. The bank's
two-level category strings are stored verbatim as the input to that resolution
(see [CLAUDE.md](./CLAUDE.md) for the column ownership).

## Behaviour

- **SyncTransactions** — a full sync pass:
  1. Calls `accounts.SyncAccounts` FIRST (refresh balances + connection health)
     before writing any transaction rows.
  2. Gets the connections to pull from `accounts.ConnectionsToSync` — the active
     and needs-reconnect ones, each with its decrypted access token and its
     provider→internal account-id map.
  3. For each connection, pulls the incremental changes from the stored cursor
     through the seam, applies them, and persists the returned cursor (rows +
     cursor in one transaction). `added`/`modified` upsert in place by provider
     id; `removed` ids delete by id. A pull that reports
     `banking.ErrReauthRequired` is skipped (its cursor left unchanged) and the
     pass continues — the overall sync does not error.

  Syncing twice over unchanged provider data is idempotent: the same row set, no
  duplicates.

- **RecentTransactions** — at most `limit` rows across all accounts, most recent
  first (date desc, then id desc), each carrying its account's display name. A
  pure read of stored rows — it never calls the provider. Backs the default
  `/transactions` view.

- **Filtered reads** — back the view's search and needs-attention filter. Unlike
  `RecentTransactions`' recent cap, an **active filter queries the full
  transaction history**: merchant search matches the **cleaned merchant**
  substring (never the raw counterparty — see the glossary), case-insensitive;
  the needs-attention filter selects the
  [needs-attention](../../../docs/domain/README.md) set. The two compose. Same
  ordering; a pure read of stored rows.

## Transactions view (`/transactions`)

The adapter serves the recent-activity page. Rows are grouped under "Month Year"
dividers — month by **transaction date** ([MonthAssignment](../../../docs/architecture/data-model.md)),
no per-month totals (those are the Month wrap's job). A **search** box filters by
cleaned merchant; a **needs-attention** toggle (`?view=needs-attention`, the
deep-link target for the future home alert) filters to the needs-attention set.
Both filters query full history; the default view stays at the recent cap
(general pagination is deferred — see [roadmap](../../../docs/roadmap.md)).

Resolving a transaction from inside the needs-attention view (ReCategorize /
MarkTransferDestination) drops it from the list once it no longer qualifies — the
worklist shrinks toward empty; the same edit in the default view updates the row
in place. The resolve handlers are therefore **view-aware**.

## Persistence

- `transactions` — one row per bank transaction, PK the provider id; indexed on
  `(date DESC)` and on `account_id` (FK → `accounts.id`).
- `transaction_sync_state` — one row per connection (PK `connection_id`) holding
  the resume cursor. A fresh connection starts from the empty cursor (full
  backfill); thereafter the stored cursor is the resume point.

## Background task

The recurring bank sync lives in `task.go` (implements `core/task.Task`) and is
registered with the task manager at the composition root. Each tick runs
`SyncTransactions` (Accounts first, then the per-connection pulls).
