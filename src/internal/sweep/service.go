package sweep

import (
	"fmt"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/timex"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// Service computes the cash-sweep Recommendation from live account balances,
// budget targets, and month-to-date checking activity. It owns no tables.
type Service struct {
	accounts     *accounts.Service
	transactions *transactions.Service
	budget       *budget.Service
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
	location *time.Location,
	margin float64,
) *Service {
	return &Service{
		accounts:     accountsSvc,
		transactions: transactionsSvc,
		budget:       budgetSvc,
		location:     location,
		margin:       margin,
		now:          time.Now,
	}
}

// Compute reads live budget, account balances, and month-to-date checking
// activity, then produces a Recommendation. The result is numeric when both
// checking and savings accounts can be uniquely derived; otherwise it is a
// needs-attention result naming the reason(s).
func (s *Service) Compute(ctx contextx.ContextX) (Recommendation, error) {
	// Derive checking and savings accounts from the active cash list.
	cashAccounts, err := s.accounts.ActiveCashAccounts(ctx)
	if err != nil {
		return Recommendation{}, fmt.Errorf("sweep: failed to list active cash accounts: %w", err)
	}
	in, checkingID, err := s.buildInput(ctx, cashAccounts)
	if err != nil {
		return Recommendation{}, err
	}
	_ = checkingID // used only when fetching MTD data; already embedded in in
	return compute(in), nil
}

// buildInput resolves the live data into a computeInput. When accounts cannot be
// uniquely derived, it short-circuits with a partially-filled input so compute
// can produce the needs-attention result without further I/O.
func (s *Service) buildInput(ctx contextx.ContextX, cashAccounts []accounts.Account) (computeInput, string, error) {
	var checkingAccounts, savingsAccounts []accounts.Account
	for _, a := range cashAccounts {
		if a.CountsAsSavings {
			savingsAccounts = append(savingsAccounts, a)
		} else {
			checkingAccounts = append(checkingAccounts, a)
		}
	}

	in := computeInput{fixedSafetyMargin: s.margin}

	// Checking: exactly one required.
	if len(checkingAccounts) == 1 {
		bal := checkingAccounts[0].Balance.Money.Amount
		in.checking = &bal
	}
	// Savings: exactly one required for numeric result. Zero or many → undetermined.
	if len(savingsAccounts) != 1 {
		in.savingsUndetermined = true
	} else {
		sav := savingsAccounts[0]
		if sav.Balance.Known {
			bal := sav.Balance.Money.Amount
			in.savingsBalance = &bal
		}
		// else: savings found but balance unknown — savingsBalance stays nil,
		// SavingsUnknown will be true in the result; computation proceeds.
	}

	// If checking is undetermined we cannot fetch MTD data; return early so
	// compute produces the needs-attention result.
	if in.checking == nil {
		return in, "", nil
	}

	checkingID := checkingAccounts[0].ID
	if err := s.fillMTD(ctx, &in, checkingID); err != nil {
		return computeInput{}, "", err
	}
	return in, checkingID, nil
}

// fillMTD populates the budget and MTD activity fields of in from live data.
func (s *Service) fillMTD(ctx contextx.ContextX, in *computeInput, checkingID string) error {
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

	now := s.now()
	year, month := timex.CurrentMonth(s.location, now)
	start, end := timex.MonthRange(year, month)
	// end is the open upper bound: "through the run instant" is satisfied by the
	// half-open [start, end) range where end is the 1st of next month at midnight.

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

