# Two Cents — PRD

Status: **draft** · Source: synthesized from scope.md + planning conversation (2026-06-13)

> Companion to [scope.md](./scope.md). Scope holds the high-level direction and the bank-provider decision (we chose Teller, then switched to Plaid — see [ADR-0002](./adr/0002-bankprovider-abstraction.md)); this PRD details the v1 feature set, modules, and testing plan.

## Problem Statement

I have money spread across multiple accounts (checking, savings, credit cards) and no single place that tells me, plainly, where my money goes and whether I'm on track. Bank apps show raw, cryptic transactions per-account; they don't aggregate spending by category, don't let me set a budget, and don't tell me mid-month whether I'm overspending. I want to make sensible decisions about my money — my two cents on my own spending — without manually wrangling exports every week.

## Solution

A personal finance app that automatically pulls my transactions and account balances from my banks (via Plaid), categorizes spending, and gives me:

- a single **overview** of total cash, credit-card debt, and net cash;
- **spending aggregated by category**;
- a **budget** I define (income, savings, per-category spending);
- a **current-month tracker** showing how much of each budget is left — total, and as a forward pace target by week and by day;
- an **end-of-month wrap** summarizing net income, savings, and spending by category;
- **categorization rules** so a given merchant always lands in the right category, plus **custom categories** of my own.

## User Stories

### Connecting & syncing

1. As the user, I want to connect a bank account via Plaid Link, so that the app can read my accounts and transactions.
2. As the user, I want to connect multiple accounts (checking, savings, credit cards), so that I see my whole financial picture.
3. As the user, I want transactions to be pulled automatically on a regular interval, so that my data stays current without me doing anything.
4. As the user, I want to trigger a sync on demand, so that I can see the latest activity immediately when I want it.
5. As the user, I want pending transactions to be shown and then reconciled when they post, so that I'm not double-counting or surprised.
6. As the user, I want syncs to dedupe against what I already have, so that re-pulling never creates duplicate transactions.
7. As the user, I want to see when each account was last synced, so that I know how fresh the data is.
8. As the user, I want to be told when a bank connection needs re-authentication, so that I can fix a broken connection before it silently goes stale.

### Accounts overview

9. As the user, I want to see my total cash across all cash accounts, so that I know how much I actually have.
10. As the user, I want to see my total credit-card debt, so that I know what I owe.
11. As the user, I want to see my net cash (total cash − credit-card debt), so that I know my real position at a glance.
12. As the user, I want to see each account's individual balance, so that I can drill into any one account.
13. As the user, I want the overview to update after every sync, so that it always reflects current balances.

### Transactions & categorization

14. As the user, I want each transaction to have a category, so that I can understand my spending.
15. As the user, I want a cleaned-up merchant name on each transaction, so that I'm not reading cryptic bank strings.
16. As the user, I want transactions to be auto-categorized using the bank-provided category as a starting point, so that most are right without effort.
17. As the user, I want to manually re-categorize a transaction, so that I can fix a wrong category.
18. As the user, I want my manual category override to stick, so that a future sync never reverts my correction.
19. As the user, I want to create a rule "transactions from this merchant should be category X going forward", so that I categorize a merchant once and never again.
20. As the user, I want rules to apply to future transactions automatically, so that recurring merchants are handled hands-off.
21. As the user, I want to define custom categories beyond the defaults, so that the categories match how I actually think about my money.
22. As the user, I want to edit or delete custom categories and rules, so that I can keep my setup tidy as my needs change.
23. As the user, I want a predictable precedence when categorizing — manual override beats merchant rule beats bank category beats uncategorized — so that the result is never ambiguous.
24. As the user, I want uncategorized transactions surfaced, so that I can clean them up.

### Spending aggregation

25. As the user, I want to see total spending grouped by category for a period, so that I can see where my money goes.
26. As the user, I want to drill from a category total into the transactions that make it up, so that I can verify and investigate.

### Budget creator

