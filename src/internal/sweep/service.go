package sweep

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/core/timex"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// Service computes the cash-sweep Recommendation from live account balances,
// budget targets, and month-to-date checking activity, and appends each run to an
// immutable snapshot history other surfaces read without re-computing.
type Service struct {
	accounts     *accounts.Service
	transactions *transactions.Service
	budget       *budget.Service
	db           *db.DB
	location     *time.Location
	margin       float64
	now          func() time.Time
}

// NewService builds a sweep Service. margin is the fixed safety margin in
// dollars (typically loaded from cfg.FixedSafetyMargin); location is the app
// timezone used to bound the current-month window.
func NewService(
	accountsSvc *accounts.Service,
	transactionsSvc *transactions.Service,
	budgetSvc *budget.Service,
	database *db.DB,
	location *time.Location,
	margin float64,
) *Service {
	return &Service{
		accounts:     accountsSvc,
		transactions: transactionsSvc,
		budget:       budgetSvc,
		db:           database,
		location:     location,
		margin:       margin,
		now:          time.Now,
	}
}

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}

// Save appends rec to the snapshot history. Never replaces a previous snapshot,
// so calling it repeatedly accumulates rather than overwrites.
func (s *Service) Save(ctx contextx.ContextX, rec Recommendation) error {
	if err := s.repo().Save(ctx, rec); err != nil {
		return fmt.Errorf("sweep: save: %w", err)
	}
	return nil
}

// Run computes a recommendation and appends it as a snapshot, returning what was
// stored. It is the one path that produces a snapshot: the monthly job and the
// user's on-demand action both call it, and neither can tell the other apart.
func (s *Service) Run(ctx contextx.ContextX) (Recommendation, error) {
	rec, err := s.Compute(ctx)
	if err != nil {
		return Recommendation{}, err
	}
	rec.ID = uuid.NewString()
	if err := s.Save(ctx, rec); err != nil {
		return Recommendation{}, err
	}
	return rec, nil
}

// Snapshot returns one stored snapshot positioned in the history, so a caller
// can render it and offer the steps away from it. An empty id selects the newest
// snapshot, which is what a plain page load wants; found is false for an empty
// history and for an id that is not in it.
//
// Instants come back in the configured app timezone ([ADR-0004]): a snapshot's
// label is read by the one person the app belongs to, in the zone the rest of
// the app reckons months and schedules in.
func (s *Service) Snapshot(ctx contextx.ContextX, id string) (Snapshot, bool, error) {
	all, err := s.List(ctx)
	if err != nil {
		return Snapshot{}, false, err
	}
	snap, found := neighbors(all, id)
	if !found {
		return Snapshot{}, false, nil
	}
	snap.Recommendation.ComputedAt = snap.Recommendation.ComputedAt.In(s.location)
	return snap, true, nil
}

// List returns every stored snapshot, newest first.
func (s *Service) List(ctx contextx.ContextX) ([]Recommendation, error) {
	recs, err := s.repo().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("sweep: list: %w", err)
	}
	return recs, nil
}

// Compute reads live budget, account balances, and month-to-date checking
// activity, then produces a Recommendation. The result is numeric when both
// checking and savings accounts can be uniquely derived; otherwise it is a
// needs-attention result naming the reason(s).
func (s *Service) Compute(ctx contextx.ContextX) (Recommendation, error) {
	// The clock is read exactly once per run and threaded through everything
	// below. A snapshot's stamped instant, the month-to-date window it measured,
	// and the balance-freshness check must all be the same moment, or the
	// snapshot describes a state of the world that never existed.
	now := s.now()

	// Derive checking and savings accounts from the active cash list.
	cashAccounts, err := s.accounts.ActiveCashAccounts(ctx)
	if err != nil {
		return Recommendation{}, fmt.Errorf("sweep: failed to list active cash accounts: %w", err)
	}
	creditAccounts, err := s.accounts.ActiveCreditAccounts(ctx)
	if err != nil {
		return Recommendation{}, fmt.Errorf("sweep: failed to list active credit accounts: %w", err)
	}

	in, checkingID, err := s.buildInput(ctx, cashAccounts, creditAccounts, now)
	if err != nil {
		return Recommendation{}, err
	}
	_ = checkingID // used only when fetching MTD data; already embedded in in
	return compute(in, now), nil
}

// accountDerivation holds the result of resolving active cash accounts into
// the checking and savings roles. It is a value type so it can be built and
// inspected directly by tests without any I/O.
type accountDerivation struct {
	// checking is the live balance in dollars. Nil when zero or more-than-one
	// active cash checking account is found, or the single candidate's balance
	// is unknown — any of these prevents a numeric result.
	checking *float64
	// checkingID is the internal account ID of the derived checking account.
	// Empty when checking is nil.
	checkingID string
	// checkingStale is true when the checking account is uniquely identified but
	// its balance is too old to advise on.
	checkingStale bool
	// savingsUndetermined is true when zero or more-than-one active cash
	// savings account is found. A single savings account whose balance is
	// unknown is still determined — computation proceeds, but SavingsUnknown
	// will be true in the result.
	savingsUndetermined bool
	// savingsBalance is the savings balance when savings is determined and its
	// balance is known. Nil when savings is determined but balance unknown.
	savingsBalance *float64
	// cardBalance is the summed balance of the active credit accounts.
	cardBalance float64
	// cardBalanceUnknown / cardBalanceStale mark a card total we cannot stand
	// behind: at least one card reported no balance, or one too old to trust.
	cardBalanceUnknown bool
	cardBalanceStale   bool
}

