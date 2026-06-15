// Package budget owns the single rolling Budget config — a monthly income
// target, a savings target, and optional per-Category spending limits — applied
// to the current month, carrying forward with no rollover. It is optional: an
// all-zero config with no limits reads as "no budget set".
//
// The module persists two concepts (the single config row and its Category
// limits) and exposes the pure plan arithmetic the read-side tracker consumes:
// ComputeResidual (the "everything else" left over) and BalanceCheck (whether
// the plan over-allocates income). A per-Category limit on an archived Category
// is inert — dropped from reads — but stays in storage so it revives when the
// Category is un-archived. The module reads categorization (the Category list)
// to validate limit targets and to skip archived ones at read time; it never
// imports transactions or accounts.
package budget

// Budget is the single rolling plan config: the monthly income and savings
// targets. The per-Category spending limits are carried alongside as a separate
// CategoryLimit set rather than embedded here.
type Budget struct {
	IncomeTarget  float64
	SavingsTarget float64
}

// CategoryLimit is a monthly spending cap on one budgeted Category. CategoryID
// references a categorization Category id; Limit is the cap in dollars.
type CategoryLimit struct {
	CategoryID string
	Limit      float64
}

// BalanceStatus is BalanceCheck's verdict on whether a plan fits its income.
type BalanceStatus string

const (
	// Balanced means the limits plus the savings target fit within income.
	Balanced BalanceStatus = "balanced"
	// OverAllocated means the limits plus the savings target exceed income —
	// surfaced to the user, never enforced; an over-allocated plan still saves.
	OverAllocated BalanceStatus = "over_allocated"
)

// ComputeResidual derives the leftover "everything else" budget and the total
// spending budget from a plan. residual is income minus every active-Category
// limit minus the savings target (the money left for unbudgeted spending);
// totalSpendingBudget is income minus savings (everything available to spend,
// budgeted or not). The caller passes only the active-Category limits — an
// archived limit is inert and must be excluded before calling.
func ComputeResidual(income, savings float64, activeLimits []CategoryLimit) (residual, totalSpendingBudget float64) {
	var totalLimits float64
	for _, l := range activeLimits {
		totalLimits += l.Limit
	}
	residual = income - totalLimits - savings
	totalSpendingBudget = income - savings
	return residual, totalSpendingBudget
}

// BalanceCheck reports whether a plan over-allocates income: the per-Category
// limits plus the savings target exceeding income is OverAllocated, otherwise
// Balanced. It is a surfaced signal, not a constraint — an over-allocated plan
// is still valid and still saved.
func BalanceCheck(income, savings float64, limits []CategoryLimit) BalanceStatus {
	var totalLimits float64
	for _, l := range limits {
		totalLimits += l.Limit
	}
	if totalLimits+savings > income {
		return OverAllocated
	}
	return Balanced
}

// IsNoBudget reports whether a config reads as "no budget set": all-zero targets
// and no limits. Because SetBudget always upserts the single config row, an
// all-zero config is indistinguishable from an absent one, so this is the live
// predicate the read-side tracker uses to fall back to actuals-only.
func IsNoBudget(b Budget, limits []CategoryLimit) bool {
	return b.IncomeTarget == 0 && b.SavingsTarget == 0 && len(limits) == 0
}