27. As the user, I want to set a monthly income target, so that the budget has a top line.
28. As the user, I want to set a monthly savings target, so that I plan to save a specific amount.
29. As the user, I want to set a spending limit per category, so that I can cap discretionary spending.
30. As the user, I want budgets to apply per calendar month and reset each month, so that the model is simple and predictable.
31. As the user, I want to edit my budget, so that I can adjust as my situation changes.
32. As the user, I want the sum of category limits + savings target to be shown against income, so that I can see whether my plan balances.

### Current-month tracker

33. As the user, I want to see how much of each spending category's budget is left this month, so that I know what's safe to spend.
34. As the user, I want to see total budget remaining across all categories, so that I have a single "how am I doing" number.
35. As the user, I want to see income received so far this month vs. my income target, so that I know if I'm on track to earn what I expected.
36. As the user, I want to see savings contributed so far this month vs. my savings target, so that I know if I'm saving as planned.
37. As the user, I want a per-week pace target — remaining budget spread across the weeks left in the month — so that I know my sustainable weekly spend.
38. As the user, I want a per-day pace target — remaining budget spread across the days left — so that I know my sustainable daily spend.
39. As the user, I want categories that are over budget clearly flagged, so that I can course-correct.

### End-of-month wrap

40. As the user, I want an end-of-month summary of net income (income − spending), so that I know if I came out ahead.
41. As the user, I want the wrap to show total savings contributed that month, so that I can confirm I hit my savings goal.
42. As the user, I want the wrap to show spending by category for the month, so that I can review where it all went.
43. As the user, I want budget-vs-actual comparison while the month is live (in the current-month tracker); historical wraps show actuals only, since budgets apply to the current month and not retroactively.
44. As the user, I want to review past months' wraps, so that I can see trends over time.

## Implementation Decisions

> Domain terms below are defined in the [domain model](./domain/README.md). Architectural decisions are recorded in [docs/adr/](./adr).

### Architecture

**Stack and architecture mirror the sibling project [`wax`](../../wax)** ([ADR-0001](./adr/0001-self-hosted-single-user-service.md)) — we adopt its conventions wholesale rather than reinvent them:

