# Two Cents — Roadmap

Status overview of what's **built**, what's **committed but not yet built**, and the **deferred
backlog**. This is a navigational summary, not a new source of truth: the *what/why* lives in
[`prd.md`](./prd.md) + [`domain/README.md`](./domain/README.md), decisions in [`adr/`](./adr/), and the
detail of each shipped feature in its build's `as-built.md` (`~/workshop/builds/two-cents-*/`). When this
list and those docs disagree, those docs win — update this file.

**How work ships:** each feature is a vertical slice taken through the four-phase
[development process](./process.md) — spec → implement → audit → merge.

Legend: ✅ shipped · 🔜 committed, not built · 🧊 deferred backlog · ⚠️ known issue / tech debt.

---

## ✅ Shipped (on `main`)

| Capability | Notes | Refs |
|---|---|---|
| **Bank-provider abstraction** | `BankProvider` seam returning our own `Account`/`Transaction` types; provider is an adapter swap. | [ADR-0002](./adr/0002-bankprovider-abstraction.md) |
| **Plaid client + provider selection** | `plaid` external-client (Link token, public-token exchange, `/transactions/sync`, item remove); `fakebank` deterministic stand-in selected by `BANK_PROVIDER`. | [ADR-0006](./adr/0006-bank-provider-selected-by-config.md) |
| **Accounts & connections persistence** | sqlc + SQLite; per-Item access token encrypted at rest (`core/cryptox`). | `two-cents-accounts-slice` |
| **Connect / disconnect / reconnect** | Plaid Link (real) + direct (fake); needs-reconnect state surfaced; inline errors. Real Link validated in Plaid **sandbox**. | `two-cents-connect-bank` |
| **Accounts overview** | Net cash = cash − credit; 3-bucket `kind` (cash/credit/other, user-seeded, `other` excluded). Now at `/accounts`. | [ADR-0005](./adr/0005-spending-tool-three-bucket-account-kind.md) |
| **Transactions sync** | Cursor-based incremental, ~6h cron + on-demand, dedupe by provider id, pending→posted reconcile, `removed`-set deletes; `/transactions` list. | `two-cents-transactions-sync` |
| **Categorization** | Pure precedence engine (override > Rule > bank PFC > direction), built-in PFC taxonomy + custom categories (archive-not-delete), Rules, `/categories` + `/rules` + re-categorize picker. | `two-cents-categorization` |
| **Transfer subtype pairing** | `ResolveTransferSubtype` (exact-cent amount, ±3-day window, conservative); savings-contribution detection; sticky `MarkTransferDestination`; transactions-page chip + picker. | [ADR-0003](./adr/0003-two-layer-transfer-detection.md), `two-cents-transfer-subtype` |
| **Budget + Tracker + Month-wrap** | Rolling budget config + `/budget` editor; current-month Tracker at `/` (remaining, pace, income/savings progress, Everything-else); per-month wraps (actuals only, settling/partial). Configured app timezone. | [ADR-0004](./adr/0004-configured-app-timezone.md), `two-cents-budget-tracker-wrap` |
| **Budget UI polish** | Budget page hides empty categories + add-category control + live residual/balance; Everything-else rendered as a category row on the Tracker. | direct commits (not a `/build`) |
| **Single local login** | Password-only login gating the whole app; hashed credential in a single `users` row, set/rotated via `task auth/set-password`; sliding `HttpOnly` session cookie; session machinery in `core`, login flow in a new `auth` module. e2e authenticates once via global setup. | [ADR-0007](./adr/0007-single-local-login.md) |
| **Account kind & savings overrides** | Inline per-row picker on `/accounts`: kind (cash/credit/other) re-buckets + recomputes net cash; counts-as-savings toggle on cash/other rows. Overriding to `credit` force-clears the savings flag; an effective flag change eagerly re-pairs transfers through an injected seam. | [ADR-0008](./adr/0008-account-kind-and-savings-overrides.md) |
| **Category spend drill-down** | `home`-owned drill view at `/wraps/{ym}/spend/{bucket}` reached from wrap + Tracker Category figures; one `{bucket}` selector (Category / uncategorized / current-month budget residual); editable rows re-render the region so the net total stays reconciled. | [ADR-0009](./adr/0009-category-spend-drill-down.md) |
| **Accounts overview enhancements** | Free cash (net cash − total savings) headline + total-savings figure; account-name disambiguation via subtype + Plaid `mask` (last-4); per-account one-click hide/unhide (separate Hidden section, excluded from totals + transfer-destination pickers). | `two-cents-accounts-overview` |
| **Transactions view (search + needs-attention + month groups)** | `/transactions` merchant search + a Needs-attention worklist filter (`?view=needs-attention`, deep-linkable), both querying full history; rows grouped under month dividers; resolving a row in the worklist drops it live. | `two-cents-transactions-view` |
| **Reusable transaction-editing modal** | One modal shell for transaction edits, opened from any surface; regions refresh via the event-driven / OOB pattern rather than page reloads. | [ADR-0010](./adr/0010-event-driven-cross-region-refresh.md), [ADR-0011](./adr/0011-reusable-transaction-editing-modal.md) |
| **Wrap income / savings / full-month drill-ins** | Wrap figures (income, savings, full-month) open drill-in detail views. | [ADR-0012](./adr/0012-wrap-income-savings-and-month-list-drill-ins.md), `two-cents-wrap-drill` |
| **Richer transaction detail** | Read-only bank display detail in the editor: raw descriptor, merchant logo / website / entity id, payment channel, categorization confidence, authorized/posted timestamps, structured counterparties ("merchant via DoorDash"). Display-only (not a categorization input); joins the sync upsert. Field set **validated against real production data**. | [ADR-0013](./adr/0013-richer-bank-transaction-detail.md) |
| **Bottom-bar navigation** | Fixed bottom navigation with a More overflow sheet; app-wide chrome mounted once in the shared layout. | [ADR-0014](./adr/0014-bottom-bar-navigation.md), `two-cents-bottom-nav` |
| **App-wide request feedback** | Lifecycle-driven top progress bar (visible while any HTMX request is in flight, clears on settle regardless of outcome) + a "Sync now" in-progress state and transient inline confirmation. | [ADR-0015](./adr/0015-app-wide-request-feedback.md), `two-cents-sync-feedback` |
| **Tracker: Total remaining + budget-used bars** | Tracker shows a Total remaining figure and per-row budget-used progress bars. | direct commit (not a `/build`) |
| **Rule editor modal + rule-aware transaction editor** | Rule create/edit as one reusable modal (second consumer of the modal shell), opened from the Rules page and from the transaction editor — which lists the Rules governing a transaction (winner marked, each editable) and offers a prefilled create when none match. Opening from a transaction returns to it on save/dismiss via an opaque same-origin handle; a Rule change announces the change event so transaction views refresh. | [ADR-0016](./adr/0016-rule-editor-modal-and-cross-modal-return.md), `two-cents-rule-editor-modal` |
| **Custom account names** | Rename any Account from the overview via a nullable `custom_name` column (non-NULL *is* the override; sync never touches it); one resolver returns custom-else-bank name, read through the accounts module everywhere; empty input clears back to the bank name. `mask` (last-4) still disambiguates. | [ADR-0017](./adr/0017-custom-account-names.md) |
| **Real-Plaid (production) validation** | Connect + account/balance shapes, transactions sync, categorization, transfer pairing, and budget/tracker/wrap exercised end-to-end against **real production-bank data** (config-only switch to `PLAID_ENV=production`). Findings filed and folded into the slices above (request-feedback, custom names + disambiguation, free-cash / total-savings, hide-account, wrap drill-ins, transactions search + month headers). Remaining open findings tracked in the backlog below. | `two-cents-real-plaid-validation` |
| **Month-navigable home** | Tracker + per-month wraps unified into one month-rail surface (current → Tracker at `/`, earlier → `/wraps/{ym}`; earliest txn month → current, no future); standalone wraps list removed; **Home** nav → **Spending** (cash-coin icon). Each **wrap** gains a colour-coded **Surplus** figure (net income − savings contributed), and its Spending figure scrolls to the full-month list. Tracker reworked into two tiers — income/savings progress (each drills into its legs) over a uniform Budget section with a gap-separated Total-remaining row. | [ADR-0018](./adr/0018-month-navigable-home.md) |
| **Transactions list on the Tracker** | The current-month Tracker carries the wrap's inline, editable full-month list (the shared `AllTransactionsFrag`, header "Transactions", every classification, both budget modes) in a self-refreshing region; an edit reconciles the figures via `transaction-changed`. Its rows are the transactions module's canonical `TransactionRowFrag` — as are the wrap month list and the spend drill-down, so every transaction-row surface (the `/transactions` tab, wrap, Tracker, and drill) shares one row component with unified chips + colours; the drill's positive net-total header stays, its rows display-sign like the rest. `CurrentMonthTracker` now reads the month once through `MonthTransactions` (the wrap's joined read) for both figures and list — the separate `ActivityRow` / `TransactionsInRange` read path is removed, so the current month excludes orphaned post-disconnect rows from the budget like every wrap. | [ADR-0010](./adr/0010-event-driven-cross-region-refresh.md), [ADR-0012](./adr/0012-wrap-income-savings-and-month-list-drill-ins.md) |
| **Transaction-row avatars** | Leading avatar on every transaction row: merchant logo when cached, otherwise a category-colored glyph (glyph + color a static in-code map; custom-category color deterministic; classification defaults). Distinct icons + colors for income, transfer, and savings. Logos proxied + cached on-origin; warmed post-sync. One shared avatar element lands on all transaction-row surfaces at once. | [ADR-0019](./adr/0019-transaction-row-avatars.md) |
| **Monthly cash-sweep recommendation** | Advisory only — never moves money. A scheduled job on the 7th (app timezone) computes and persists `suggested_sweep = current_checking − reserve − fixed_safety_margin` (reserve = unspent budget + unmet savings target, each floored at 0) with a checking↔savings direction; `/sweep` shows the latest with its full breakdown, an empty first-run state, and a needs-attention state when checking/savings can't be derived from the account model. Reads no card/liability balance; the budgeted savings transfer is reserved for the user. New `sweep` domain module (the first persisted read-projection). | [ADR-0020](./adr/0020-monthly-cash-sweep-recommendation.md), `two-cents-cash-sweep` |

