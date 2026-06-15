# Transfer subtype pairing slice (2a) — design spec

Date: 2026-06-14. Status: **subagent-audited (READY-WITH-FIXES → fixes incorporated)** → ready for `/build`.

The vertical slice through **ADR-0003 layer 2** — destination/subtype resolution for Transfers.
It builds the **`ResolveTransferSubtype` pairing engine**, the second sticky facet
(`transferDestinationOverridden`), the `MarkTransferDestination` operation, the auto-pairing pass
inside sync, and the transactions-page UI to see/correct a Transfer's destination — so that a
Transfer into a counts-as-savings Account is detected as a **Savings contribution**.

This is the prerequisite the user chose to build **before** the Budget/Tracker/Month-wrap slice (2b):
the tracker's *savings-so-far* and the wrap's *savings-contributed* both read `SavingsContributed`,
which is only well-defined once savings contributions are resolved here.

Layer-1 transfer **classification** (bank `personal_finance_category` primary → `Transfer`, no
pairing) already shipped with the categorization slice. This slice adds **only** layer 2 (the
"where did it go" pairing). It touches no Budget/Tracker code.

## Goal (smallest genuinely-useful version)

Every outflow Transaction classified `Transfer` gets, automatically on sync, a resolved
**destination** (a connected Account, or *unknown*) and a **subtype** (*Savings contribution* when
the destination counts-as-savings, else *plain Transfer*). The user can **mark / correct** any
Transfer's destination — sticky, survives re-sync, independent of the categorization override. The
`/transactions` rows show the subtype/destination and surface unknown-destination Transfers to mark.
All browser-testable.

## What the domain doc already decides (not free choices)

From [`docs/domain/README.md`](../../domain/README.md) §Categorization (`ResolveTransferSubtype`),
§Transactions (`MarkTransferDestination`, the two independent facets), §Derived projections
(`SavingsContributed`), and [ADR-0003](../../adr/0003-two-layer-transfer-detection.md):

- **`ResolveTransferSubtype` is a Categorization policy — pure, writes nothing.** It pairs an inflow
  leg on **another connected Account** (exact amount, ±3-day window) to learn the destination, then
  derives the subtype from that Account's `counts-as-savings` flag.
- **Pairing is conservative** (ADR-0003): exact amount, ±3 days, and **ambiguous matches are left
  unmatched**. A false pair silently hides real spending — worse than a missed pair the user can fix.
- **Subtype lives on the source (outflow) leg only**, so aggregations count it once. The paired
  **inflow leg stays a plain Transfer — the excluded mirror, never the carrier.**
- **Destination decides subtype:** destination counts-as-savings → **Savings contribution**;
  otherwise (incl. destination `kind=credit` — a credit-card payment) → **plain Transfer**, no
  counted subtype; **no connected match → destination unknown**, cannot count as savings until the
  user marks it.
- **`transferDestinationOverridden` is a second, independent sticky facet** (destination + subtype),
  separate from `categorizationOverridden` (Classification + Category). Each survives re-sync, beats
  its auto path, and **locks only its own facet**. `MarkTransferDestination` sets this facet; it
  does not touch categorization, and `ReCategorize` does not touch this one.
- **Invariant unchanged: Categorization decides, Transactions writes.** `ResolveTransferSubtype`
  returns a decision; **Transactions** is the only writer of the transfer-destination/subtype
  columns (cross-domain write ledger — same shape as auto-categorize).
- **`SavingsContributed` = Σ source-leg Transfers with subtype = Savings contribution** in a month.
  This slice produces the field; 2b consumes it. (No tracker/wrap code here.)

## Decisions made for this slice (the docs leave these open; chosen, with rationale)

