// Package home is the dashboard's composing module: it injects the domain
// services (budget, transactions, categorization, accounts) and the configured
// app timezone, fetches the month-scoped data, fills the pure tracker/reporting
// projection inputs, and joins Category names back onto the results to produce
// the rendered view models for the current-month Tracker and the month wraps.
//
// It owns no tables and no repo — it is the legitimate composition root for the
// read-side dashboard, the only module that may import multiple domain services.
// It must never import a provider client; it reaches the bank only transitively
// through the services it composes.
package home

import (
	"fmt"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/timex"
	"github.com/alecdray/two-cents/src/internal/reporting"
	"github.com/alecdray/two-cents/src/internal/tracker"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// Service composes the read-side dashboard from the domain services. The clock
// is held as a field (defaulting to time.Now) so "the current month" and
// "days left" are reckoned in the configured app timezone and remain injectable
// for tests.
type Service struct {
	budget         *budget.Service
	transactions   *transactions.Service
	categorization *categorization.Service
	accounts       *accounts.Service
	location       *time.Location
	now            func() time.Time
}

// NewService builds the composing Service over the domain services it reads and
// the configured app timezone. The timezone decides which calendar month "now"
// falls in and how many days remain in it.
func NewService(
	budgetSvc *budget.Service,
	transactionsSvc *transactions.Service,
	categorizationSvc *categorization.Service,
	accountsSvc *accounts.Service,
	location *time.Location,
) *Service {
	return &Service{
		budget:         budgetSvc,
		transactions:   transactionsSvc,
		categorization: categorizationSvc,
		accounts:       accountsSvc,
		location:       location,
		now:            time.Now,
	}
}

// CategoryRow is one budgeted Category's standing for the month, in dollars,
// with its display name joined from the taxonomy.
type CategoryRow struct {
	Name       string
	Limit      float64
	NetSpend   float64
	Remaining  float64
	DailyPace  float64
	WeeklyPace float64
	OverBudget bool
}

// ProgressBar is movement toward a target rendered for a bar: the amount so far
// and the target in dollars plus the integer percent (clamped 0..100) for the
// bar width. The raw ratio can exceed 1; Percent never does.
type ProgressBar struct {
	SoFar   float64
	Target  float64
	Percent int
}

// TrackerView is the rendered current-month dashboard. When NeedsBudget is true
// only the actuals (TotalSpend, Income, Savings) are meaningful and the page
// prompts for a budget; otherwise the budget-relative fields are populated.
type TrackerView struct {
	NeedsBudget bool

	Categories               []CategoryRow
	TotalRemaining           float64
	TotalDailyPace           float64
	TotalWeeklyPace          float64
	EverythingElseBudget     float64
	EverythingElseSpent      float64
	EverythingElseRemaining  float64
	EverythingElseDailyPace  float64
	EverythingElseWeeklyPace float64
	EverythingElseOverBudget bool
	IncomeProgress           ProgressBar
	SavingsProgress          ProgressBar

	TotalSpend float64
	Income     float64
	Savings    float64
}

// WrapCategoryRow is one Category's net spend in a wrapped month, in dollars,
// with its display name joined from the taxonomy.
type WrapCategoryRow struct {
	Name     string
	NetSpend float64
}

// WrapView is the rendered month-wrap dashboard — actuals only, never compared
// against a budget.
type WrapView struct {
	Label              string
	NetIncome          float64
	SavingsContributed float64
	Categories         []WrapCategoryRow
	Settling           bool
	Partial            bool
}

// WrapSummary is one row of the wraps list: the month, its URL slug, its label,
// and the settling/partial badges.
type WrapSummary struct {
	YM       string
	Label    string
	Settling bool
	Partial  bool
}

// CurrentMonthTracker composes the current-month Tracker: it resolves the month
// window in the app timezone, reads the month's rows, the budget, and the
// taxonomy, aggregates the rows into the pure tracker input (signed net spend per
// Category, income and savings totals), runs BuildTracker, and joins Category
// names onto the result.
func (s *Service) CurrentMonthTracker(ctx contextx.ContextX) (TrackerView, error) {
	now := s.now()
	year, month := timex.CurrentMonth(s.location, now)
	start, end := timex.MonthRange(year, month)

	rows, err := s.transactions.TransactionsInRange(ctx, start, end)
	if err != nil {
		return TrackerView{}, err
	}
	b, limits, err := s.budget.GetBudget(ctx)
	if err != nil {
		return TrackerView{}, err
	}
	names, err := s.categoryNames(ctx)
	if err != nil {
		return TrackerView{}, err
	}

	in := tracker.TrackerInput{
		Spend:             monthSpend(rows),
		IncomeCents:       incomeCents(rows),
		SavingsCents:      savingsCents(rows),
		DaysLeftInclusive: timex.DaysLeftInclusive(s.location, now),
	}
	if !budget.IsNoBudget(b, limits) {
		in.Budget = budgetView(b, limits)
	}

	out := tracker.BuildTracker(in)
	return trackerView(out, names), nil
}

// MonthWrap composes the wrap for a single month: it reads the month's rows and
// the partial-edge signal, maps the rows into the pure reporting input, runs
// BuildWrap, and joins Category names onto the spend breakdown.
func (s *Service) MonthWrap(ctx contextx.ContextX, year int, month time.Month) (WrapView, error) {
	start, end := timex.MonthRange(year, month)
	rows, err := s.transactions.TransactionsInRange(ctx, start, end)
	if err != nil {
		return WrapView{}, err
	}
	names, err := s.categoryNames(ctx)
	if err != nil {
		return WrapView{}, err
	}
	partial, err := s.partialMonth(ctx, year, month)
	if err != nil {
		return WrapView{}, err
	}

	txns := make([]reporting.WrapTxn, 0, len(rows))
	for _, r := range rows {
		txns = append(txns, reporting.WrapTxn{
			CategoryID:      r.CategoryID,
			Classification:  string(r.Classification),
			AmountCents:     cents(r.Amount.Amount),
			TransferSubtype: string(r.TransferSubtype),
			Pending:         r.Pending,
		})
	}

	out := reporting.BuildWrap(reporting.WrapInput{Txns: txns, Partial: partial})
	return wrapView(monthLabel(year, month), out, names), nil
}

// WrapList builds the month list from the earliest transaction's month through
// the current month (app timezone), most-recent first. The current month always
// appears; with no transactions the list collapses to just the current month.
// Transaction-date months are read from the stored calendar date directly (UTC),
// never re-zoned, so a 1st-of-month row is not mis-bucketed (audit M1).
func (s *Service) WrapList(ctx contextx.ContextX) ([]WrapSummary, error) {
	curYear, curMonth := timex.CurrentMonth(s.location, s.now())

	earliestYear, earliestMonth := curYear, curMonth
	earliest, ok, err := s.transactions.EarliestTransactionDate(ctx)
	if err != nil {
		return nil, err
	}
	if ok {
		earliestYear, earliestMonth = earliest.Year(), earliest.Month()
	}

	var summaries []WrapSummary
	for y, m := curYear, curMonth; y > earliestYear || (y == earliestYear && m >= earliestMonth); y, m = prevMonth(y, m) {
		wrap, err := s.MonthWrap(ctx, y, m)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, WrapSummary{
			YM:       monthSlug(y, m),
			Label:    monthLabel(y, m),
			Settling: wrap.Settling,
			Partial:  wrap.Partial,
		})
	}
	return summaries, nil
}

