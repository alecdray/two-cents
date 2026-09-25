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

// CurrentMonthTracker composes the current-month Tracker: it resolves the month
// window in the app timezone, reads the month's rows once (the same joined
// MonthTransactions read the wrap uses), aggregates them into the pure tracker
// input (signed net spend per Category, income and savings totals), runs
// BuildTracker, joins Category names onto the result, and carries the same rows
// as the inline Transactions list — so the figures and the list reconcile
// from one set.
func (s *Service) CurrentMonthTracker(ctx contextx.ContextX) (TrackerView, error) {
	now := s.now()
	year, month := timex.CurrentMonth(s.location, now)
	start, end := timex.MonthRange(year, month)

	rows, err := s.transactions.MonthTransactions(ctx, start, end)
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
	view := trackerView(monthSlug(year, month), out, names)
	view.Label = monthLabel(year, month)
	view.MonthList = rows
	view.Rail, err = s.monthRail(ctx, year, month)
	if err != nil {
		return TrackerView{}, err
	}
	return view, nil
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
	view.Rail, err = s.monthRail(ctx, year, month)
	if err != nil {
		return WrapView{}, err
	}
	return view, nil
}

// IsCurrentMonth reports whether the given calendar month is the current one in
// the app timezone. The wrap handler uses it to send the current month's wrap
// address back to the root Tracker — the current month's one canonical face.
func (s *Service) IsCurrentMonth(year int, month time.Month) bool {
	curYear, curMonth := timex.CurrentMonth(s.location, s.now())
	return year == curYear && month == curMonth
}

// monthRail builds the month-navigation rail for the month being viewed: one chip
// per month from the earliest transaction's month through the current month,
// chronological with the current month last, each linking to its page — the
// current month to the root Tracker, earlier months to their wrap. The active
// chip is the viewed month; with no transactions the rail is just the current
// month. Transaction-date months are read from the stored calendar date directly
// (UTC), never re-zoned, so a 1st-of-month row is not mis-bucketed (audit M1).
func (s *Service) monthRail(ctx contextx.ContextX, activeYear int, activeMonth time.Month) ([]MonthChip, error) {
	curYear, curMonth := timex.CurrentMonth(s.location, s.now())

	earliestYear, earliestMonth := curYear, curMonth
	earliest, ok, err := s.transactions.EarliestTransactionDate(ctx)
	if err != nil {
		return nil, err
	}
	if ok {
		earliestYear, earliestMonth = earliest.Year(), earliest.Month()
	}

	var chips []MonthChip
	for y, m := earliestYear, earliestMonth; y < curYear || (y == curYear && m <= curMonth); y, m = nextMonth(y, m) {
		href := "/wraps/" + monthSlug(y, m)
		if y == curYear && m == curMonth {
			href = "/"
		}
		chips = append(chips, MonthChip{
			YM:     monthSlug(y, m),
			Label:  monthLabel(y, m),
			Href:   href,
			Active: y == activeYear && m == activeMonth,
		})
	}
	return chips, nil
}

// partialMonth reports whether a month sits at or before the backfill edge — the
// earliest transaction we hold. A month is partial when there are transactions
// and it is at or before that earliest transaction's month; with no transactions
// nothing is partial (audit S2). The edge is the earliest *transaction*, not the
// connection's created_at: the provider backfills history from before the connect
// date, so anchoring to created_at would flag every backfilled month. The month
// is read from the stored calendar date directly (UTC), never re-zoned, matching
// the month rail's span computation (audit M1).
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
