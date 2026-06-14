# Transactions sync UI — design spec

Date: 2026-06-14. Status: approved, ready for `/build`.

A read-only transactions list, fed by a real incremental sync, as the tracer-bullet vertical
slice through the documented **Transactions** domain ([`docs/domain/README.md` §Transactions,
§Sync orchestration](../../domain/README.md)). It builds the **bank-sourced subset** of that
domain — rows, dedupe, pending reconcile, cursor, sync orchestration — and explicitly defers
everything that requires the not-yet-built Categorization/Budget domains.

## Goal (smallest genuinely-useful version)

Sync transactions from connected banks, persist them, and show them at `/transactions` as a flat,
dated, newest-first list (date · merchant · account · signed amount, pending flagged). Read-only.
Filtering, search, categorization, and editing are later slices.

## What the domain doc already decides (not free choices)

- **Transactions is its own domain → new `transactions` module, which hosts the sync task.**
- **Dependency direction is one-way: `transactions` imports `accounts.Service`; Accounts never
  calls Transactions.** Module graph stays an acyclic DAG, `accounts` a leaf.
- **A full sync is accounts-first:** `SyncTransactions` calls `Accounts.SyncAccounts` first, then
  pulls/dedupes/reconciles its own rows. Each domain writes only its own tables.
- **Connect/reconnect are orchestrated from the Transactions side**, never from Accounts: the
  connect callback calls `Accounts.ConnectBank` then `Transactions.SyncTransactions` (initial
  backfill from an empty cursor).
- Identity is the **stable provider id**; `added`/`modified` dedupe by it; `removed` deletes by it.

## 1. Module shape

New `src/internal/transactions/` domain module (standard archetype):
`service.go` + `repo.go` (only file touching `core/db/sqlc`) + `task.go` (cron) +
`adapters/{http.go,routes.go}` + `adapters/views/`. Plus `transactions/CLAUDE.md` and
`transactions/README.md` mirroring the `accounts` module's docs.

Imports: `accounts` (the `Service`, one-way), `banking` (the seam + `ErrReauthRequired`),
`core/*`. **Never imports `plaid`** — same rule as `accounts`, enforced by
`architecture/isolation_test.go` (extended here).

## 2. Data model

New goose migration (`task db/create -- transactions`) + sqlc queries in `db/queries/transactions.sql`
(regenerate with `task build/sqlc`, apply with `task db/up`). Update `db/schema.sql`.

**`transactions` table** — owned/written only by the `transactions` module:

| column | notes |
|---|---|
| `id` TEXT PK | the stable **provider transaction id** (the `DedupeKey`) |
| `account_id` TEXT FK → accounts(id) | the account the movement is on |
| `date` TEXT | bank transaction date (calendar date; basis for future month assignment) |
| `amount_amount` REAL | signed: **outflow positive, inflow negative** (domain convention) |
| `amount_currency` TEXT | currency code |
| `merchant` TEXT | provider-supplied merchant (cleaned-merchant is a Categorization concern, deferred) |
| `counterparty` TEXT | raw bank payee |
| `category_primary` TEXT | bank category, stored as-is for later auto-categorization |
| `category_detailed` TEXT | bank category, stored as-is |
| `status` TEXT CHECK(`pending`,`posted`) | from the provider |
| `created_at`,`updated_at` TIMESTAMP | |

Indexes: `idx_transactions_date` on `date DESC` (list ordering), `idx_transactions_account_id`.

**`transaction_sync_state` table** — the per-connection cursor, owned by `transactions`:

| column | notes |
|---|---|
| `connection_id` TEXT PK | one cursor per connection (Plaid Item) |
| `cursor` TEXT | resume point; empty/absent ⇒ initial backfill |
| `updated_at` TIMESTAMP | |

**Deliberately omitted this slice** (arrive with their owning domains, no dead columns now):
Classification axis, `categorizationOverridden`/`transferDestinationOverridden`, transfer
destination + subtype.

## 3. `Transactions.SyncTransactions` (bank-sourced subset)

1. Call `accounts.SyncAccounts(ctx)` first (balances + connection health).
2. For each connection to sync (from `accounts.ConnectionsToSync`, see §4a — active plus
   needs-reconnect; the latter fail the provider call and are skipped): read its stored cursor, call
   `banking.BankProvider.SyncTransactions(ctx, accessToken, cursor)` (the seam pages internally
   over `has_more`), yielding `added`/`modified`/`removed`/`cursor`.
3. Apply per `DedupeKey`: `added`/`modified` → insert-or-update-in-place by provider id; `removed`
   → delete by provider id. (`PendingReconcileMatch` collapses to "overwrite bank-sourced fields"
   — there are no override facets to preserve yet.)
