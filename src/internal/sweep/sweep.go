// Package sweep computes the monthly cash-sweep recommendation: the suggested
// dollar amount to move between the user's checking and savings accounts to keep
// checking adequately funded while maximising savings contributions. It reads
// existing budget, account, and transaction data through the domain services and
// produces a Recommendation carrying every component figure — or a
// needs-attention result listing the reasons a numeric result cannot be produced.
//
// The module owns no tables and no repo. It is a read-side composer, importing
// budget, accounts, and transactions services exactly like the home module does.
// It must never import a bank provider or write any row.
package sweep

// RecommendationKind classifies the sweep output: numeric when the computation
// can be completed, needs-attention when a required input is unavailable.
type RecommendationKind string

const (
	// KindNumeric is a fully computed result with every component figure.
	KindNumeric RecommendationKind = "numeric"
	// KindNeedsAttention means the computation cannot proceed — the Reasons field
	// names what is missing.
	KindNeedsAttention RecommendationKind = "needs_attention"
)

// NeedsAttentionReason identifies one cause that prevents a numeric result.
type NeedsAttentionReason string

const (
	// ReasonCheckingUndetermined is returned when zero or more than one active
	// cash account with CountsAsSavings=false is found; the checking account
	// cannot be uniquely derived.
	ReasonCheckingUndetermined NeedsAttentionReason = "checking_undetermined"
	// ReasonSavingsUndetermined is returned when zero or more than one active
	// cash account with CountsAsSavings=true is found; the savings account
	// cannot be uniquely derived. An identifiable savings account with an unknown
	// balance is NOT this reason — the computation still proceeds.
	ReasonSavingsUndetermined NeedsAttentionReason = "savings_undetermined"
)

// SweepDirection is the direction of the suggested transfer.
type SweepDirection string

const (
	// DirectionCheckingToSavings means SuggestedSweep is positive: move money
	// from checking to savings.
	DirectionCheckingToSavings SweepDirection = "checking->savings"
	// DirectionSavingsToChecking means SuggestedSweep is negative: pull money
	// back from savings to checking.
	DirectionSavingsToChecking SweepDirection = "savings->checking"
	// DirectionNone means SuggestedSweep is exactly zero: no transfer needed.
	DirectionNone SweepDirection = "none"
)

// Recommendation is the output of the sweep computation. It is either a numeric
// result carrying every component figure (Kind == KindNumeric) or a
// needs-attention result listing the reasons a number cannot be produced (Kind
// == KindNeedsAttention).
//
// Numeric field semantics (sign convention: outflow positive, inflow negative):
//
//   - CurrentChecking: the checking account's live balance in dollars.
//   - CurrentSavings: the savings account's live balance in dollars.
//     Zero when SavingsUnknown=true; never used in the sweep arithmetic.
//   - SavingsUnknown: true when the savings account was identified but its
//     balance is not reported by the provider. The result is still numeric;
//     the savings balance simply does not enter the arithmetic.
//   - TotalSpendingBudget: income − savings_target from the active budget.
//     Zero when no budget is set.
//   - MtdSpending: the signed net of this month's Spending transactions on the
//     checking account (outflows positive, refund inflows negative).
//   - SavingsTarget: the monthly savings target from the budget. Zero when no
//     budget is set.
//   - MtdSavingsContributed: the sum of savings-contribution transfer outflows
//     from checking so far this month.
//   - Reserve: the dollars that must stay in checking to cover the remaining
//     budget and savings target.
//     Reserve = max(0, TotalSpendingBudget − MtdSpending)
//            + max(0, SavingsTarget − MtdSavingsContributed)
//     Both terms are floored at 0 independently: spending past budget never
//     reduces the savings reserve, and saving past target never inflates the
//     spending reserve.
//   - FixedSafetyMargin: the minimum dollar cushion kept in checking regardless
//     of the budget (default $500, configurable).
//   - SuggestedSweep: the net dollars to move. Positive → move from checking
//     to savings; negative → pull from savings; zero → no action.
//     SuggestedSweep = CurrentChecking − Reserve − FixedSafetyMargin
//     (not floored — a negative value is a meaningful pull-back signal).
//   - Direction: the transfer direction encoded from the sign of SuggestedSweep.
type Recommendation struct {
	Kind RecommendationKind

	// Numeric fields — meaningful when Kind == KindNumeric.
	CurrentChecking       float64
	CurrentSavings        float64
	SavingsUnknown        bool
	TotalSpendingBudget   float64
	MtdSpending           float64
	SavingsTarget         float64
	MtdSavingsContributed float64
	Reserve               float64
	FixedSafetyMargin     float64
	SuggestedSweep        float64
	Direction             SweepDirection

	// Needs-attention field — populated when Kind == KindNeedsAttention.
	Reasons []NeedsAttentionReason
}

// computeInput carries the pre-fetched figures the sweep arithmetic operates on.
// The Service.Compute method resolves live data and fills this struct; tests
// build it directly to stay off the database.
type computeInput struct {
	// checking is nil when no single active cash account with CountsAsSavings=false
	// can be derived (zero or two-or-more found).
	checking *float64

	// savingsUndetermined is true when no single active cash account with
	// CountsAsSavings=true can be derived. When false, savingsBalance holds the
	// balance (nil if the provider has not reported one).
	savingsUndetermined bool
	savingsBalance      *float64

	totalSpendingBudget   float64
	savingsTarget         float64
	mtdSpending           float64
	mtdSavingsContributed float64
	fixedSafetyMargin     float64
}

// compute derives the Recommendation from the pre-fetched inputs. It is a pure
// function; all I/O is resolved by the caller before this is invoked.
func compute(in computeInput) Recommendation {
	var reasons []NeedsAttentionReason
	if in.checking == nil {
		reasons = append(reasons, ReasonCheckingUndetermined)
	}
	if in.savingsUndetermined {
		reasons = append(reasons, ReasonSavingsUndetermined)
	}
	if len(reasons) > 0 {
		return Recommendation{Kind: KindNeedsAttention, Reasons: reasons}
	}

	// Reserve: each term floored at 0 independently so spending past budget
	// never cancels the savings reserve, and saving past target never swells the
	// spending reserve.
	spendingReserve := in.totalSpendingBudget - in.mtdSpending
	if spendingReserve < 0 {
		spendingReserve = 0
	}
	savingsReserve := in.savingsTarget - in.mtdSavingsContributed
	if savingsReserve < 0 {
		savingsReserve = 0
	}
	reserve := spendingReserve + savingsReserve

	// Suggested sweep: positive means sweep checking → savings;
	// negative means pull savings → checking. Not floored.
	suggestedSweep := *in.checking - reserve - in.fixedSafetyMargin

	direction := DirectionNone
	if suggestedSweep > 0 {
		direction = DirectionCheckingToSavings
	} else if suggestedSweep < 0 {
		direction = DirectionSavingsToChecking
	}

	rec := Recommendation{
		Kind:                  KindNumeric,
		CurrentChecking:       *in.checking,
		TotalSpendingBudget:   in.totalSpendingBudget,
		MtdSpending:           in.mtdSpending,
		SavingsTarget:         in.savingsTarget,
		MtdSavingsContributed: in.mtdSavingsContributed,
		Reserve:               reserve,
		FixedSafetyMargin:     in.fixedSafetyMargin,
		SuggestedSweep:        suggestedSweep,
		Direction:             direction,
	}
	if in.savingsBalance == nil {
		rec.SavingsUnknown = true
	} else {
		rec.CurrentSavings = *in.savingsBalance
	}
	return rec
}