- **The pure `ResolveTransferSubtype` stays a leaf in `categorization` — no `accounts` import.** The
  engine takes the destination Account's `kind`/`counts-as-savings` as **plain input** (the caller
  supplies candidate inflow legs already annotated with their account's facets). So `categorization`
  gains **no** import of `accounts` or `transactions`; the isolation invariants are unchanged.
  **`transactions` orchestrates** the pairing — it already imports both `categorization` (the engine)
  and `accounts` (the facets). This mirrors how auto-categorize already works (pure engine in
  `categorization`, `transactions` assembles inputs and writes). The doc files the policy under the
  Categorization *domain*; the *pure function* lives in the `categorization` package, called directly
  (not via a DB-loading `Service.Resolve`, since the inputs are transactions'/accounts' data, not
  categorization's own tables).
- **Exact-amount match is compared in integer cents** (`round(amount*100)`), to avoid float wobble on
  the `banking.Money.Amount float64` values. Pairing condition: `cents(|outflow|) == cents(|inflow|)`.
- **The auto-pairing pass re-resolves every non-overridden outflow Transfer leg each sync**, not just
  newly-added ones — because a *later* sync may add the matching inflow leg that lets a
  previously-unknown outflow pair. **The candidate set is every _stored_ inflow Transfer leg on another
  connected Account within the window — loaded from the repo, not just the current pull** (so the
  inflow-first/outflow-later direction pairs too; see §4). The pass is bounded by the transfer count,
  cheap. Overridden legs (`transferDestinationOverridden`) are skipped. (Symmetry with
  `ApplyCategorization`'s "re-resolve from scratch, no provenance" stance.)
- **Subtype storage:** one `transfer_subtype` column, values `''` (non-transfer / not-yet-resolved),
  `'savings_contribution'`, `'plain'`. Destination is a nullable FK; **destination-unknown** is the
  observable state `classification='transfer' AND amount>0 AND transfer_destination_account_id IS NULL
  AND transfer_destination_overridden=0` (an unresolved source leg the UI prompts to mark). An unknown
  source leg carries subtype `'plain'` for aggregation (never counted as savings) but is rendered as
  "destination unknown".
- **Only outflow legs (amount > 0) carry a subtype.** Inflow Transfer legs (amount < 0) are the
  excluded mirror — left subtype `''`, never resolved, never counted. This keeps `SavingsContributed`
  a single-leg sum with no double counting.
- **fakebank gains a paired inflow leg** so pairing is exercisable end-to-end (see §6). It stays
  canned/deterministic, within the external-client archetype.

## What's currently true (starting point)

`transactions` has `classification` / `category_id` / `categorization_overridden` columns (categorization
slice) but **no** transfer-destination/subtype columns and **no** `transferDestinationOverridden`.
`categorization` resolves layer-1 Transfer classification (`TRANSFER_IN`/`TRANSFER_OUT`/`LOAN_PAYMENTS`
→ `Transfer`) but has **no** `ResolveTransferSubtype`. `accounts` exposes `kind`/`counts-as-savings`
on `Account` but no focused per-account facet read for pairing. fakebank's canned set has one
unilateral transfer-out (`fake-txn-transfer`, $500 Checking→savings) with **no matching inflow leg**.
The `/transactions` UI shows the categorization picker but nothing about transfer destination.

## 1. Module shape & dependency direction

No new module. Changes land in three existing modules + fakebank + the architecture test:

- **`categorization`** (decider): add the pure `ResolveTransferSubtype` + its input/decision types and
  a `TransferSubtype` string type, in `categorization.go` (the topic file, beside `ResolveCategorization`).
  **No new imports** (stays `core/*` + `banking` only — explicitly *not* `accounts`, *not* `transactions`).
- **`accounts`** (read side): add a public `Service` method exposing per-Account pairing facets for
  connected (non-closed) Accounts — `id`, `kind`, `counts-as-savings`, display `name`. Owns its rows;
  no new cross-domain edge.
- **`transactions`** (writer + orchestrator): new columns + sqlc; the auto-pairing pass in
  `SyncTransactions`; `MarkTransferDestination`; read-model additions; the override-aware reconcile;
  the new HTTP route + view. Already imports `categorization` and `accounts`.
- **`fakebank`**: add the mirror inflow leg(s).
- **`architecture/isolation_test.go`**: extend `TestCategorizationDependencyDirection` to also assert
  `categorization` imports neither `accounts` nor `transactions` (locks the pure-leaf decision), and
  confirm the no-provider sweep still covers it. (`transactions`→`accounts`/`categorization` edges are
  already asserted; this slice adds none.)

**Dependency check:** the only cross-domain reads are `transactions` → `categorization.ResolveTransferSubtype`
(pure call) and `transactions` → `accounts.Service` (facets) — both pre-existing edges. The acyclic DAG
(`accounts` leaf; `transactions` imports `accounts` + `categorization`; `categorization` imports neither)
is preserved.

## 2. Data model

New goose migration (`task db/create -- transaction_transfer_destination`) + sqlc query additions in
`db/queries/transactions.sql` (`task build/sqlc`, `task db/up` — which also snapshots `db/schema.sql`).

**`transactions` additions** (owned/written only by `transactions`):

| column | notes |
|---|---|
| `transfer_destination_account_id` TEXT NULL REFERENCES accounts(id) | the paired or user-marked destination Account; NULL = unknown |
| `transfer_subtype` TEXT NOT NULL DEFAULT `''` | `''` (non-transfer / unresolved) \| `'savings_contribution'` \| `'plain'`; set only on outflow Transfer legs |
| `transfer_destination_overridden` INTEGER NOT NULL DEFAULT 0 | the second sticky facet, independent of `categorization_overridden` |

No CHECK on subtype values (mirrors the existing `classification` column's no-CHECK style; validity is
enforced in Go). The FK is nullable and un-cascaded to match the existing nullable `category_id` FK
convention. **Note (audit Q1):** sqlite FKs here are declarative-only (`core/db` opens the DB without
`_foreign_keys=on`), and `accounts.Disconnect` currently **hard-deletes** account rows (it does not set
them `closed`) while leaving transactions in place. So after a disconnect a saved
`transfer_destination_account_id` can dangle: the destination-name JOIN returns empty → a past Savings
contribution renders with a blank destination name. This is a **display** degradation only — the
contribution is still summed correctly (the future `SavingsContributed` keys on `subtype`, not the
join) and is **not** re-flagged "unknown" (the column is non-NULL). Acceptable for v1; do not add cascade.
(The Disconnect hard-delete vs. the domain's `closed` state is a pre-existing divergence, out of scope here.)

New/updated sqlc queries: a query to **list candidate Transfer legs** for the pairing pass (id,
account_id, amount, date, classification, transfer_destination_overridden — connected accounts only,
window-bounded by the caller), and `SetTransactionTransferDestination` / `OverrideTransactionTransferDestination`
writes (auto vs. manual, the latter setting the override flag) — named to match the existing
`SetTransactionCategorization` / `OverrideTransactionCategorization` pair in `db/queries/transactions.sql`.

## 3. The engine — `ResolveTransferSubtype` (pure, in `categorization`)

```go
type TransferSubtype string
const (
    SubtypeNone               TransferSubtype = ""                     // non-transfer / unresolved
    SubtypeSavingsContribution TransferSubtype = "savings_contribution"
    SubtypePlain              TransferSubtype = "plain"
)

type TransferLeg struct {        // a candidate inflow leg, annotated with its account's facets
    TransactionID   string
    AccountID       string
    AmountCents     int64         // round(|amount|*100); inflow leg
    Date            time.Time
    CountsAsSavings bool          // destination account's flag — the ONLY savings/plain discriminator
}

type TransferSubtypeInput struct {
    SourceAccountID string
    AmountCents     int64         // round(|outflow amount|*100)
    Date            time.Time
    Candidates      []TransferLeg // inflow Transfer legs on OTHER connected accounts
    WindowDays      int           // 3
}

type TransferSubtypeDecision struct {
    DestinationAccountID *string         // nil = unknown
    Subtype              TransferSubtype // SavingsContribution | Plain (never None for a resolved source leg)
}

func ResolveTransferSubtype(in TransferSubtypeInput) TransferSubtypeDecision
```

Logic (pure, side-effect free):
1. Filter `Candidates` to legs with `AccountID != SourceAccountID`, `AmountCents == in.AmountCents`,
   and a **calendar-date** difference ≤ `WindowDays` (inclusive). Diff on the date (truncate to the
   day in the app's reckoning), **not** a raw 24h `time.Sub`, so "±3 days inclusive" stays exact even
   if a real Plaid row carries a time component (audit Q3). (fakebank dates are midnight, so either
   works there.)
2. **Exactly one** match → destination known: `DestinationAccountID = &leg.AccountID`; subtype =
   `SavingsContribution` if `leg.CountsAsSavings` else `Plain` (a credit destination simply has
   `CountsAsSavings == false`, so it falls out as `Plain` — no separate kind check).
3. **Zero or more-than-one** match → `DestinationAccountID = nil`, subtype = `Plain` (unknown,
   conservative — never count an ambiguous/missing pair as savings).

Callers pass only **outflow** legs (amount > 0) that are classified `Transfer` and **not** overridden.
Exhaustively unit-tested (table-driven): savings vs. plain vs. credit-destination, exact-amount
boundary, ±3-day inclusive boundary, same-account exclusion, ambiguous (2 matches) → unknown,
zero matches → unknown.

## 4. Transactions wiring (the ledger write + the manual override)

- **Auto-pairing pass in `SyncTransactions`.** After the categorize step, run a transfer-subtype pass:
  fetch the connected-account facets from `accounts.Service`; **load every stored inflow Transfer leg
  in the window from the repo** (the new candidate query); for each **outflow Transfer leg without
  `transfer_destination_overridden`**, build the inflow-candidate list (other connected accounts,
  annotated with each account's `counts-as-savings`), call `categorization.ResolveTransferSubtype`, and
  write `transfer_destination_account_id` + `transfer_subtype` via the auto-write query. Overridden
  legs are skipped. Re-runs every sync (a later inflow can resolve an earlier unknown).
- **Facet preservation is by the existing emergent pattern, not a `PendingReconcileMatch` function**
  (there is none — audit S1). Stickiness today works because `UpsertTransaction` deliberately **omits**
  the categorization columns from its INSERT/ON-CONFLICT (`db/queries/transactions.sql`, see its
  comment) so a re-sync cannot clobber them, and the auto pass skips already-resolved/overridden rows.
  This slice follows suit: the new `UpsertTransaction` must **NOT** add the three transfer columns to
  its INSERT/ON-CONFLICT, and the auto-pairing pass skips `transfer_destination_overridden` rows. The
  two facets stay independent because they are distinct columns written by distinct paths — never fix
  this by adding the columns to the upsert.
- **`MarkTransferDestination(ctx, txnID string, destinationAccountID *string, subtype categorization.TransferSubtype) error`** —
  the manual override. Validates the row is an outflow `Transfer`; sets destination + subtype; sets
  `transfer_destination_overridden = 1`. Sticky — the auto pass never reverts it. Does **not** touch
  `categorization_overridden`. (Marking a destination counts-as-savings is how the user attributes an
  unconnected-account transfer to savings.)

## 5. Read model

`RecentTransaction` (the `/transactions` row read model) gains, for Transfer rows:
- `TransferSubtype categorization.TransferSubtype`
- `TransferDestinationName string` (joined display name of the destination Account, empty if unknown)
- `TransferDestinationUnknown bool` — computed from `classification='transfer' AND amount>0 AND
  transfer_destination_account_id IS NULL AND transfer_destination_overridden=0`. **Key it on the
  destination column, never on `transfer_subtype`** (audit Q2): `'plain'` is overloaded across
  resolved-non-savings and unknown legs, so the subtype can't tell them apart — the NULL destination can.

so the row renders "→ Savings (High-Yield Savings)", "→ Travel Rewards Card", a plain "Transfer", or
"Transfer · destination unknown — mark". The list query joins `accounts` for the destination name
(read-only join; `transactions` already joins `accounts` for the source account name).

## 6. fakebank (deterministic test data for pairing)

Add to the canned set so pairing is exercisable:
- **A matching inflow leg for the existing $500 transfer**: a `TRANSFER_IN` row on `fake-savings`,
  amount `-500.00`, date within ±3 days of `fake-txn-transfer` (Jun 4), e.g. same day —
  `id: fake-txn-transfer-in`, merchant "Transfer from Checking", `TRANSFER_IN`/`TRANSFER_IN_ACCOUNT_TRANSFER`.
  → the $500 Checking outflow now pairs to `fake-savings` (counts-as-savings) ⇒ **Savings contribution**.
- **(Optional, if it keeps the e2e crisp) a credit-card payment pair**: an outflow `TRANSFER_OUT` on
  Checking + matching `TRANSFER_IN` on `fake-credit`, so a *plain* (non-savings) resolved transfer is
  also demonstrable. Include only if it doesn't bloat the canned set or perturb existing categorization
  e2e assertions; otherwise leave the credit/plain path to unit tests.

**This is NOT additive — adding a 6th canned row breaks existing hard-count and positional-index
assertions, which must be updated in the same change** (audit M1). The dependent sites:
- `e2e/spec/transactions.spec.ts` — `transactions-row` `toHaveCount(5)` (≈ lines 38, 62, 73) → 6.
- `e2e/spec/transaction-categorization.spec.ts` — `toHaveCount(5)` (≈ 45, 48) → 6, **and** the
  positional row indices (`ROW_SIDE_HUSTLE`/`ROW_TRANSFER`/`ROW_WHOLE_FOODS` ≈ 13-15) must be
  re-derived: with `ORDER BY date DESC, id DESC`, `fake-txn-transfer-in` sorts ahead of
  `fake-txn-transfer`, so every index at/after the insertion point shifts.
- `src/internal/transactions/adapters/transactions_page_test.go` — `strings.Count(body, "transactions-row")`
  == 5 (≈ 157, 273) → 6.
- `src/internal/fakebank/service_test.go` — `len(changes.Added)` == 5 (≈ 172) → 6.
- `src/internal/fakebank/service.go` — the doc comment on the canned set (≈ 90-106) that says "change
  it only with the dependent tests" — update it to describe the new pair.

Prefer **stable, testid/merchant-based selection** over positional indices when fixing the categorization
spec, so the next canned-set change doesn't re-break it. Keep the set deterministic.

## 7. UI (HTMX-first, fragments over pages, inline errors, theme tokens — `docs/design/`)

On `/transactions`, each **Transfer** row shows its destination/subtype as a chip and offers a
**mark-destination** control (mirrors the re-categorize picker, but the independent facet):
- Chip states: `→ Savings · <account>`, `→ <account>` (plain), `Transfer` (plain, destination among
  connected but not savings), and the flagged `destination unknown — mark` for unresolved outflows.
- The control opens a picker to choose a destination Account (the connected-account list from
  `accounts.Service`) and/or mark the subtype (Savings contribution | plain Transfer).
  `POST /transactions/{id}/transfer-destination` → `MarkTransferDestination` → htmx-swap that row's
  fragment (inline error, no redirect). Non-transfer rows show no such control.
- Picker/account list comes via the transactions handler's existing `accounts.Service` dependency
  (already injected — see `start.go`).
- **Testids** (register in `docs/design/testids.md`): `txn-transfer-destination` (chip),
  `txn-mark-destination` (control), `txn-destination-unknown` (the prompt state),
  `txn-destination-picker`, plus the picker's account-option ids.

## 8. Testing (gate: `go build ./...`, `go test ./src/...`, `task test/e2e` all green)

- **Go unit (pure):** `ResolveTransferSubtype` table-driven — the full pairing matrix in §3 (savings,
  plain, credit-destination, exact-amount boundary, ±3-day inclusive boundary incl. day 4 = no match,
  same-account exclusion, ambiguous → unknown, zero → unknown).
- **Go integration (repo + service over temp sqlite, mirror existing transactions tests):**
  auto-pairing on sync resolves the fakebank savings pair → source leg `savings_contribution` with the
  savings destination, inflow mirror untouched (subtype `''`); a later-arriving inflow resolves a
  previously-unknown outflow on re-sync; `MarkTransferDestination` stickiness across a re-sync
  (`PendingReconcileMatch` preserves `transferDestinationOverridden`, re-runs the rest); the two
  override facets are **independent** (overriding categorization does not lock the transfer facet and
  vice-versa); ambiguous/no-match leaves destination unknown.
- **Cross-cutting invariant test:** a Savings-contribution source leg is counted once and its inflow
  mirror is never counted (the precondition `SavingsContributed` in 2b will rely on).
- **e2e (`BANK_PROVIDER=fake`)** — new `e2e/feat/transfer-destination.feature` + matching
  `.spec.ts`: connect (fake) → `/transactions` shows the $500 transfer auto-resolved to
  `→ Savings` → re-categorize/mark flow: mark a destination on an unknown transfer (seed one via
  `helpers/db.ts` if the canned set has none) and assert the chip updates and **survives Sync-now**
  (facet stickiness). Reuse the existing seeding/connect/sync helpers.
- **Architecture test:** extend `TestCategorizationDependencyDirection` (no `accounts`/`transactions`
  import from `categorization`); confirm the no-plaid sweep still green.

## Out of scope (named, so it isn't mistaken for missing)

Budget / Tracker / Month-wrap (slice 2b — consumes `SavingsContributed` produced here). Configured app
timezone (ADR-0004 — lands with 2b, where "today/days-left/month" first matter). Pairing **across more
than two legs**, fuzzy amount/date matching, and inflow→prior-outflow **refund** pairing (a named
post-v1 gap, distinct from transfer pairing). Marking a destination to an **unconnected** account by
free-text label (v1 marks a subtype + optionally a connected account; a richer "external account"
entity is post-v1).
