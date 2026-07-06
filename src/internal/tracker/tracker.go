// Package tracker holds the current-month, forward-looking read-model: given a
// month's actuals and (optionally) the rolling Budget config, it derives what is
// remaining per budgeted Category and for "Everything else", the daily/weekly
// pace needed to stay within plan, income and savings progress toward target,
// the over-budget flags, and the fraction of each budget used (per Category,
// Everything else, and the whole plan) for the per-row spend bars.
//
// It is a pure utility leaf: it persists nothing and imports no domain package.
// Go imports are package-level — there is no way to import "the Budget type but
// not the Budget service" — so every input is a local struct defined here, with
// raw Category ids (*string; nil = uncategorized) and money as signed integer
// cents. The composing layer fetches the data, fills these structs, and joins
// Category names back afterward; this module does only the arithmetic.
//
// Money sign convention (inherited from banking): an outflow is positive and an
// inflow negative. Net spend is therefore a SIGNED sum — a refund is a negative
// inflow that reduces spend, so a Category whose refunds exceed its purchases has
// negative net spend and thus more remaining. Income and savings totals are
// passed in as positive magnitudes (the composer negates the inflow legs).
package tracker

// MonthSpend is the net spend on one Category for the month, in signed cents
// (outflow positive, refund inflow negative). A nil CategoryID is uncategorized
// spend. The composer pre-aggregates the month's rows into these; multiple
// entries for the same Category are summed.
type MonthSpend struct {
	CategoryID *string
	NetCents   int64
}

// CategoryLimitView is one active per-Category spending cap, in cents. Only
// active limits are passed (the composer drops archived ones), so every limit
// here is counted toward the residual and rendered as a budgeted row.
type CategoryLimitView struct {
	CategoryID string
	LimitCents int64
}

// BudgetView is the rolling Budget config as the tracker needs it: the monthly
// income and savings targets in cents plus the active per-Category limits. A nil
// *BudgetView (or an all-zero one) means no budget is set — the tracker then
// reports actuals only.
type BudgetView struct {
	IncomeTargetCents  int64
	SavingsTargetCents int64
	Limits             []CategoryLimitView
}

// TrackerInput is everything BuildTracker needs: the optional budget, the
// month's net spend per Category, income and savings so far (positive cents),
// and days left in the month inclusive of today (clamped >= 1 by the composer).
type TrackerInput struct {
	Budget            *BudgetView
	Spend             []MonthSpend
	IncomeCents       int64
	SavingsCents      int64
	DaysLeftInclusive int
}

// Pace is the spend-down rate that keeps a remaining figure on plan: daily is
// max(0, remaining) / days-left-inclusive and weekly is daily * 7. Pace is a
// spending concept only — income and savings are shown as progress, never pace.
type Pace struct {
	DailyCents  int64
	WeeklyCents int64
}

// CategoryRemaining is one budgeted Category's standing for the month: its limit,
// its signed net spend, what is left, the pace to hold the line, whether it is
// already over budget (net spend exceeds the limit), and the fraction of the
// limit spent (net spend ÷ limit; can be negative on a net refund or exceed 1
// when over budget — the composer clamps it for the bar width).
type CategoryRemaining struct {
	CategoryID     string
	LimitCents     int64
	NetSpendCents  int64
	RemainingCents int64
	Pace           Pace
	OverBudget     bool
	UsedRatio      float64
}

// Progress is movement toward a target: the amount so far, the target, and their
// ratio (so-far / target; 0 when the target is 0). It is never a pace.
type Progress struct {
	SoFarCents  int64
	TargetCents int64
	Ratio       float64
}

// TrackerView is the rendered current-month read-model. When NeedsBudget is true
// the budget-relative fields (Categories, totals, everything-else, progress) are
// omitted and only the actuals (TotalSpendCents, IncomeCents, SavingsCents) are
// meaningful.
type TrackerView struct {
	// NeedsBudget is true when no budget is set (input nil or all-zero); the view
	// then carries actuals only and the UI prompts the user to create a budget.
	NeedsBudget bool

	// Budget-relative cards — populated only when a budget exists. TotalRemaining
	// is every row's remaining summed — each budgeted Category plus the
	// everything-else residual below — i.e. income − savings − total net spend.
	// TotalPace is the spend-down pace that holds that whole total. TotalUsedRatio
	// is total net spend ÷ the whole spendable plan (income − savings).
	Categories          []CategoryRemaining
	TotalRemainingCents int64
	TotalBudgetCents    int64
	TotalPace           Pace
	TotalUsedRatio      float64

	// Everything else — the unallocated residual, treated like a Category: its
	// "limit" is the residual (income − Σ limits − savings), its "net spend" is
	// the spend no active limit covers (unbudgeted + uncategorized), and it can
	// be over budget like any Category.
	EverythingElseBudgetCents    int64
	EverythingElseSpentCents     int64
	EverythingElseRemainingCents int64
	EverythingElsePace           Pace
	EverythingElseOverBudget     bool
	EverythingElseUsedRatio      float64

	IncomeProgress  Progress
	SavingsProgress Progress

	// Actuals — always populated, in both modes.
	TotalSpendCents int64
	IncomeCents     int64
	SavingsCents    int64
}