Covers PRD user stories 1–44 and spending-by-category aggregation (the wrap).

---

## 🔜 Committed, not yet built

Things v1 intends (named in the PRD/ADRs) that aren't built yet:

_All committed v1 work is now built — this section is empty until the next commitment lands._

---

## 🧊 Backlog (deferred / out of scope for v1)

From the PRD's *Out of Scope*, the domain model's deferred notes, and the slices' *Known gaps*:

**Near-term candidates (usability):**
- **Home needs-attention alert.** When the current month has uncategorized or otherwise-incomplete
  transactions, surface an alert on the home Tracker that deep-links to the needs-attention worklist
  (`/transactions?view=needs-attention`, already shipped). Open finding from the real-Plaid validation run.
- **Sync result count / last-synced time.** The in-flight signal shipped ([ADR-0015](./adr/0015-app-wide-request-feedback.md):
  progress bar + "Sync now" in-progress state + transient confirmation), which fixed the
  "looks like nothing happened" problem. Still deferred, as ADR-0015 notes: a concrete **"Synced (n updated)"**
  count (needs a sync summary the service doesn't return today) and a surfaced **last-synced time**.
- **Rules matching richer transaction detail** ([ADR-0013](./adr/0013-richer-bank-transaction-detail.md)
  deferred note). Today rules match only the cleaned merchant (Plaid `merchant_name` → e.g. `Two Boots`),
  so the platform/intermediary and raw descriptor we now ingest are *shown* but not *matchable* — a
  "DoorDash → Dining" rule can't catch a `DD *DOORDASH …` order. Feed the structured `counterparties`
  (the typed marketplace/payment-app entries) and/or the raw `description` into the rule engine, with an
  explicit precedence decision (platform vs sub-merchant). Its own slice; keeps the display-only-vs-input
  boundary deliberate.
- Transactions **pagination** (the unfiltered default list is still capped at the recent 100; search +
  needs-attention now query full history — see Shipped) and **per-account drill-down**.
- **Transaction groupings / spending events.** Tag transactions into an ad-hoc named group that cuts
  across categories and months (e.g. an "Italy trip") and see the group's total spend. A new capability,
  not polish on an existing one — needs its own slice (likely a lightweight many-to-many tag on
  transactions + a group view). Captured idea (personal notes), not in the PRD.
- **Refund → prior-outflow pairing** (a refund inflow matched to its original purchase) — a named post-v1 gap.
- **External-account entity** for transfers to *unconnected* accounts (today you can mark a subtype, not a real destination).
- **Precise provider history window** for the wrap's `partial` flag (today an earliest-transaction heuristic; under-flags later connections — see tech debt).
- **Sync reconciliation safety net.** Transaction sync trusts Plaid's `/transactions/sync` cursor as
  the *sole* completeness mechanism. The design is self-correcting within that contract — writes are
  idempotent upserts keyed by the provider id, the cursor advances atomically with the row writes (so a
  partial apply never skips data), and `removed`-set deletes apply by id — but **nothing independently
  re-checks for drift**. There is no automatic full reconcile and no prune: the only full re-pull is
  manually clearing `transaction_sync_state` (used for migrations), and even that is *upsert-only*, so a
  row the bank silently drops without a `removed` event, or a cursor invalidated mid-pagination
  (`TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION`, today just left to retry from the last good cursor
  next pass), is never corrected. Candidates: a "force full reconcile" action that diffs a fresh full
  pull against the stored set and prunes orphans, and/or a periodic drift check. Robustness hardening,
  not a current bug — Plaid's cursor is contractually complete.

**Explicitly deferred (PRD out-of-scope):**
- **Budget rollover** / envelope carry-over (v1 is monthly, no rollover).
- **Historical per-week/day actual breakdown** (the week/day view is forward pace only).
- **Goal-based named savings** (beyond a single monthly savings target).
- **Bill / subscription detection & reminders.**
- **Category merge** (archive/rename only in v1).
- **Fuzzy / pattern merchant matching** in rules (v1 is substring on the cleaned merchant).
- **Investments / holdings & liabilities detail** (loan APR, interest breakdown).
- **Payments / money movement** initiated from the app.
- **Multi-user** / managing others' accounts.
- **Mobile app.**
- **Non-USD / non-US banks.**

---

## ⚠️ Known issues / tech debt

- **Flaky e2e** `transaction-categorization.spec.ts:67` ("manual re-categorization survives a later sync")
  — an htmx `selectOption → waitForResponse` race; predates the budget slices. Suite is green with
  `--retries=2`. Candidate for a `/diagnose`.
- **Manual categorization/transfer overrides can be lost on pending → posted.** The sync model assumes
  a transaction moves pending → posted *in place* (same provider id), so the override facets survive
  (they're excluded from the upsert). But when an institution reissues a **new** provider id for the
  posted transaction (Plaid links the two via `pending_transaction_id`), the pending row is deleted via
  the `removed` set and the posted row arrives as a fresh `added` row — so a manual re-categorization
  (and any transfer-destination override) on the pending row is silently dropped and the posted row
  re-categorizes from scratch. Fix candidate: when applying an `added` row that carries a
  `pending_transaction_id`, carry the superseded pending row's override facets forward.
- **Disconnect hard-deletes accounts** instead of the domain's terminal `closed` state (a dangling
  transfer-destination FK) — see [`architecture/known-gaps.md`](./architecture/known-gaps.md).
- **`PartialFlag` under-flags** later-added connections' backfill-edge months (correct for the common
  single-connection case; a precise history window is in the backlog above).
- Architectural violations, as they arise, are tracked in
  [`architecture/known-gaps.md`](./architecture/known-gaps.md).

---

## Pointers

- Feature intent & v1 scope — [`prd.md`](./prd.md) · direction — [`scope.md`](./scope.md)
- Domain language & decomposition — [`domain/README.md`](./domain/README.md)
- Decisions — [`adr/`](./adr/) · architecture rules — [`architecture/`](./architecture/)
- Per-feature build records — Scope→…→Done, as-built, and the per-slice design spec — `~/workshop/builds/two-cents-*/`
