# budget

Owns the single rolling **Budget** config — a monthly **income target**, a
**savings target**, and optional per-Category **spending limits** — applied to
the current month, carrying forward with no rollover. The plan is optional: an
all-zero config with no limits reads as "no budget set". The module also exposes
the pure plan arithmetic the read-side tracker consumes.

See the domain model: [`docs/domain/README.md`](../../../docs/domain/README.md)
(§Budget).

## Entities

- **Budget** — `IncomeTarget`, `SavingsTarget` (dollars). The single rolling
  config. The per-Category limits are carried alongside as a separate set, not
  embedded.
- **CategoryLimit** — `CategoryID` (a categorization Category id), `Limit` (the
  monthly spending cap, dollars).
- **BalanceStatus** — `Balanced` | `OverAllocated`, the `BalanceCheck` verdict.

## Boundaries

Imports `core/*` and `categorization` only — never the `transactions` or
`accounts` modules and never a provider client. It reads categorization (the
Category list) to validate limit targets on save and to drop archived targets
from reads; it writes only its own `budget` and `budget_category_limits` tables.

## Pure arithmetic (`budget.go`)

- `ComputeResidual(income, savings, activeLimits) → (residual,
  totalSpendingBudget)` where `residual = income − Σlimits − savings` (the
  "everything else" left for unbudgeted spending) and `totalSpendingBudget =
  income − savings`. The caller passes only **active** limits; archived ones are
  excluded first.
- `BalanceCheck(income, savings, limits) → Balanced | OverAllocated` —
  over-allocated when `Σlimits + savings > income`. Surfaced, never enforced.
- `IsNoBudget(budget, limits) → bool` — the live "no budget set" predicate,
  `income==0 && savings==0 && len(limits)==0`.

## Service

- `NewService(d *db.DB, categorization *categorization.Service) *Service`.
- `GetBudget(ctx) (Budget, []CategoryLimit, error)` — the stored config and its
  **active**-Category limits. An unset config reads as a zero Budget with no
  limits. A limit on an **archived** Category is dropped from the result but kept
  in storage, so it reappears on un-archive.
- `SetBudget(ctx, income, savings float64, limits []CategoryLimit)
  (BalanceStatus, error)` — validates each limit's Category exists (else a
  `ValidationError`, nothing saved), then upserts the config row and **replaces**
  the entire limit set in one transaction, and returns the `BalanceCheck`
  verdict. The verdict is **non-blocking** — an over-allocated plan is still
  saved.

## Glossary

- **Residual / "everything else"** — defined in the [domain glossary](../../../docs/domain/README.md); computed here by `ComputeResidual` from income, active limits, and savings.
- **Total spending budget** — income minus savings: everything available to
  spend, budgeted or not.
- **Over-allocated** — the limits plus the savings target exceed income. A
  warning, not a block.
- **Inert-while-archived** — a limit on an archived Category is skipped in reads
  but retained in storage, reviving when the Category is un-archived.
- **No budget set** — an all-zero config with no limits; the tracker falls back
  to actuals-only.

## Persistence

- `budget` — single config row, `id` fixed `'default'`, `income_target`,
  `savings_target` (REAL, default 0), timestamps.
- `budget_category_limits` — `category_id` (PK → `categories.id`),
  `limit_amount` (REAL). Replaced wholesale on each save (delete-all + insert).
