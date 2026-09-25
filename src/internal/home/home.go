package home

// This file holds the composed dashboard's value types and the pure functions
// over them. The Service methods that fetch and compose live in service.go.

import (
	"errors"
	"fmt"
	"time"

	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/reporting"
	"github.com/alecdray/two-cents/src/internal/tracker"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// Drill bucket selectors that are not a Category id: the uncategorized-Spending
// bucket, the budget residual ("everything else"), and the wrap's income / savings
// figures. Any other bucket value is a Category id.
const (
	bucketUncategorized  = "uncategorized"
	bucketEverythingElse = "everything-else"
	bucketIncome         = "income"
	bucketSavings        = "savings"
)

// ErrResidualBucketUnavailable is returned when the everything-else (budget
// residual) drill is requested for a month other than the current one. The
// residual is defined against the Budget config, which applies only to the
// current month, so there is no residual to drill for a past month; the handler
// maps this to a 404.
var ErrResidualBucketUnavailable = errors.New("everything-else drill is only available for the current month")

// CategoryRow is one budgeted Category's standing for the month, in dollars,
// with its display name joined from the taxonomy.
type CategoryRow struct {
	Name        string
	Bucket      string
	Limit       float64
	NetSpend    float64
	Remaining   float64
	DailyPace   float64
	WeeklyPace  float64
	OverBudget  bool
	UsedPercent int
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

	// YM is the current month's YYYY-MM slug, the base for the per-row drill links.
	YM string
	// Label is the current month's display label (e.g. "July 2026"), shown as the
	// page header — matching the month header a past-month wrap carries.
	Label string

	Categories                []CategoryRow
	TotalRemaining            float64
	TotalBudget               float64
	TotalDailyPace            float64
	TotalUsedPercent          int
	TotalOverBudget           bool
	EverythingElseBudget      float64
	EverythingElseSpent       float64
	EverythingElseRemaining   float64
	EverythingElseDailyPace   float64
	EverythingElseWeeklyPace  float64
	EverythingElseOverBudget  bool
	EverythingElseUsedPercent int
	IncomeProgress            ProgressBar
	SavingsProgress           ProgressBar

	TotalSpend float64
	Income     float64
	Savings    float64

	// MonthList is the current month's whole transaction set (every classification),
	// newest-first — the inline editable Transactions list. Like the wrap's, it is
	// not a reconciling figure; it spans the rows behind all of them.
	MonthList []transactions.RecentTransaction

	// Rail is the month-navigation rail for this page, active on the current month.
	Rail []MonthChip
}

// MonthChip is one month in the navigation rail: its YYYY-MM slug, display label,
// the page it links to (the root Tracker for the current month, the month's wrap
// otherwise), and whether it is the month currently being viewed.
type MonthChip struct {
	YM     string
	Label  string
	Href   string
	Active bool
}

// WrapCategoryRow is one Category's net spend in a wrapped month, in dollars,
// with its display name joined from the taxonomy and its drill bucket selector.
type WrapCategoryRow struct {
	Name     string
	Bucket   string
	NetSpend float64
}

// WrapView is the rendered month-wrap dashboard — actuals only, never compared
// against a budget.
type WrapView struct {
	Label              string
	YM                 string
	NetIncome          float64
	GrossIncome        float64
	TotalSpending      float64
	SavingsContributed float64
	// Surplus is net income − savings contributed for the month; may be negative.
	Surplus float64
	// Rail is the month-navigation rail for this page, active on this month.
	Rail       []MonthChip
	Categories []WrapCategoryRow
	// MonthList is the month's whole transaction set (every classification),
	// newest-first — the inline editable list under spend-by-Category. It is not a
	// reconciling figure; it spans the rows behind all of them.
	MonthList []transactions.RecentTransaction
	Settling  bool
	Partial   bool
}

// DrillView is the rendered spend drill-down: the Spending transactions making
// up one bucket's net figure for a month, newest-first, with the net total that
// the list reconciles to and the labels the page renders. Rows are edited through
// the shared modal, which reads its own taxonomy, so the view carries none.
type DrillView struct {
	YM         string
	Bucket     string
	Label      string
	MonthLabel string
	NetTotal   float64
	Rows       []transactions.RecentTransaction
}

// nameMap indexes a taxonomy by id → display name.
func nameMap(cats []categorization.Category) map[string]string {
	names := make(map[string]string, len(cats))
	for _, c := range cats {
		names[c.ID] = c.Name
	}
	return names
}

// --- pure mapping helpers (no I/O) ---

// monthSpend turns the month's rows into the tracker's per-row signed net spend:
// one MonthSpend per Spending row carrying its (possibly nil) Category id and
// signed cents (BuildTracker sums rows sharing a Category). Refund inflows are
// negative and so reduce a Category's net spend.
func monthSpend(rows []transactions.RecentTransaction) []tracker.MonthSpend {
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
func incomeCents(rows []transactions.RecentTransaction) int64 {
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
func savingsCents(rows []transactions.RecentTransaction) int64 {
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
func trackerView(ym string, in tracker.TrackerView, names map[string]string) TrackerView {
	out := TrackerView{
		YM:                        ym,
		NeedsBudget:               in.NeedsBudget,
		TotalRemaining:            dollars(in.TotalRemainingCents),
		TotalBudget:               dollars(in.TotalBudgetCents),
		TotalDailyPace:            dollars(in.TotalPace.DailyCents),
		EverythingElseBudget:      dollars(in.EverythingElseBudgetCents),
		EverythingElseSpent:       dollars(in.EverythingElseSpentCents),
		EverythingElseRemaining:   dollars(in.EverythingElseRemainingCents),
		EverythingElseDailyPace:   dollars(in.EverythingElsePace.DailyCents),
		EverythingElseWeeklyPace:  dollars(in.EverythingElsePace.WeeklyCents),
		EverythingElseOverBudget:  in.EverythingElseOverBudget,
		EverythingElseUsedPercent: percentOf(in.EverythingElseUsedRatio),
		IncomeProgress:            progressBar(in.IncomeProgress),
		SavingsProgress:           progressBar(in.SavingsProgress),
		TotalUsedPercent:          percentOf(in.TotalUsedRatio),
		TotalOverBudget:           in.TotalRemainingCents < 0,
		TotalSpend:                dollars(in.TotalSpendCents),
		Income:                    dollars(in.IncomeCents),
		Savings:                   dollars(in.SavingsCents),
	}
	for _, c := range in.Categories {
		out.Categories = append(out.Categories, CategoryRow{
			Name:        displayName(names, c.CategoryID),
			Bucket:      c.CategoryID,
			Limit:       dollars(c.LimitCents),
			NetSpend:    dollars(c.NetSpendCents),
			Remaining:   dollars(c.RemainingCents),
			DailyPace:   dollars(c.Pace.DailyCents),
			WeeklyPace:  dollars(c.Pace.WeeklyCents),
			OverBudget:  c.OverBudget,
			UsedPercent: percentOf(c.UsedRatio),
		})
	}
	return out
}

// wrapView maps the pure WrapView (cents, raw ids) onto the rendered view
// (dollars, Category names).
func wrapView(label, ym string, in reporting.WrapView, names map[string]string) WrapView {
	out := WrapView{
		Label:              label,
		YM:                 ym,
		NetIncome:          dollars(in.NetIncomeCents),
		GrossIncome:        dollars(in.GrossIncomeCents),
		TotalSpending:      dollars(in.TotalSpendingCents),
		SavingsContributed: dollars(in.SavingsContributedCents),
		Surplus:            dollars(in.SurplusCents),
		Settling:           in.State == reporting.WrapSettling,
		Partial:            in.Partial,
	}
	for _, c := range in.SpendByCategory {
		name := "Uncategorized"
		bucket := bucketUncategorized
		if c.CategoryID != nil {
			name = displayName(names, *c.CategoryID)
			bucket = *c.CategoryID
		}
		out.Categories = append(out.Categories, WrapCategoryRow{Name: name, Bucket: bucket, NetSpend: dollars(c.NetCents)})
	}
	return out
}

// progressBar turns a pure Progress into a rendered bar with dollars and an
// integer percent clamped to 0..100.
func progressBar(p tracker.Progress) ProgressBar {
	return ProgressBar{SoFar: dollars(p.SoFarCents), Target: dollars(p.TargetCents), Percent: percentOf(p.Ratio)}
}

// percentOf turns a raw used/progress ratio into an integer bar width clamped to
// 0..100: a net refund (negative ratio) reads empty and over budget (ratio > 1)
// reads full. The over-budget colour is carried separately by the OverBudget flag.
func percentOf(ratio float64) int {
	percent := int(ratio * 100)
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
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

// nextMonth steps one calendar month forward, rolling December to the next January.
func nextMonth(year int, month time.Month) (int, time.Month) {
	if month == time.December {
		return year + 1, time.January
	}
	return year, month + 1
}

// monthSlug formats a month as the YYYY-MM URL slug used in the wrap routes.
func monthSlug(year int, month time.Month) string {
	return fmt.Sprintf("%04d-%02d", year, int(month))
}

// monthLabel formats a month for display, e.g. "June 2026".
func monthLabel(year int, month time.Month) string {
	return fmt.Sprintf("%s %d", month.String(), year)
}