// BuildTracker derives the current-month view from the input. With no budget it
// returns actuals plus the NeedsBudget flag; with a budget it computes per-
// Category and everything-else remaining, the pace targets, and income/savings
// progress.
func BuildTracker(in TrackerInput) TrackerView {
	catNet := make(map[string]int64)
	var uncategorizedNet, totalSpend int64
	for _, s := range in.Spend {
		totalSpend += s.NetCents
		if s.CategoryID == nil {
			uncategorizedNet += s.NetCents
			continue
		}
		catNet[*s.CategoryID] += s.NetCents
	}

	view := TrackerView{
		TotalSpendCents: totalSpend,
		IncomeCents:     in.IncomeCents,
		SavingsCents:    in.SavingsCents,
	}

	if budgetIsEmpty(in.Budget) {
		view.NeedsBudget = true
		return view
	}

	b := in.Budget
	budgeted := make(map[string]struct{}, len(b.Limits))
	var totalLimits, totalRemaining int64
	for _, l := range b.Limits {
		budgeted[l.CategoryID] = struct{}{}
		totalLimits += l.LimitCents

		netSpend := catNet[l.CategoryID]
		remaining := l.LimitCents - netSpend
		over := netSpend > l.LimitCents
		totalRemaining += remaining

		view.Categories = append(view.Categories, CategoryRemaining{
			CategoryID:     l.CategoryID,
			LimitCents:     l.LimitCents,
			NetSpendCents:  netSpend,
			RemainingCents: remaining,
			Pace:           paceFor(remaining, in.DaysLeftInclusive),
			OverBudget:     over,
			UsedRatio:      ratioOf(netSpend, l.LimitCents),
		})
	}

	// Everything else: the residual left after limits and savings, drawn down by
	// spend that no active limit covers (categorized-but-unbudgeted + uncategorized).
	var unbudgetedNet int64
	for id, net := range catNet {
		if _, ok := budgeted[id]; !ok {
			unbudgetedNet += net
		}
	}
	residual := in.Budget.IncomeTargetCents - totalLimits - in.Budget.SavingsTargetCents
	residualSpend := unbudgetedNet + uncategorizedNet
	everythingElse := residual - residualSpend
	view.EverythingElseBudgetCents = residual
	view.EverythingElseSpentCents = residualSpend
	view.EverythingElseRemainingCents = everythingElse
	view.EverythingElsePace = paceFor(everythingElse, in.DaysLeftInclusive)
	view.EverythingElseOverBudget = residualSpend > residual
	view.EverythingElseUsedRatio = ratioOf(residualSpend, residual)

	// The total folds in everything-else, so it is the sum of every row shown —
	// each budgeted Category plus the everything-else residual — which collapses
	// to income − savings − total net spend: the whole month's spendable plan.
	totalRemaining += everythingElse
	totalBudget := in.Budget.IncomeTargetCents - in.Budget.SavingsTargetCents
	view.TotalRemainingCents = totalRemaining
	view.TotalBudgetCents = totalBudget
	view.TotalPace = paceFor(totalRemaining, in.DaysLeftInclusive)
	view.TotalUsedRatio = ratioOf(totalSpend, totalBudget)

	view.IncomeProgress = progressFor(in.IncomeCents, b.IncomeTargetCents)
	view.SavingsProgress = progressFor(in.SavingsCents, b.SavingsTargetCents)

	return view
}

// budgetIsEmpty reports whether the budget input reads as "no budget set": nil,
// or all-zero targets with no limits. This mirrors the budget module's no-budget
// predicate so the tracker falls back to actuals on either a nil pointer or an
// untouched config.
func budgetIsEmpty(b *BudgetView) bool {
	return b == nil || (b.IncomeTargetCents == 0 && b.SavingsTargetCents == 0 && len(b.Limits) == 0)
}

// paceFor turns a remaining figure into a daily/weekly spend-down rate. Negative
// remaining (over budget) clamps to a zero pace, and a non-positive days-left
// guards against division by zero (the composer clamps days-left >= 1).
func paceFor(remainingCents int64, daysLeftInclusive int) Pace {
	if remainingCents <= 0 || daysLeftInclusive < 1 {
		return Pace{}
	}
	daily := remainingCents / int64(daysLeftInclusive)
	return Pace{DailyCents: daily, WeeklyCents: daily * 7}
}

// ratioOf is the fraction of a budget that spend consumes (spent ÷ budget),
// guarding a non-positive budget (ratio 0). The raw fraction is returned
// unclamped — it is negative on a net refund and exceeds 1 when over budget; the
// composer clamps it to a 0..100 bar width.
func ratioOf(spentCents, budgetCents int64) float64 {
	if budgetCents <= 0 {
		return 0
	}
	return float64(spentCents) / float64(budgetCents)
}

// progressFor builds a Progress toward a target, guarding a zero target (ratio 0).
func progressFor(soFarCents, targetCents int64) Progress {
	p := Progress{SoFarCents: soFarCents, TargetCents: targetCents}
	if targetCents > 0 {
		p.Ratio = float64(soFarCents) / float64(targetCents)
	}
	return p
}
