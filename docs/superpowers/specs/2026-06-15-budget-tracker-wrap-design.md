# Budget + Tracker + Month-wrap slice (2b) — design spec

Date: 2026-06-15. Status: **subagent-audited (READY-WITH-FIXES → fixes incorporated)** → ready for `/build`.

The vertical slice that turns categorized + transfer-resolved transactions into a **plan and a
scorecard**: a single rolling **Budget** config, the current-month **Tracker** (am I on track), and the
retrospective **Month wrap** (how the month went). It consumes the savings-contribution signal that
slice 2a (transfer pairing) now produces, so the savings figures are real.

Domain authority: [`docs/domain/README.md`](../../domain/README.md) §Budget, §"Derived projections"
(Tracker + Reporting derivation cards), [`docs/prd.md`](../../prd.md) (user stories 27–44),
[`docs/architecture/data-model.md`](../../architecture/data-model.md), and ADR-0004 (configured app
timezone).

## Goal (smallest genuinely-useful version)

The user sets a monthly Budget (income target, savings target, optional per-Category spending limits).
The **home page** (`/`) shows this month's Tracker: remaining per budgeted Category and for "Everything
else", total remaining, daily/weekly **pace** targets, **income** and **savings** progress vs. target,
and over-budget flags — all computed in the **configured app timezone**. A **Wraps** section lists past
months and shows each month's **actuals** wrap (net income, savings contributed, spend by Category) with
**settling**/**final** and **partial** markers. With no Budget set, the Tracker shows actuals-only and
prompts to create one. All browser-testable against the fake provider.

## What the domain doc already decides (not free choices)

- **Budget is a single rolling config** (income target, savings target, per-Category limits), applied to
  the **current month**, persisting/carrying forward, **no rollover**. Optional. → new `budget` domain
  module. `ComputeResidual` ("Everything else" = income − Σ active-Category limits − savings; total
  spending budget = income − savings); `BalanceCheck` (Σlimits + savings > income → over-allocated,
  **surfaced not enforced**). An archived Category's limit is **inert** (skipped) and revives on
  un-archive.
- **Tracker and Reporting are pure utility modules** (no Service, no repo, no DB, no `adapters/`) — the
  read-side derivation cards. A **composing module** injects the domain services, fetches the data, and
  **passes it in**; the projections return view models it renders.
- **Tracker (current-month, forward-looking):** `Remaining` (limit − net spend per Category + total),
  `EverythingElseRemaining` (residual − unbudgeted&uncategorized spend), `PaceTarget`
  (`max(0,remaining) ÷ days-left-inclusive`; weekly = daily×7; **spending only**), `IncomeProgress` /
  `SavingsProgress` (so-far vs target; savings so-far = Σ this month's Savings-contribution legs),
  `OverBudgetFlag`. **Budget-relative cards are defined only when a Budget exists**, and count only
  **active**-Category limits.
- **Reporting / Month wrap (any month, retrospective, ACTUALS ONLY — never vs budget):** `NetIncome`
  (total Income − total Spending, Transfers excluded both sides), `SavingsContributed` (Σ source-leg
  Savings contributions in month), `SpendByCategory` (Σ net spend per Category, refunds negative),
  `WrapState` (any pending → settling; all posted → final), `PartialFlag` (connect month or backfilled
  edge month → possibly incomplete).
- **Shared basis `MonthAssignment`:** a Transaction belongs to the calendar month of its **transaction
  date** (not posted date). Transfers excluded from spend/income; refunds are negative Spending; savings
  measured by movement (the 2a Savings-contribution subtype).
- **Time basis (ADR-0004):** "today / days-left / current month" are reckoned in a **single configured
  app timezone**, a persisted setting (default EST), available to background jobs — not a per-request
  browser zone.
- **Account state is not a flow filter:** aggregations count every existing Transaction regardless of
  its Account's hidden/closed state.
- **Income excludes Transfers; refunds are negative Spending; savings = movement** (already enforced by
  classification + the 2a subtype).

## Decisions made for this slice (the docs leave these open; chosen, with rationale)

- **Configured app timezone = a new `core/timex` + a `Config.AppTimezone`.** New env `APP_TIMEZONE`
  (IANA name, default `America/New_York` — the ADR's "EST"), loaded into `Config` as a `*time.Location`
  at startup (fail-soft to the default on a bad name, logged). `core/timex` holds the pure time logic so
  it is unit-testable with an injected `now`: `CurrentMonth(loc, now) (year, month)`,
  `MonthRange(year, month) (start, end time.Time)`, `DaysLeftInclusive(loc, now) int` (today counts;
  clamped ≥1 on the last day). ADR-0004 settles the zone; this is where month/day math first matters.
- **CRITICAL — month bucketing compares CALENDAR DATES, not zoned instants (audit M1).** Transaction
  dates are stored as **zoneless calendar dates at midnight UTC** (`plaid/entities.go` `parseDate`,
  `fakebank` `time.Date(...UTC)`). So `loc`/`now` decide **which** month (`CurrentMonth`) and
  **days-left** (`DaysLeftInclusive`) — but the range-filter boundaries `MonthRange` returns must be
  **UTC-midnight** (`[2026-06-01T00:00:00Z, 2026-07-01T00:00:00Z)`), i.e. compared as calendar dates,
  **never** a `loc`-zoned instant. A `loc`-zoned boundary (e.g. `2026-06-01T00:00-04:00` = `…T04:00Z`)
  would mis-bucket a June-1 UTC-midnight row into May — the exact off-by-one ADR-0004 says cannot
  happen *because both sides are calendar-date-based in one reckoning*. `MonthRange` therefore takes
  `(year, month)` only (no `loc`); the `loc` is used solely to pick the current `(year, month)` and
  compute days-left. The §7 "MonthRange boundaries / DST" test must assert UTC-midnight boundaries.
- **`DaysLeftInclusive` is a calendar-date difference, not a duration division (audit Q2).** Reuse the
  existing midnight-reduction pattern in `categorization.go` `calendarDayDiff` (reduce both dates to
  their own day before differencing) so a DST transition can't miscount; today counts (inclusive),
  clamped ≥1 on the last day of the month.
- **Composing module = new `home` domain-module-shaped composer.** It owns no domain tables; it injects
  `budget` + `transactions` + `categorization` + `accounts` services + the app timezone, and serves the
  **dashboard pages**. Routes: **`GET /{$}`** (the current-month Tracker — the new landing),
  **`GET /wraps`** (month list), **`GET /wraps/{ym}`** (a single month wrap, `ym` = `YYYY-MM`). It calls
  the pure `tracker` / `reporting` functions. (Per the IA decision below.)
- **IA (chosen with the user): the Tracker is the home page `/`; the accounts overview moves to
  `/accounts`.** Only the **route** for the overview changes (in `accounts/adapters`, `GET /{$}` →
  `GET /accounts`); the overview page/code stays in `accounts`. `home` owns `/` and `/wraps[/{ym}]`;
  `budget` owns `/budget`. (Minor divergence from the doc's "home serves the overview too" — recorded as
  a known gap, not worth the churn of relocating working overview code now.) Navbar becomes **Home,
  Accounts, Transactions, Budget, Wraps, Categories, Rules**.
- **Pure projections receive a date-range row set, not pre-aggregated sums.** The composing module
  computes the month's `[start,end)` via `timex`, calls a new `transactions.Service` range read, and
  passes the rows to the pure `tracker`/`reporting` functions, which do **all** aggregation
  (`SpendByCategory`, income/savings sums, residual draw) and math. This keeps each derivation card in
  its utility module (per the doc) and keeps `transactions` a row provider. The row read is bounded to a
  month.
- **Money stays `float64`** (existing `banking.Money` convention); budget amounts stored as SQLite
  `REAL`. Not relitigated here.
- **Budget storage:** a single-row `budget` table (fixed id `'default'`, `income_target REAL NOT NULL
  DEFAULT 0`, `savings_target REAL NOT NULL DEFAULT 0`, timestamps) + a `budget_category_limits` table
  (`category_id TEXT` PK → categories(id), `limit_amount REAL NOT NULL`). "No budget set" = the
  `budget` row absent (or untouched defaults + no limits); the tracker treats a missing config as
  actuals-only. Editing upserts the single row + replaces the limit set.
- **`PartialFlag` rule (concrete):** a month is **partial** if it is the **connect month** of the
  earliest-connected Connection (its `created_at` month, in app tz) **or earlier** — i.e. at/over the
  backfill edge of provider history; months strictly after are treated as complete. Cheap, and right for
  the single-connection common path (the fake provider, most real users at v1). **Caveat (audit Q1):**
  with *multiple* connections added at different times, a later connection's own backfill-edge months
  (after the first connect) are not flagged — the heuristic can **under**-flag there; a precise
  per-connection history window is explicitly out of scope (Plaid doesn't expose it). Don't call it
  "conservative" — it's a cheap single-connection approximation.
- **Empty-DB edges (audit S2):** with **no transactions**, the `Wraps` list is just the current month
  (no earlier bound); `EarliestTransactionDate` returning no rows must be handled (not an error path).
  With **no connections**, `EarliestConnectedAt` returns `(_, false)` → `PartialFlag = false`.
- **`Wraps` list = the set of calendar months from the earliest transaction's month through the current
  month** (app tz), most-recent first; each links to `/wraps/{ym}`. The current month always appears
  (its wrap is naturally settling/partial as applicable); with no transactions, only the current month.

## What's currently true (starting point)

`/` is the accounts overview (`accounts/adapters`, `GET /{$}`). Transactions carry classification +
category + transfer subtype (savings contributions resolved by 2a). There is **no** `budget`,
`tracker`, `reporting`, or `home` module; **no** timezone config (`Config` has none; dates render in
server-local time); **no** budget/tracker/wrap pages; navbar = Overview/Transactions/Categories/Rules.

## 1. Module shape & dependency direction

- **`core/timex`** (new, under `core/`): pure time helpers (above). Imported by `home` (and any future
  cron consumer). No domain concepts.
- **`core/app` Config**: add `AppTimezone *time.Location` from `APP_TIMEZONE` (default America/New_York).
- **`budget`** (new domain module — standard archetype): `service.go` + `repo.go` (only file touching
  sqlc) + `budget.go` (entities `Budget`, `CategoryLimit`, pure `ComputeResidual`/`BalanceCheck`) +
  `adapters/{http.go,routes.go,views/}` + `CLAUDE.md` + `README.md`. Reads `categorization` (Category
  list — for limit attach + archived-skip). Imports `core/*`, `categorization`. **Not** `transactions`,
  **not** `accounts`.
- **`tracker`** (new utility module — pure): `tracker.go` (+ `tracker_test.go`). The Tracker derivation
  functions over passed-in inputs. **Imports NO domain package (audit M4):** only `core/*` (and the
  stdlib). Go imports are package-level — you cannot import "the budget type but not the budget service"
  — so the inputs are **locally-defined structs** in `tracker` (raw `CategoryID *string`, cents ints;
  Category *names* are joined later by `home`). No DB, no `adapters/`. This makes the isolation rule a
  clean "imports no domain package."
- **`reporting`** (new utility module — pure): `reporting.go` (+ tests). The wrap/period derivations
  over passed-in rows. Same rule — **no domain-package imports**, local input structs only.
- **`home`** (new composing module — domain-module-shaped, **owns no tables, no repo**): `service.go`
  (composes reads into tracker/reporting view models) + `adapters/{http.go,routes.go,views/}` +
  `CLAUDE.md` + `README.md`. Injects `budget` + `transactions` + `categorization` + `accounts` services
  + `*time.Location`. Imports those services + `tracker` + `reporting` + `core/*`. It is the **only**
  new module that may import multiple domain services (the legitimate composition root for the
  dashboard, mirroring how the doc describes the composing layer).
- **`transactions`**: add a **read-only** range query `TransactionsInRange(ctx, start, end time.Time)
  ([]ActivityRow, error)` returning the minimal fields the projections need (date, classification,
  category_id, amount, transfer_subtype, pending, account state is irrelevant — count all). Plus a
  helper to get the **earliest transaction date** (for the wraps list + partial-edge). No writes.
- **`accounts`**: change the overview route `GET /{$}` → `GET /accounts` (+ nav). **Expose the earliest
  connect time — this is genuinely new work (audit S1):** today `Connection{ID, ProviderItemID, State}`
  drops `created_at` and `connectionFromModel` doesn't map it (though `connections.sql` selects it). Add
  a `CreatedAt time.Time` field + mapping, and a service read `EarliestConnectedAt(ctx) (time.Time, bool)`
  (the `bool` false when there are no connections) for `PartialFlag`.

**Dependency direction (new acyclic edges):** `home` → {`budget`, `transactions`, `categorization`,
`accounts`, `tracker`, `reporting`}; `budget` → `categorization`; `tracker`/`reporting` → leaves
(`core`, `banking`, type-only). No cycles: `home` is a new top-level composer below `server`; the
utilities are leaves; `budget` only adds `budget → categorization` (categorization imports neither, so
acyclic). **Extend `architecture/isolation_test.go`:** `tracker` and `reporting` import **no domain package at
all** (only `core/*`/stdlib) — a clean, package-granular leaf assertion (audit M4); `budget` imports
neither `transactions` nor `accounts`; nothing imports `home` except `server`; no new module imports a
provider.

## 2. Data model

New migration (`task db/create -- budget`) + sqlc queries `db/queries/budget.sql`
(`task build/sqlc`, `task db/up`; updates `db/schema.sql`).

**`budget`** (single config row; owned/written only by `budget`):

| column | notes |
|---|---|
| `id` TEXT PK | fixed `'default'` (one rolling config) |
| `income_target` REAL NOT NULL DEFAULT 0 | monthly income target |
| `savings_target` REAL NOT NULL DEFAULT 0 | monthly savings target |
| `created_at`,`updated_at` TIMESTAMP | |

**`budget_category_limits`** (owned/written only by `budget`):

| column | notes |
|---|---|
| `category_id` TEXT PK REFERENCES categories(id) | the budgeted Category |
| `limit_amount` REAL NOT NULL | the monthly spending cap |

Queries: `GetBudget`, `UpsertBudget`, `ListCategoryLimits`, `ReplaceCategoryLimits` (delete-all +
insert the submitted set, in a tx). Plus `transactions` range queries (`TransactionsInRange`,
`EarliestTransactionDate`) in `db/queries/transactions.sql`.

## 3. The pure projections (`tracker`, `reporting`)

To keep the utilities true leaves with **no cross-domain service imports**, define their inputs as small
structs in the utility package (the composing module fills them):

```go
// tracker
type MonthSpend struct { CategoryID *string; NetCents int64 }   // nil CategoryID = uncategorized
type TrackerInput struct {
    Budget          *BudgetView          // nil => actuals-only mode
    Spend           []MonthSpend         // net spend per category in the month
    IncomeCents     int64
    SavingsCents    int64
    DaysLeftInclusive int
}
type BudgetView struct { IncomeTargetCents, SavingsTargetCents int64; Limits []CategoryLimitView /* active only */ }
func BuildTracker(in TrackerInput) TrackerView   // Remaining, EverythingElseRemaining, PaceTarget, progress, flags
```

Logic per the derivation cards: `remaining[c]=limit[c]−netSpend[c]`; everything-else =
`(income−Σlimits−savings) − (unbudgeted+uncategorized spend)`; pace `daily=max(0,remaining)÷daysLeft`,
`weekly=daily×7`; income/savings progress = so-far vs target; over-budget = `netSpend>limit`.
**Cents conversion is SIGN-AWARE (audit M2):** the existing `categorization.AmountCents` discards the
sign (magnitude only), so it is correct **only** for same-sign sums (income, savings). For
**net spend** the rows must be summed **signed** (`round(amount*100)` keeping sign — outflow positive,
**refund inflow negative**, so `purchase − refund`); use a signed `cents(amount)` helper, never
`AmountCents`, for spend/net-income. For income/savings totals the source amounts are **inflow
(negative)** / outflow legs — negate to a positive total as appropriate. Format back to dollars in the
view. **No-budget mode** (`Budget==nil`): return actuals (spend total, income, savings so-far) + a
`NeedsBudget` flag; omit remaining/pace/over-budget. **No-budget predicate (audit S4):** because
`SetBudget` always upserts the single `'default'` row, the live "no budget" test the composer uses is
`income==0 && savings==0 && len(limits)==0` (an all-zero config reads as no-budget); pin this exact
predicate so composer + e2e agree.

```go
// reporting
type WrapTxn struct { CategoryID *string; Classification string; AmountCents int64; TransferSubtype string; Pending bool }
type WrapInput struct { Txns []WrapTxn; Partial bool }
func BuildWrap(in WrapInput) WrapView   // NetIncome, SavingsContributed, SpendByCategory, WrapState(settling/final)
```

`NetIncome = ΣIncome − ΣSpending` (net refunds; transfers excluded); `SavingsContributed = Σ source-leg
savings_contribution`; `SpendByCategory` groups net spend; `WrapState = any pending ? settling : final`;
`PartialFlag` passed in (computed by the composer from connect month). Both functions are exhaustively
table-tested (the §Testing priority).

## 4. The `budget` domain module

- Entities `Budget{IncomeTarget, SavingsTarget float64}`, `CategoryLimit{CategoryID string, Limit
  float64}`. Pure `ComputeResidual(income, savings float64, activeLimits []CategoryLimit) (residual,
  totalSpendingBudget float64)` and `BalanceCheck(...) Balanced|OverAllocated`.
- `Service`: `GetBudget(ctx) (Budget, []CategoryLimit, error)` (returns a zero/empty config when unset),
  `SetBudget(ctx, income, savings float64, limits []CategoryLimit) error` (upsert row + replace limits;
  run `BalanceCheck` and surface the verdict to the handler; **non-blocking** — an over-allocated plan
  saves). Reads `categorization.ListCategories` to validate limit targets exist + to drop archived
  targets from the residual at read time (inert-while-archived).
- Adapter: **`GET /budget`** (the creator/editor: income, savings, a row per active Category with its
  limit) + **`POST /budget`** (save) → htmx swap showing the saved config + the residual + the
  balanced/over-allocated banner (inline). Mirror the categories/rules page idioms.

## 5. The `home` composing module + the pages

- `home.Service` methods: `CurrentMonthTracker(ctx) (TrackerView, error)` and `MonthWrap(ctx, year,
  month) (WrapView, error)` and `WrapList(ctx) ([]WrapSummary, error)`. Each: resolve the month
  `[start,end)` via `timex` + `Config.AppTimezone`; read `transactions.TransactionsInRange`; read
  `budget.GetBudget` + `categorization.ListCategories` (active, for names + the active-limit filter);
  compute `DaysLeftInclusive`; assemble the pure-function inputs; call `tracker.BuildTracker` /
  `reporting.BuildWrap`; map to a view model with Category **names** (joined from categorization) and
  formatted money.
- Pages (`home/adapters`): **`GET /{$}`** the Tracker dashboard (budgeted categories with
  remaining/pace/over-budget chips; Everything-else; totals; income & savings progress bars; a
  "set a budget" prompt + link when none); **`GET /wraps`** the month list (most-recent first, each with
  settling/final + partial badges, linking to the wrap); **`GET /wraps/{ym}`** a single wrap (net income,
  savings contributed vs target-not-shown/actuals-only, spend-by-Category table, state + partial
  badges). htmx-first, theme tokens, inline states; empty/no-data states handled.
- **Route move:** `accounts/adapters` overview `GET /{$}` → `GET /accounts`; update internal links.
  **The e2e blast radius is wider than the overview spec (audit M3):** every spec that bootstraps via
  `page.goto('/')` to reach the connect/disconnect/reconnect chrome (which lives on the overview) must
  switch to `/accounts` — `e2e/spec/{connect-bank,disconnect-bank,reconnect-bank,accounts-overview,
  transaction-categorization,transfer-destination}.spec.ts` and their `.feature` files (e.g.
  `connect-bank.feature` "the overview page at /"). The overview testid root is `accounts-overview-page`
  (not path-encoded), so it does **not** change. Audit-verify by grepping `goto('/')` and `goto("/")`
  across `e2e/spec/` and fixing each.
- **Navbar** (`core/templates/navbar.templ`): Home (`/`), Accounts (`/accounts`), Transactions, Budget
  (`/budget`), Wraps (`/wraps`), Categories, Rules. **Rename `nav-overview` → `nav-accounts`** (it now
  points at `/accounts`, not `/`) and add `nav-home`, `nav-budget`, `nav-wraps`; **edit
  `docs/design/testids.md`** to retire the `nav-overview` entry and register the new ones (audit S3),
  and update any e2e asserting `nav-overview`.
- **Composition root** (`server/`): construct `budget.Service`; construct `home.Service` injecting
  budget/transactions/categorization/accounts + `cfg.AppTimezone`; register `budget` + `home` routes;
  ensure construction order is acyclic (budget after categorization; home last among domain composers).

## 6. Testid + design

Register in `docs/design/testids.md`: `tracker-page`, `tracker-category-row`, `tracker-everything-else`,
`tracker-total`, `tracker-pace-daily`/`-weekly`, `tracker-income-progress`, `tracker-savings-progress`,
`tracker-over-budget`, `tracker-needs-budget`; `budget-page`, `budget-income`, `budget-savings`,
`budget-limit-row`, `budget-residual`, `budget-balance-banner`, `budget-save`; `wraps-page`, `wrap-row`,
`wrap-page`, `wrap-net-income`, `wrap-savings`, `wrap-category-row`, `wrap-state`, `wrap-partial`;
`nav-home`, `nav-accounts`, `nav-budget`, `nav-wraps`. Follow `docs/design/` (HTMX-first, fragments,
inline errors, theme tokens, money formatting helper).

## 7. Testing (gate: `go build ./...`, `go test ./src/...`, `task test/e2e` all green)

- **Go unit — the priority (pure, no DB):**
  - `core/timex`: `MonthRange` (boundaries, year wrap, DST months), `DaysLeftInclusive` (month start,
    last day → 1, mid-month), `CurrentMonth`.
  - `tracker.BuildTracker`: remaining (per cat + total), everything-else residual draw
    (unbudgeted+uncategorized), pace (`÷ days-left`; **last-day = ÷1**, over-budget clamps to 0 + flags),
    income/savings progress, **no-budget actuals-only** mode, archived-limit excluded.
  - `reporting.BuildWrap`: net income (refunds negative, transfers excluded), savings contributed
    (source-leg only — the 2a invariant), spend-by-category, settling vs final, partial passthrough.
  - `budget`: `ComputeResidual` (incl. archived-skip), `BalanceCheck` (balanced/over-allocated).
- **Go integration (repo + service over temp sqlite):** `budget` upsert + limit replace + archived-skip
  at read time; `transactions.TransactionsInRange`/`EarliestTransactionDate` correctness; `home.Service`
  end-to-end against the fake provider (current-month tracker numbers + a month wrap) — asserting the
  composed view, since the fake set (incl. the 2a savings pair, income, varied spend) falls in the
  current month.
- **e2e (`BANK_PROVIDER=fake`)** — new `e2e/feat/budget.feature`, `tracker.feature`, `wrap.feature`
  (+ specs): set a budget (income/savings/limits) → home `/` shows remaining/pace/over-budget,
  income+savings progress (savings reflects the auto-paired $500 contribution), and the
  Everything-else line; over-spend a category → over-budget flag; with no budget → the needs-budget
  prompt; `/wraps` lists the current month → open it → net income, savings, spend-by-category, the
  settling/partial badge (the fake set has a pending row → settling). **Update the moved overview e2e**
  to `/accounts` and the nav assertions. Reuse the connect/sync/seed helpers; prefer testid/merchant
  selectors.
- **Architecture test:** the §1 dependency assertions (utilities pure; `budget` isolation; `home` only
  imported by `server`; no provider imports).

## Suggested build decomposition (goal seam — audit Q3)

This slice is large; build it as a foundation goal + two consumers so the riskiest correctness fixes
land pure and table-tested before any page exists:

- **Goal 1 (foundation, no UI):** `core/timex` + `Config.AppTimezone`; `transactions.TransactionsInRange`
  + `EarliestTransactionDate`; `accounts` `CreatedAt` + `EarliestConnectedAt`; the pure
  `budget.ComputeResidual`/`BalanceCheck`, `tracker.BuildTracker`, `reporting.BuildWrap` (with the M1
  calendar-date bucketing, M2 signed cents, M4 leaf purity baked in); isolation-test additions. All
  exhaustively unit-tested. Publishes the contracts the consumers build on.
- **Goal 2 (Builds on 1):** `budget` persistence (migration + sqlc + service + repo) + the `/budget`
  page + e2e.
- **Goal 3 (Builds on 1):** the `home` composer + `/` (tracker), `/wraps`, `/wraps/{ym}`; the accounts
  route move (+ the M3 e2e blast-radius rewrite) + navbar (S3); home integration tests + the three new
  e2e pairs.

(Plan owns the final decomposition; this is the recommended seam.)

## Out of scope (named, so it isn't mistaken for missing)

Budget **rollover** / envelope carry-over; **historical** per-week/day actual breakdown (v1 week/day is
forward pace only); goal-based named savings; bill/subscription detection; **multi-currency**; relocating
the accounts overview *code* into `home` (only its route moves); a precise provider history-window for
`PartialFlag` (the connect-month heuristic stands); per-account drill-down and transactions
filtering/search/pagination. Each is a later slice.