// partialMonth reports whether a month sits at or before the backfill edge: it
// is partial when there is at least one connection and the month is at or before
// the earliest connection's month (its created_at reckoned in the app timezone).
// With no connections nothing is partial (audit S2).
func (s *Service) partialMonth(ctx contextx.ContextX, year int, month time.Month) (bool, error) {
	connectedAt, ok, err := s.accounts.EarliestConnectedAt(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	connYear, connMonth := timex.CurrentMonth(s.location, connectedAt)
	if year < connYear {
		return true, nil
	}
	if year == connYear && month <= connMonth {
		return true, nil
	}
	return false, nil
}

// categoryNames reads the active taxonomy and returns an id→display-name map for
// joining names onto the projection results.
func (s *Service) categoryNames(ctx contextx.ContextX) (map[string]string, error) {
	cats, err := s.categorization.ListCategories(ctx, false)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(cats))
	for _, c := range cats {
		names[c.ID] = c.Name
	}
	return names, nil
}

// --- pure mapping helpers (no I/O) ---

// monthSpend turns the month's rows into the tracker's per-row signed net spend:
// one MonthSpend per Spending row carrying its (possibly nil) Category id and
// signed cents (BuildTracker sums rows sharing a Category). Refund inflows are
// negative and so reduce a Category's net spend.
func monthSpend(rows []transactions.ActivityRow) []tracker.MonthSpend {
	spend := make([]tracker.MonthSpend, 0, len(rows))
	for _, r := range rows {
		if r.Classification != categorization.Spending {
			continue
		}
		spend = append(spend, tracker.MonthSpend{CategoryID: r.CategoryID, NetCents: cents(r.Amount.Amount)})
	}
	return spend
}

// incomeCents sums the month's Income legs as a positive total (income legs are
// inflows, stored negative, so each is negated).
func incomeCents(rows []transactions.ActivityRow) int64 {
	var total int64
	for _, r := range rows {
		if r.Classification == categorization.Income {
			total += -cents(r.Amount.Amount)
		}
	}
	return total
}

// savingsCents sums the month's savings-contribution source legs (positive
// outflows); the paired mirror inflow carries a different subtype and is skipped.
func savingsCents(rows []transactions.ActivityRow) int64 {
	var total int64
	for _, r := range rows {
		if r.TransferSubtype == categorization.SubtypeSavingsContribution {
			total += cents(r.Amount.Amount)
		}
	}
	return total
}

