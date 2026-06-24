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
	"errors"
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

	Categories                []CategoryRow
	TotalRemaining            float64
	TotalDailyPace            float64
	TotalWeeklyPace           float64
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
	SavingsContributed float64
	Categories         []WrapCategoryRow
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
	return trackerView(monthSlug(year, month), out, names), nil
}

// SpendDrill composes the spend drill-down for one bucket of a month: it reads
// the month's Spending rows (the same source set the wrap aggregates), keeps the
// rows the bucket selects, and sums their signed net into the reconciling total.
// The bucket is a Category id, the uncategorized bucket, or the budget residual
// ("everything else") — the residual is current-month only (it needs the Budget
// config) and returns ErrResidualBucketUnavailable otherwise.
func (s *Service) SpendDrill(ctx contextx.ContextX, year int, month time.Month, bucket string) (DrillView, error) {
	start, end := timex.MonthRange(year, month)

	// The income and savings figures are their own exact row sets (no bucket
	// predicate), read no budget, and apply to any month — composed directly.
	switch bucket {
	case bucketIncome:
		rows, err := s.transactions.IncomeTransactionsInRange(ctx, start, end)
		if err != nil {
			return DrillView{}, err
		}
		return s.figureDrill(year, month, bucket, "Income", rows, true), nil
	case bucketSavings:
		rows, err := s.transactions.SavingsContributionsInRange(ctx, start, end)
		if err != nil {
			return DrillView{}, err
		}
		return s.figureDrill(year, month, bucket, "Savings contributed", rows, false), nil
	}

	// Spending buckets: a Category id, uncategorized, or the budget residual.
	rows, err := s.transactions.SpendingTransactionsInRange(ctx, start, end)
	if err != nil {
		return DrillView{}, err
	}
	cats, err := s.categorization.ListCategories(ctx, false)
	if err != nil {
		return DrillView{}, err
	}

	inBucket, label, err := s.bucketSelector(ctx, year, month, bucket, cats)
	if err != nil {
		return DrillView{}, err
	}

	var net int64
	kept := make([]transactions.RecentTransaction, 0, len(rows))
	for _, r := range rows {
		if !inBucket(r) {
			continue
		}
		kept = append(kept, r)
		net += cents(r.Amount.Amount)
	}

	return DrillView{
		YM:         monthSlug(year, month),
		Bucket:     bucket,
		Label:      label,
		MonthLabel: monthLabel(year, month),
		NetTotal:   dollars(net),
		Rows:       kept,
	}, nil
}

// figureDrill composes the drill for a wrap figure whose rows are already the exact
// set behind it (the Income legs, or the savings-contribution source legs) — there
// is no bucket predicate. The reconciling total sums the rows in the figure's
// positive orientation: when negate is set (income), the legs are inflows stored
// negative, so each row's display amount is negated to a positive number and summed
// into the positive gross-income total; savings source legs are positive outflows
// summed as-is. Both read no budget and apply to any month.
func (s *Service) figureDrill(year int, month time.Month, bucket, label string, rows []transactions.RecentTransaction, negate bool) DrillView {
	var total int64
	kept := make([]transactions.RecentTransaction, 0, len(rows))
	for _, r := range rows {
		if negate {
			r.Amount.Amount = -r.Amount.Amount
		}
		total += cents(r.Amount.Amount)
		kept = append(kept, r)
	}
	return DrillView{
		YM:         monthSlug(year, month),
		Bucket:     bucket,
		Label:      label,
		MonthLabel: monthLabel(year, month),
		NetTotal:   dollars(total),
		Rows:       kept,
	}
}

// bucketSelector returns the membership predicate and display label for a drill
// bucket. A Category id matches rows assigned that Category; the uncategorized
// bucket matches Spending with no Category; the residual matches Spending no
// active Budget limit covers (uncategorized or a category without a limit) — and
// is rejected for any month but the current one.
func (s *Service) bucketSelector(ctx contextx.ContextX, year int, month time.Month, bucket string, cats []categorization.Category) (func(transactions.RecentTransaction) bool, string, error) {
	switch bucket {
	case bucketUncategorized:
		return func(r transactions.RecentTransaction) bool { return r.CategoryID == nil }, "Uncategorized", nil
	case bucketEverythingElse:
		curYear, curMonth := timex.CurrentMonth(s.location, s.now())
		if year != curYear || month != curMonth {
			return nil, "", ErrResidualBucketUnavailable
		}
		budgeted, err := s.budgetedCategoryIDs(ctx)
		if err != nil {
			return nil, "", err
		}
		return func(r transactions.RecentTransaction) bool {
			if r.CategoryID == nil {
				return true
			}
			_, covered := budgeted[*r.CategoryID]
			return !covered
		}, "Everything else", nil
	default:
		id := bucket
		return func(r transactions.RecentTransaction) bool {
			return r.CategoryID != nil && *r.CategoryID == id
		}, displayName(nameMap(cats), id), nil
	}
}

// budgetedCategoryIDs is the set of Category ids the current Budget covers with
// an active limit. GetBudget already drops archived-Category limits, so this
// matches the Tracker's budgeted set exactly — the residual bucket is its
// complement (uncategorized + unbudgeted), reconciling to EverythingElseSpent.
func (s *Service) budgetedCategoryIDs(ctx contextx.ContextX) (map[string]struct{}, error) {
	_, limits, err := s.budget.GetBudget(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(limits))
	for _, l := range limits {
		set[l.CategoryID] = struct{}{}
	}
	return set, nil
}

// MonthWrap composes the wrap for a single month: it reads the month's rows and
// the partial-edge signal, maps the rows into the pure reporting input, runs
// BuildWrap, and joins Category names onto the spend breakdown.
func (s *Service) MonthWrap(ctx contextx.ContextX, year int, month time.Month) (WrapView, error) {
	start, end := timex.MonthRange(year, month)
	// Read the month's full rows once: they back both the reporting figures and the
	// inline full-month list. Every transaction in the range is included regardless
	// of its account's hidden/closed state.
	rows, err := s.transactions.MonthTransactions(ctx, start, end)
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
	view := wrapView(monthLabel(year, month), monthSlug(year, month), out, names)
	view.MonthList = rows
	return view, nil
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

// partialMonth reports whether a month sits at or before the backfill edge — the
// earliest transaction we hold. A month is partial when there are transactions
// and it is at or before that earliest transaction's month; with no transactions
// nothing is partial (audit S2). The edge is the earliest *transaction*, not the
// connection's created_at: the provider backfills history from before the connect
// date, so anchoring to created_at would flag every backfilled month. The month
// is read from the stored calendar date directly (UTC), never re-zoned, matching
// WrapList's span computation (audit M1).
func (s *Service) partialMonth(ctx contextx.ContextX, year int, month time.Month) (bool, error) {
	earliest, ok, err := s.transactions.EarliestTransactionDate(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	edgeYear, edgeMonth := earliest.Year(), earliest.Month()
	if year < edgeYear {
		return true, nil
	}
	if year == edgeYear && month <= edgeMonth {
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
	return nameMap(cats), nil
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
func trackerView(ym string, in tracker.TrackerView, names map[string]string) TrackerView {
	out := TrackerView{
		YM:                        ym,
		NeedsBudget:               in.NeedsBudget,
		TotalRemaining:            dollars(in.TotalRemainingCents),
		TotalDailyPace:            dollars(in.TotalPace.DailyCents),
		TotalWeeklyPace:           dollars(in.TotalPace.WeeklyCents),
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
		SavingsContributed: dollars(in.SavingsContributedCents),
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
