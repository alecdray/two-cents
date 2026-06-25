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
  category (stored verbatim), and a `pending` / `posted` status. It also carries
  **read-only bank display detail** ([ADR-0013](../../../docs/adr/0013-richer-bank-transaction-detail.md)):
  the raw descriptor, merchant logo / website / entity id, payment channel, the
  bank's categorization confidence, the authorized and posted timestamps, and the
  structured counterparties list. All of it is bank-sourced and refreshed by sync
  (so it is part of the upsert — see [CLAUDE.md](./CLAUDE.md)); none of it feeds
  categorization, which still resolves on the cleaned merchant and bank category.

This module is the **sole writer** of a Transaction's Classification + Category
and its transfer destination + subtype, though the *decisions* come from
[Categorization](../categorization/README.md): the sync calls
`categorization.Resolve` per new/uncategorized row, and `ReCategorize` /
`MarkTransferDestination` record the user's sticky manual overrides. A
`ReCategorize` that moves a row **off** Transfer clears its transfer
destination + subtype — a subtype is meaningless on a non-Transfer row, and
Reporting counts a Savings contribution by subtype alone, so a stale one would
double-count (see the domain
[ReCategorize](../../../docs/domain/README.md) card). The bank's two-level
category strings are stored verbatim as the input to that resolution (see
[CLAUDE.md](./CLAUDE.md) for the column ownership).

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
  4. Sweeps every still-uncategorized, non-overridden row — across the whole
     stored set, not just this pass's delta — through Categorization, then
     re-pairs transfer destinations. Both run over stored rows, so categorization
     **self-heals**: a row left uncategorized by an earlier sync resolves on the
     next one (no full re-backfill needed).

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
cleaned merchant; a **needs-attention** toggle (`?view=needs-attention`) filters
to the needs-attention set.
Both filters query full history; the default view stays at the recent cap
(general pagination is deferred — see [roadmap](../../../docs/roadmap.md)).

A **Sync now** control pulls activity on demand. While its request is in flight the
control is disabled and reads as working (so a second click is ignored), returning to
normal once the swap lands. The outcome surfaces in an inline slot beside the control
([ADR-0015](../../../docs/adr/0015-app-wide-request-feedback.md)): a recoverable failure
renders a persistent error there; a success renders a transient confirmation that
auto-clears, so a sync that changed nothing visible still acknowledges it ran.

Clicking a row opens the shared **transaction-editing modal** ([ADR-0011](../../../docs/adr/0011-reusable-transaction-editing-modal.md))
— the whole row is the trigger (it has no navigational target of its own). The
module serves the editor content from an edit endpoint into the modal shell. Its
header surfaces read-only context — the account (with its mask), the bank's raw
category (the signal behind auto-categorization), and an Auto/Manual badge for
whether the categorization is the guess or a sticky override — alongside the
richer bank display detail ([ADR-0013](../../../docs/adr/0013-richer-bank-transaction-detail.md)):
the merchant with its logo and website, the intermediary from the counterparties
list ("merchant via DoorDash"), the raw descriptor, payment channel, the
authorized/posted timestamps, and the categorization confidence (surfaced only
when low). Each is rendered only when the bank populated it.
The editor also surfaces the **Rules governing this transaction**
([ADR-0016](../../../docs/adr/0016-rule-editor-modal-and-cross-modal-return.md)):
it asks categorization (`RulesMatching`, the only new cross-module call — the
`transactions → categorization` edge already exists) for the Rules matching the
row's merchant and lists them with the winner marked, each a control that opens
categorization's rule editor modal in edit mode; when none match it offers a
**create** control prefilled from the transaction (merchant substring + current
outcome). These controls open the modal by URL only — no categorization view is
imported — and hand it this transaction's own edit endpoint as the opaque
**return handle**, so saving, deleting, *or* dismissing the rule modal re-mounts this
editor, refreshed (any re-categorization having run). Opening the rule modal replaces this
one (the shell mounts one at a time); the return handle is what brings the user back.

The editor is one form with a single **Save**; on save it runs the existing operations
in turn — `ReCategorize`, then `MarkTransferDestination` for an outflow Transfer —
and emits `transaction-changed`
([ADR-0010](../../../docs/adr/0010-event-driven-cross-region-refresh.md)). The list
region self-refreshes on that event by re-fetching itself in the current search +
view: the needs-attention worklist re-queries and so drops a row once it no longer
qualifies (shrinking toward empty), while the default view shows the edited row in
its new state. The region owns the refresh, so the edit endpoint stays view-agnostic
— it announces the change, it does not know the caller.

## Persistence

- `transactions` — one row per bank transaction, PK the provider id; indexed on
  `(date DESC)` and on `account_id` (FK → `accounts.id`). The read-only bank
  display detail ([ADR-0013](../../../docs/adr/0013-richer-bank-transaction-detail.md))
  lives in the same row; the structured counterparties list is stored as a JSON
  column (display-only, never queried).
- `transaction_sync_state` — one row per connection (PK `connection_id`) holding
  the resume cursor. A fresh connection starts from the empty cursor (full
  backfill); thereafter the stored cursor is the resume point.

## Background task

The recurring bank sync lives in `task.go` (implements `core/task.Task`) and is
registered with the task manager at the composition root. Each tick runs
`SyncTransactions` (Accounts first, then the per-connection pulls).
