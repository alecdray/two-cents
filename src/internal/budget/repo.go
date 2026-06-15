package budget

import (
	"context"

	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"
)

// Repo is the budget module's data access layer. It is the only file in package
// budget that imports core/db/sqlc; its methods take and return this package's
// domain types (Budget, CategoryLimit) — never sqlc.* shapes.
type Repo struct {
	q *sqlc.Queries
}

// NewRepo binds a Repo to the given Queries. Callers bind to db.Queries() for
// the global handle or to tx.Queries() inside a db.WithTx callback for
// transactional work.
func NewRepo(q *sqlc.Queries) *Repo {
	return &Repo{q: q}
}

// --- conversion helpers (private — only repo.go touches sqlc types) ---

func budgetFromModel(m sqlc.Budget) Budget {
	return Budget{
		IncomeTarget:  m.IncomeTarget,
		SavingsTarget: m.SavingsTarget,
	}
}

func categoryLimitFromModel(m sqlc.BudgetCategoryLimit) CategoryLimit {
	return CategoryLimit{
		CategoryID: m.CategoryID,
		Limit:      m.LimitAmount,
	}
}

// budgetID is the fixed primary key of the single rolling config row.
const budgetID = "default"

// GetBudget returns the stored config, or (zero, sql.ErrNoRows) when the single
// row has never been written. The Service maps the missing-row case to a zero
// Budget.
func (r *Repo) GetBudget(ctx context.Context) (Budget, error) {
	model, err := r.q.GetBudget(ctx, budgetID)
	if err != nil {
		return Budget{}, err
	}
	return budgetFromModel(model), nil
}

// UpsertBudget writes the single config row's targets, inserting it on first
// write and updating it thereafter.
func (r *Repo) UpsertBudget(ctx context.Context, b Budget) (Budget, error) {
	model, err := r.q.UpsertBudget(ctx, sqlc.UpsertBudgetParams{
		ID:            budgetID,
		IncomeTarget:  b.IncomeTarget,
		SavingsTarget: b.SavingsTarget,
	})
	if err != nil {
		return Budget{}, err
	}
	return budgetFromModel(model), nil
}

// ListCategoryLimits returns every stored per-Category limit, archived targets
// included — the Service drops archived ones at read time.
func (r *Repo) ListCategoryLimits(ctx context.Context) ([]CategoryLimit, error) {
	models, err := r.q.ListCategoryLimits(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryLimit, len(models))
	for i, m := range models {
		out[i] = categoryLimitFromModel(m)
	}
	return out, nil
}

// ReplaceCategoryLimits replaces the entire limit set with the given limits: it
// deletes every existing limit and inserts the submitted set, so a removed limit
// disappears and the stored set always matches the last save. The Service calls
// it through a tx-bound Repo (inside db.WithTx) so the delete and inserts — and
// the accompanying budget upsert — commit atomically.
func (r *Repo) ReplaceCategoryLimits(ctx context.Context, limits []CategoryLimit) error {
	if err := r.q.DeleteAllCategoryLimits(ctx); err != nil {
		return err
	}
	for _, l := range limits {
		if err := r.q.CreateCategoryLimit(ctx, sqlc.CreateCategoryLimitParams{
			CategoryID:  l.CategoryID,
			LimitAmount: l.Limit,
		}); err != nil {
			return err
		}
	}
	return nil
}
