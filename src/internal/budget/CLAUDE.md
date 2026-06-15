# budget — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Owns the single rolling Budget config — a monthly income target, a savings target, and optional per-Category spending limits — plus the pure plan arithmetic the read-side tracker consumes. Domain authority: [`docs/domain/README.md`](../../../docs/domain/README.md) §Budget; design: [`docs/superpowers/specs/2026-06-15-budget-tracker-wrap-design.md`](../../../docs/superpowers/specs/2026-06-15-budget-tracker-wrap-design.md).

Module-specific notes:
- **Single rolling config, applied to the current month, carrying forward, no rollover.** One `budget` row (fixed id `'default'`) holds the income/savings targets; the `budget_category_limits` table holds the per-Category caps. The whole plan is **optional** — see the no-budget predicate below.
- **Pure arithmetic (`budget.go`):** `ComputeResidual(income, savings, activeLimits)` returns `(residual, totalSpendingBudget)` = `(income − Σlimits − savings, income − savings)`; `BalanceCheck(income, savings, limits)` returns `Balanced` / `OverAllocated` (over when `Σlimits + savings > income`). Both are pure — no storage. `ComputeResidual` takes **only active** limits; the caller (or `GetBudget`) excludes archived ones first.
- **No-budget predicate:** `IsNoBudget(b, limits)` = `income==0 && savings==0 && len(limits)==0`. Because `SetBudget` always upserts the single `'default'` row, an all-zero config is indistinguishable from an absent one, so this exact predicate is the live "no budget set" test the tracker uses. Pin it so composer + e2e agree.
- **Inert-while-archived limits.** A limit on an archived Category is **dropped from `GetBudget`** (consulting `categorization.ListCategories(active)`) but **kept in storage** — it revives when the Category is un-archived. The row is never deleted on archive.
- **`SetBudget` is a single transaction + non-blocking verdict.** It validates every limit's Category id exists (via `categorization.ListCategories(includeArchived=true)`; an unknown target is a `ValidationError`, nothing saved), then upserts the config row and **replaces** the entire limit set (delete-all + insert) in one `db.WithTx`. It returns the `BalanceCheck` verdict — **surfaced, not enforced**: an over-allocated plan still saves.
- **Reads categorization, writes only its own tables.** Imports `core/*` and `categorization` only — never `transactions`, never `accounts`, never a provider client.
- `repo.go` is the only file that touches `core/db/sqlc`; its methods take/return this package's domain types (`Budget`, `CategoryLimit`), never `sqlc.*`. `ReplaceCategoryLimits` runs delete-all + insert on its bound (tx) handle so the Service can compose it with the upsert atomically.
- Validation failures surface as `ValidationError` (a limit targeting a nonexistent Category); adapters render the message inline. Other errors are 500s.

> The `/budget` page (`adapters/`) and the composition-root wiring land in a later slice; this module is persistence + pure logic only so far.