// budgetView maps the stored budget + active limits onto the tracker's input
// view, in cents.
func budgetView(b budget.Budget, limits []budget.CategoryLimit) *tracker.BudgetView {
	out := &tracker.BudgetView{
		IncomeTargetCents:  cents(b.IncomeTarget),
		SavingsTargetCents: cents(b.SavingsTarget),
	}
	for _, l := range limits {
		out.Limits = append(out.Limits, tracker.CategoryLimitView{CategoryID: l.CategoryID, LimitCents: cents(l.Limit)})
	}
	return out
}

// trackerView maps the pure TrackerView (cents, raw ids) onto the rendered view
// (dollars, Category names).
func trackerView(in tracker.TrackerView, names map[string]string) TrackerView {
	out := TrackerView{
		NeedsBudget:              in.NeedsBudget,
		TotalRemaining:           dollars(in.TotalRemainingCents),
		TotalDailyPace:           dollars(in.TotalPace.DailyCents),
		TotalWeeklyPace:          dollars(in.TotalPace.WeeklyCents),
		EverythingElseBudget:     dollars(in.EverythingElseBudgetCents),
		EverythingElseSpent:      dollars(in.EverythingElseSpentCents),
		EverythingElseRemaining:  dollars(in.EverythingElseRemainingCents),
		EverythingElseDailyPace:  dollars(in.EverythingElsePace.DailyCents),
		EverythingElseWeeklyPace: dollars(in.EverythingElsePace.WeeklyCents),
		EverythingElseOverBudget: in.EverythingElseOverBudget,
		IncomeProgress:           progressBar(in.IncomeProgress),
		SavingsProgress:          progressBar(in.SavingsProgress),
		TotalSpend:               dollars(in.TotalSpendCents),
		Income:                   dollars(in.IncomeCents),
		Savings:                  dollars(in.SavingsCents),
	}
	for _, c := range in.Categories {
		out.Categories = append(out.Categories, CategoryRow{
			Name:       displayName(names, c.CategoryID),
			Limit:      dollars(c.LimitCents),
			NetSpend:   dollars(c.NetSpendCents),
			Remaining:  dollars(c.RemainingCents),
			DailyPace:  dollars(c.Pace.DailyCents),
			WeeklyPace: dollars(c.Pace.WeeklyCents),
			OverBudget: c.OverBudget,
		})
	}
	return out
}

// wrapView maps the pure WrapView (cents, raw ids) onto the rendered view
// (dollars, Category names).
func wrapView(label string, in reporting.WrapView, names map[string]string) WrapView {
	out := WrapView{
		Label:              label,
		NetIncome:          dollars(in.NetIncomeCents),
		SavingsContributed: dollars(in.SavingsContributedCents),
		Settling:           in.State == reporting.WrapSettling,
		Partial:            in.Partial,
	}
	for _, c := range in.SpendByCategory {
		name := "Uncategorized"
		if c.CategoryID != nil {
			name = displayName(names, *c.CategoryID)
		}
		out.Categories = append(out.Categories, WrapCategoryRow{Name: name, NetSpend: dollars(c.NetCents)})
	}
	return out
}

// progressBar turns a pure Progress into a rendered bar with dollars and an
// integer percent clamped to 0..100.
func progressBar(p tracker.Progress) ProgressBar {
	percent := int(p.Ratio * 100)
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return ProgressBar{SoFar: dollars(p.SoFarCents), Target: dollars(p.TargetCents), Percent: percent}
}

// displayName resolves a Category id to its display name, falling back to the
// raw id when the Category is missing (e.g. archived between read and render).
func displayName(names map[string]string, id string) string {
	if name, ok := names[id]; ok {
		return name
	}
	return id
}

// cents converts a signed dollar amount to signed integer cents, rounding to the
// nearest cent while preserving sign (outflow positive, refund inflow negative).
func cents(amount float64) int64 {
	if amount < 0 {
		return -int64(-amount*100 + 0.5)
	}
	return int64(amount*100 + 0.5)
}

// dollars converts signed integer cents back to a dollar amount for rendering.
func dollars(c int64) float64 {
	return float64(c) / 100
}

// prevMonth steps one calendar month back, rolling January to the prior December.
func prevMonth(year int, month time.Month) (int, time.Month) {
	if month == time.January {
		return year - 1, time.December
	}
	return year, month - 1
}

// monthSlug formats a month as the YYYY-MM URL slug used in the wrap routes.
func monthSlug(year int, month time.Month) string {
	return fmt.Sprintf("%04d-%02d", year, int(month))
}

// monthLabel formats a month for display, e.g. "June 2026".
func monthLabel(year int, month time.Month) string {
	return fmt.Sprintf("%s %d", month.String(), year)
}