- **Self-hosted single-user service** — one Go binary, server-rendered **templ** + **htmx** (mobile-first; Tailwind v4 + DaisyUI theme + Bootstrap Icons; tokens-not-raw-colors), custom `httpx.Mux` (no web framework), packaged as a Docker container on infra the user controls. Plaid secrets (app credentials + per-Item `access_token`s) + DB on a mounted volume.
- **Data layer:** SQLite via **mattn/go-sqlite3** (cgo, matching wax) + **goose** migrations (`db/migrations`) + **sqlc** type-safe queries (`db/queries`, no sqlx); text UUID ids.
- **Scheduling:** **robfig/cron/v3** behind a `core/task.Task`, registered in the `server/` composition root — runs the 6h sync.
- **Build/dev:** **Task** (`taskfile.yml`). **Auth:** single local login (JWT) — no third-party OAuth (wax's one deviation).
- **Codebase follows wax's archetypes:** `core/` (shared infra: db, httpx, task, templates, config) and `server/` (composition root) singletons; **domain modules** (`service.go` + `repo.go` — only repo touches sqlc — + `adapters/` with `http.go`/`routes.go`/`views/*.templ`); **external-client** modules (Plaid: `client.go`/`entities.go`/`service.go`, no persistence); **utility** modules (pure, no persistence). Per-module `README.md` + `CLAUDE.md` declaring archetype.
- **Bank access behind a `BankProvider` interface** returning our own `Account`/`Transaction` types; the Plaid external-client satisfies it, so the provider is an adapter swap, not a rewrite ([ADR-0002](./adr/0002-bankprovider-abstraction.md)).
- Business logic lives in **deep modules** testable without network or DB (fake `BankProvider`, in-memory or tx-scoped repo).

### Modules

Mapped to wax's archetypes. Each domain module owns its own tables via its `repo.go` — there is **no central "Store"** (wax convention); cross-module reads go through the owning module's `*Service`.

**External-client** (`client.go` + `entities.go` + `service.go`; no persistence):
- **`plaid`** — satisfies the `BankProvider` interface: `listAccounts()`, `getBalances()`, `syncTransactions(cursor)`. App-credential auth (`client_id` + `secret`), exchanging the Plaid Link `public_token` for a per-Item `access_token`; cursor-based `/transactions/sync`; translates Plaid wire shapes → our domain types. The only code that talks to the bank network.

**Domain modules** (`service.go` + `repo.go` + optional `task.go` + `adapters/`):
- **`accounts`** — owns Connections + Accounts: balances, user-overridable **kind** (cash/credit/other), **counts-as-savings** flag, and per-Connection **needs-reconnect** state. Service exposes the overview inputs (total cash incl. savings, total credit debt, net cash).
- **`transactions`** — owns Transactions and their Classification/Category + manual overrides. Hosts the **sync `task.go`**: pulls via the `plaid` service on a **~6h** cron + on demand using cursor-based `/transactions/sync`; applies the `added`/`modified`/`removed` sets — dedupes/updates by transaction `id` (same id across pending→posted → update in place), and deletes rows in Plaid's `removed` set directly (no age-based reaper).
- **`categorization`** — owns the built-in + custom Category taxonomy (archive-not-delete) and Rules (substring match on cleaned merchant → full outcome, future + existing non-overridden). Resolves Classification + Category with precedence **manual override > rule > bank category (`personal_finance_category`) > uncategorized**; Transfer subtype derived by destination pairing ([ADR-0003](./adr/0003-two-layer-transfer-detection.md)).
- **`budget`** — owns the single rolling Budget config (income target, savings target, per-Category limits) applied to the **current month**; it persists and carries forward (no recreation) and unspent never rolls over. Plus the **Everything else** residual.

**Utility** (pure, no persistence — domain-shaped calculators consuming the above via services):
- **`tracker`** — current-month remaining per Category, Everything else, and total; income/savings progress; forward **pace targets** (`max(0,remaining) ÷ days-left-inclusive`, weekly = daily×7); over-budget flags.
- **`reporting`** — Month wrap (net income, savings contributed, spend-by-Category vs. budget; settling/partial states) + period spend-by-Category aggregation.

### Domain decisions (settled in design grill)

- **Transfers are first-class and excluded from spending + income.** A Transfer moves money between two of the user's own Accounts. Spending is counted only on the originating purchase, never on the transfer that later settles it (e.g. a credit-card payment is a Transfer, not spending). See [ADR-0003](./adr/0003-two-layer-transfer-detection.md).
- **Two axes per transaction: Classification (Income/Spending/Transfer) + Category** (only when Spending). Surfaced as one re-categorize picker; choosing a spending Category sets Classification = Spending; choosing Income/Transfer clears the Category.
- **Savings = transfers into a savings-flagged Account** (money actually moved in), not "income minus spending." Savings-flag is per-Account, default-on for bank-type savings, user-settable. A transfer to an *unconnected* account can't be auto-attributed to savings until the user marks it.
- **Account kind = cash | credit | other**, seeded from the provider type (depository → cash, credit → credit, everything else → other) but user-overridable; drives the overview. **Net cash = all cash balances (savings included) − all credit balances**, excluding `other`. Two Cents is a spending tool, not a net-worth tracker: loan, mortgage, and investment/brokerage accounts are tracked as `other` — stored and listed but excluded from net cash. See [ADR-0005](./adr/0005-spending-tool-three-bucket-account-kind.md).
- **Budget = income target + savings target + optional per-Category limits**, monthly, no rollover. Spending outside any limit (unbudgeted category or uncategorized) draws from **"Everything else" = income − Σ(category limits) − savings**. Total spending budget = income − savings.
- **Income excludes Transfers; refunds/reimbursements are negative Spending** in their Category (not income), keeping spend-by-category truthful. Net income = total Income − total Spending.
- **Pace targets are forward-looking and spending-only:** `daily = max(0, remaining) ÷ days-left-inclusive` (today counted); `weekly = daily × 7`; computed per Category, for Everything else, and total; over-budget clamps to 0 and flags. Income/savings shown as progress vs. target, not as a pace.
- **A transaction belongs to a month by its transaction date** (not posted date). Pending transactions **count** in the live tracker (marked pending in UI). A **wrap is "settling"** while its month holds any pending transaction; **"partial"** when coverage is incomplete (connect month, or backfilled edge months). On first connect, **backfill max provider history**.
- **Sync:** cursor-based `/transactions/sync` (`added`/`modified`/`removed` + a per-Connection cursor); dedupe/update by transaction `id`; dropped pendings arrive in the `removed` set (no age-based reaper); ~6h auto-sync + on-demand; per-Connection needs-reconnect surfaced, never silent.
- **Currency: USD only**; **period = calendar month** throughout.

## Testing Decisions

Follows wax's testing conventions: **Go unit tests next to the code** (`*_test.go`, grouped `Test<Func>` with `t.Run` subtests named as behaviors) for logic, and **Playwright e2e** (`e2e/feat/*.feature` ↔ `e2e/spec/*.spec.ts`, BDD-style, run against the real app — no mocks) for full-stack flows. Tests are written in the same change as the feature, not a later phase.

- **Assert external behavior, not implementation** — given inputs (transactions, balances, budgets, rules), assert the computed outputs (classification + category, remaining amounts, pace targets, wrap totals). No assertions on call order or private structure.
- **The `utility` modules are the priority for unit tests** — pure, no DB, highest logic density:
  - **`tracker`** — remaining math, pace-target division (edge cases: month start, last day, zero days left, over-budget clamp), income/savings progress.
  - **`reporting`** — wrap totals, actual-vs-budget per Category, settling/partial states, period aggregation.
- **Domain-module service logic** unit-tested with a small locally-declared repo interface (wax pattern) or tx-scoped repo:
  - **`categorization`** — precedence resolution, substring rule matching, override stickiness across re-sync, archive behavior.
  - **`budget`** — validation, Everything-else residual math.
  - **`accounts`** — cash/debt/net aggregation across mixed kinds.
  - **`transactions` sync task** — tested against a **fake `BankProvider`**: dedupe, pending→posted update-in-place, `removed`-set deletion, needs-reconnect — no network.
- **`plaid` external-client** — thin translation; tested against recorded/fixture responses, not live calls.
- Prior art: none yet (greenfield) — but mirror wax's `e2e/` gate rules (feature↔spec pairing, testid selectors, no fixed-timeout waits).
- **Test-first targets:** `tracker`, `reporting`, `categorization`, `budget` first; `accounts` and the fake-provider sync tests follow as built.

## Out of Scope

- Investments / holdings detail and liabilities (loan APR, credit-card interest breakdown).
- Payments / money movement initiated from the app.
- Multi-user / managing other people's accounts.
- Mobile app (platform TBD — see open questions in scope.md).
- Non-USD / non-US banks (US-only coverage).
- **Budget rollover** (envelope carry-over) — explicitly deferred; v1 is monthly-no-rollover.
- **Historical per-week/per-day actual spend breakdown** — v1's week/day view is forward pace targets only.
- Goal-based savings (named goals like "trip fund") beyond a single monthly savings target.
- Bill/subscription detection and reminders.
- **Merging categories** — archive/rename only in v1.
- Fuzzy/pattern merchant matching in rules — v1 is substring match on the cleaned merchant name.

## Further Notes

- Re-auth is a first-class state, not an error: connections expire (password change, MFA, bank-side OAuth). The sync engine surfaces it; the UI must let the user re-link.
- Secrets (Plaid app credentials + per-Item `access_token`s) are stored by us — encrypt at rest, never commit. (From scope.md risks.)
- Categorization is never 100%; the API/rule category is always a default the user can override, and overrides persist.
- Open questions from scope.md are now **resolved**: stack = Go + templ + htmx + Tailwind/DaisyUI/Bootstrap Icons + SQLite (goose/sqlc), self-hosted single-user via Docker ([ADR-0001](./adr/0001-self-hosted-single-user-service.md)); initial history = backfill max; taxonomy = **own, mapped onto Plaid's `personal_finance_category`**.
- Domain language is the source of truth in the [domain model](./domain/README.md); architectural rationale in [docs/adr/](./adr).