4. **Auto-categorization is skipped** — no Categorization domain yet; bank category is stored as-is.
5. Persist the next cursor in `transaction_sync_state`.

Writes happen in a transaction per connection. A `banking.ErrReauthRequired` from a connection is
non-fatal — skip it and continue (mirrors `accounts.syncConnection`); `accounts.SyncAccounts` flips
the badge.

Read side: `Transactions.RecentTransactions(ctx, limit)` returns the most recent `limit` rows joined
to account name, ordered `date DESC`. Never blocks on the provider.

## 4. Cross-module seams (approved)

**(a) Token access.** `accounts.Service` gains `ConnectionsToSync(ctx) → []SyncTarget{ConnectionID,
AccessToken}` (decrypt happens inside `accounts`, handed to `transactions` in-process). The
`accounts/CLAUDE.md` invariant "plaintext never leaves the service" is **softened** to "never
persisted unencrypted, never sent to the client." Only active/needs-reconnect connections are
returned (needs-reconnect ones will fail the provider call and get skipped).

**(b) Connect-time backfill orchestration.** A thin composition seam wired in `server` — a
`ConnectOrchestrator` holding both `accounts.Service` and `transactions.Service`. The connect
(`POST /accounts/connections`) and reconnect (`POST /accounts/connections/{id}/reconnect`) handlers
call it: `Accounts.CompleteConnect`/`CompleteReconnect` then `Transactions.SyncTransactions` for the
backfill. The connect UI stays on the overview; the overview handlers delegate the orchestration to
the seam rather than relocating into `transactions`. (Implementation note: this means the existing
accounts connect/reconnect handlers need a hook to the orchestrator — wire via `server`, keeping
`accounts` free of any `transactions` import.)

## 5. UI

- **Route:** `GET /transactions` (transactions adapter), rendered with the shared
  `PageLayoutComponent`.
- **Navbar primitive:** new `core/templates/navbar.templ` (a primitive — domain-free) with links to
  `/` (Overview) and `/transactions`, wired into the layout `Navbar` slot. The overview page starts
  passing it too.
- **List:** flat dated rows, newest first, **most recent 100** (no pagination this slice). Each row:
  date · merchant · account name · signed amount; `pending` flagged.
- **Sync-now:** `POST /transactions/sync` runs `SyncTransactions` then htmx-swaps the list region
  (`hx-target` the list frag, `hx-swap="innerHTML"`), mirroring the overview swap pattern.
  Recoverable failures render inline, never redirect (design principles).
- **Empty states:** (i) no connections → prompt to connect (link to `/`); (ii) connected but nothing
  synced → "No transactions yet — Sync now."
- **Testids** (registered in `docs/design/testids.md`): `transactions-page`, `transactions-list`,
  `transactions-row`, `transactions-row-merchant`, `transactions-row-account`,
  `transactions-row-amount`, `transactions-row-pending`, `transactions-sync`,
  `transactions-empty-no-connections`, `transactions-empty-no-transactions`, `nav-overview`,
  `nav-transactions`.

## 6. Testing (the gate: `go build ./...`, `go test ./src/...`, `task test/e2e` all green)

- **fakebank deterministic transactions.** `fakebank.SyncTransactions` currently only echoes the
  cursor. Make it return a fixed `added` set across its three accounts on the empty-cursor first
  call, and an empty delta thereafter — so e2e is deterministic and the incremental-cursor path is
  exercised. Stays within the `external-client` archetype (canned data, no persistence).
- **e2e (`BANK_PROVIDER=fake`)** — new `e2e/feat/transactions.feature` + `e2e/spec/transactions.spec.ts`:
  connect via fake → go to `/transactions` (initial backfill) → assert deterministic rows, account
  names, signed amounts, pending flag → click "Sync now" → assert idempotent (no dupes, `DedupeKey`).
  Plus both empty states. Uses existing `helpers/db.ts` seeding style.
- **Go unit tests:** `DedupeKey` insert vs update-in-place by provider id; `removed` deletes; cursor
  persist/resume; recent-list query + limit ordering. Pure where possible (mirror `computeOverview`).
- **Architecture test:** extend `isolation_test.go` — `transactions` does not import `plaid`; the
  `accounts`-leaf / `transactions`-imports-`accounts` direction holds (no cycle).

## Out of scope (named, so it isn't mistaken for missing)

Categorization (Classification/Category axes, cleaned-merchant, rules), override facets, transfer
detection/subtype (ADR-0003), filtering/search, pagination, per-account drill-down, transactions on
the overview page, manual transaction entry. Each is a later slice with its own spec.