// deriveAccounts splits the active cash account list into checking and savings
// roles, applying the determination rules. It is a pure function; all I/O is
// resolved by the caller before calling it.
//
// Checking rule: exactly one active cash account with CountsAsSavings=false,
// whose balance is known. Zero candidates, two-or-more, or an unknown balance
// each leave checking nil.
//
// Savings rule: exactly one active cash account with CountsAsSavings=true.
// Zero or two-or-more mark it undetermined. A single savings account with an
// unknown balance is determined — its balance simply does not enter the
// arithmetic (it is non-load-bearing, unlike checking).
func deriveAccounts(cashAccounts, creditAccounts []accounts.Account, now time.Time) accountDerivation {
	var checkingAccounts, savingsAccounts []accounts.Account
	for _, a := range cashAccounts {
		if a.CountsAsSavings {
			savingsAccounts = append(savingsAccounts, a)
		} else {
			checkingAccounts = append(checkingAccounts, a)
		}
	}

	var d accountDerivation

	// Checking: exactly one required, its balance must be known, and that balance
	// must be fresh enough to advise on. A stale balance is reported as its own
	// reason rather than folded into "undetermined": the account is perfectly well
	// identified, and getting the sync working is not the same fix as designating
	// an account. An unknown balance stays undetermined — there is no figure to be
	// stale about.
	if len(checkingAccounts) == 1 && checkingAccounts[0].Balance.Known {
		if checkingAccounts[0].BalanceStale(now) {
			d.checkingStale = true
		} else {
			bal := checkingAccounts[0].Balance.Money.Amount
			d.checking = &bal
			d.checkingID = checkingAccounts[0].ID
		}
	}

	// Savings: exactly one required for a determined result. Zero or many →
	// undetermined. A single savings account with an unknown balance is still
	// determined; savingsBalance stays nil and SavingsUnknown will be set in
	// the final Recommendation.
	if len(savingsAccounts) != 1 {
		d.savingsUndetermined = true
	} else {
		sav := savingsAccounts[0]
		if sav.Balance.Known {
			bal := sav.Balance.Money.Amount
			d.savingsBalance = &bal
		}
	}

	// Cards: every active one counts and they sum — debt is additive, so unlike
	// checking and savings there is no single-account requirement and no count of
	// cards is ambiguous. The total is load-bearing, though, so a card we cannot
	// stand behind (no balance reported, or one too old to trust) blocks rather
	// than being silently treated as nothing owed ([ADR-0023]).
	for _, c := range creditAccounts {
		switch {
		case !c.Balance.Known:
			d.cardBalanceUnknown = true
		case c.BalanceStale(now):
			d.cardBalanceStale = true
		default:
			d.cardBalance += c.Balance.Money.Amount
		}
	}

	return d
}

// buildInput resolves the live data into a computeInput. When accounts cannot be
// uniquely derived, it short-circuits with a partially-filled input so compute
// can produce the needs-attention result without further I/O.
func (s *Service) buildInput(ctx contextx.ContextX, cashAccounts, creditAccounts []accounts.Account, now time.Time) (computeInput, string, error) {
	d := deriveAccounts(cashAccounts, creditAccounts, now)

	in := computeInput{
		fixedSafetyMargin:   s.margin,
		checking:            d.checking,
		checkingStale:       d.checkingStale,
		cardBalance:         d.cardBalance,
		cardBalanceUnknown:  d.cardBalanceUnknown,
		cardBalanceStale:    d.cardBalanceStale,
		savingsUndetermined: d.savingsUndetermined,
		savingsBalance:      d.savingsBalance,
	}

	// If checking is undetermined we cannot fetch MTD data; return early so
	// compute produces the needs-attention result.
	if in.checking == nil {
		return in, "", nil
	}

	if err := s.fillMTD(ctx, &in, d.checkingID, now); err != nil {
		return computeInput{}, "", err
	}
	return in, d.checkingID, nil
}

// fillMTD populates the budget and MTD activity fields of in from live data.
func (s *Service) fillMTD(ctx contextx.ContextX, in *computeInput, checkingID string, now time.Time) error {
	b, limits, err := s.budget.GetBudget(ctx)
	if err != nil {
		return fmt.Errorf("sweep: failed to load budget: %w", err)
	}
	if !budget.IsNoBudget(b, limits) {
		_, totalSpending := budget.ComputeResidual(b.IncomeTarget, b.SavingsTarget, limits)
		in.totalSpendingBudget = totalSpending
		in.savingsTarget = b.SavingsTarget
	}
	// When no budget: totalSpendingBudget = 0, savingsTarget = 0 (zero value).

	year, month := timex.CurrentMonth(s.location, now)
	start, end := timex.MonthRange(year, month)
	// end is the open upper bound: "through the run instant" is satisfied by the
	// half-open [start, end) range where end is the 1st of next month at midnight.
	// That works because no Transaction is ever future-dated — a transaction date
	// is an authorization or posting date the bank has already reported, never a
	// scheduled one — so nothing between now and month-end can match. Do NOT
	// "tighten" this bound to now: it changes no result, and the month boundary is
	// what keeps this window identical to the budget's month bucketing. The sweep
	// can be run at any instant, so this holds mid-month as much as on the 7th.

	spendRows, err := s.transactions.SpendingByAccountInRange(ctx, checkingID, start, end)
	if err != nil {
		return fmt.Errorf("sweep: failed to load MTD spending: %w", err)
	}
	var mtdSpending float64
	for _, r := range spendRows {
		mtdSpending += r.Amount.Amount
	}
	in.mtdSpending = mtdSpending

	savingsRows, err := s.transactions.SavingsContributionsByAccountInRange(ctx, checkingID, start, end)
	if err != nil {
		return fmt.Errorf("sweep: failed to load MTD savings contributions: %w", err)
	}
	var mtdSavings float64
	for _, r := range savingsRows {
		mtdSavings += r.Amount.Amount
	}
	in.mtdSavingsContributed = mtdSavings

	return nil
}

