package sweep

import (
	"testing"
)

// ptr is a helper that turns a float64 literal into a pointer.
func ptr(f float64) *float64 { return &f }

// --- Criterion 1: suggested_sweep = current_checking - reserve - fixed_safety_margin ---

func TestSuggestedSweepFormula(t *testing.T) {
	// Single checking ($3000), single savings (known, $500), budget $2000 spend
	// + $200 savings, MTD spend $800, MTD savings $0, margin $500.
	// reserve = max(0, 2000-800) + max(0, 200-0) = 1200 + 200 = 1400
	// suggested_sweep = 3000 - 1400 - 500 = 1100
	in := computeInput{
		checking:              ptr(3000),
		savingsUndetermined:   false,
		savingsBalance:        ptr(500),
		totalSpendingBudget:   2000,
		savingsTarget:         200,
		mtdSpending:           800,
		mtdSavingsContributed: 0,
		fixedSafetyMargin:     500,
	}
	got := compute(in)
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric, got %s", got.Kind)
	}
	if got.SuggestedSweep != 1100 {
		t.Errorf("SuggestedSweep: want 1100, got %v", got.SuggestedSweep)
	}
	if got.Reserve != 1400 {
		t.Errorf("Reserve: want 1400, got %v", got.Reserve)
	}
	if got.CurrentChecking != 3000 {
		t.Errorf("CurrentChecking: want 3000, got %v", got.CurrentChecking)
	}
}

// --- Criterion 2: independent floor-at-0 for each reserve term ---

func TestReserveIndependentFloors(t *testing.T) {
	// Spent $2500 against a $2000 budget (over budget by $500): spending reserve
	// should be 0 (floored), not a negative that would reduce the savings term.
	// Contributed $0 toward a $200 savings target: savings reserve = $200.
	// reserve = max(0, 2000-2500) + max(0, 200-0) = 0 + 200 = 200
	// suggested_sweep = 3000 - 200 - 500 = 2300
	in := computeInput{
		checking:              ptr(3000),
		savingsBalance:        ptr(500),
		totalSpendingBudget:   2000,
		savingsTarget:         200,
		mtdSpending:           2500, // over budget
		mtdSavingsContributed: 0,
		fixedSafetyMargin:     500,
	}
	got := compute(in)
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric, got %s", got.Kind)
	}
	if got.Reserve != 200 {
		t.Errorf("Reserve: want 200 (only savings reserve), got %v", got.Reserve)
	}
	if got.SuggestedSweep != 2300 {
		t.Errorf("SuggestedSweep: want 2300, got %v", got.SuggestedSweep)
	}

	// Saved $300 against a $200 target (over target): savings reserve should be
	// 0 (floored), not a negative that would inflate the sweep.
	// Spent $500 against a $2000 budget: spending reserve = 1500.
	// reserve = max(0, 2000-500) + max(0, 200-300) = 1500 + 0 = 1500
	// suggested_sweep = 3000 - 1500 - 500 = 1000
	in2 := computeInput{
		checking:              ptr(3000),
		savingsBalance:        ptr(500),
		totalSpendingBudget:   2000,
		savingsTarget:         200,
		mtdSpending:           500,
		mtdSavingsContributed: 300, // over target
		fixedSafetyMargin:     500,
	}
	got2 := compute(in2)
	if got2.Reserve != 1500 {
		t.Errorf("Reserve: want 1500 (only spending reserve), got %v", got2.Reserve)
	}
	if got2.SuggestedSweep != 1000 {
		t.Errorf("SuggestedSweep: want 1000, got %v", got2.SuggestedSweep)
	}

	// Both over: spending over by $300, savings over by $100.
	// reserve = max(0, 2000-2300) + max(0, 200-300) = 0 + 0 = 0
	// suggested_sweep = 3000 - 0 - 500 = 2500
	in3 := computeInput{
		checking:              ptr(3000),
		savingsBalance:        ptr(500),
		totalSpendingBudget:   2000,
		savingsTarget:         200,
		mtdSpending:           2300,
		mtdSavingsContributed: 300,
		fixedSafetyMargin:     500,
	}
	got3 := compute(in3)
	if got3.Reserve != 0 {
		t.Errorf("Reserve: want 0 (both over), got %v", got3.Reserve)
	}
	if got3.SuggestedSweep != 2500 {
		t.Errorf("SuggestedSweep: want 2500, got %v", got3.SuggestedSweep)
	}
}

// --- Criterion 3: MTD spending is net Spending (refunds net down), excludes Transfers/Income ---
// This criterion is enforced by the data sourcing in service.go (SQL query
// filters classification='spending' only). The compute function receives the
// pre-aggregated net; the test verifies the arithmetic behaves correctly when
// the net is negative (more refunds than spending this month).

func TestMtdSpendingNet(t *testing.T) {
	// MTD net spending = -$50 (more refunds than outflows this month).
	// spending reserve = max(0, 2000 - (-50)) = max(0, 2050) = 2050
	// savings reserve = max(0, 200 - 0) = 200
	// reserve = 2250
	// suggested_sweep = 1500 - 2250 - 500 = -1250 (pull from savings)
	in := computeInput{
		checking:              ptr(1500),
		savingsBalance:        ptr(500),
		totalSpendingBudget:   2000,
		savingsTarget:         200,
		mtdSpending:           -50, // net negative: more refunds than spend
		mtdSavingsContributed: 0,
		fixedSafetyMargin:     500,
	}
	got := compute(in)
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric, got %s", got.Kind)
	}
	wantReserve := 2050.0 + 200
	if got.Reserve != wantReserve {
		t.Errorf("Reserve: want %v, got %v", wantReserve, got.Reserve)
	}
	wantSweep := 1500.0 - wantReserve - 500
	if got.SuggestedSweep != wantSweep {
		t.Errorf("SuggestedSweep: want %v, got %v", wantSweep, got.SuggestedSweep)
	}
}

// --- Criterion 4: Direction follows sign of suggested_sweep ---

func TestDirection(t *testing.T) {
	cases := []struct {
		name      string
		checking  float64
		reserve   float64
		margin    float64
		wantDir   SweepDirection
		wantSign  string
	}{
		{
			name:     "positive sweep -> checking to savings",
			checking: 3000, reserve: 1400, margin: 500,
			// sweep = 3000 - 1400 - 500 = 1100 > 0
			wantDir:  DirectionCheckingToSavings,
			wantSign: "positive",
		},
		{
			name:     "negative sweep -> savings to checking",
			checking: 800, reserve: 1400, margin: 500,
			// sweep = 800 - 1400 - 500 = -1100 < 0
			wantDir:  DirectionSavingsToChecking,
			wantSign: "negative",
		},
		{
			name:     "zero sweep -> none",
			checking: 1900, reserve: 1400, margin: 500,
			// sweep = 1900 - 1400 - 500 = 0
			wantDir:  DirectionNone,
			wantSign: "zero",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build input so reserve = tc.reserve with zero MTD (budget = reserve).
			in := computeInput{
				checking:            ptr(tc.checking),
				savingsBalance:      ptr(0),
				totalSpendingBudget: tc.reserve,
				fixedSafetyMargin:   tc.margin,
			}
			got := compute(in)
			if got.Kind != KindNumeric {
				t.Fatalf("expected numeric, got %s", got.Kind)
			}
			if got.Direction != tc.wantDir {
				t.Errorf("Direction: want %s, got %s", tc.wantDir, got.Direction)
			}
		})
	}
}

// --- Criterion 5: no budget -> totals are 0, numeric result still produced ---

func TestNoBudget(t *testing.T) {
	// With no budget, totalSpendingBudget and savingsTarget are both 0.
	// reserve = max(0, 0-0) + max(0, 0-0) = 0
	// suggested_sweep = 3000 - 0 - 500 = 2500
	in := computeInput{
		checking:              ptr(3000),
		savingsBalance:        ptr(1000),
		totalSpendingBudget:   0, // no budget
		savingsTarget:         0, // no budget
		mtdSpending:           0,
		mtdSavingsContributed: 0,
		fixedSafetyMargin:     500,
	}
	got := compute(in)
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric result even with no budget, got %s", got.Kind)
	}
	if got.TotalSpendingBudget != 0 {
		t.Errorf("TotalSpendingBudget: want 0, got %v", got.TotalSpendingBudget)
	}
	if got.SavingsTarget != 0 {
		t.Errorf("SavingsTarget: want 0, got %v", got.SavingsTarget)
	}
	if got.SuggestedSweep != 2500 {
		t.Errorf("SuggestedSweep: want 2500, got %v", got.SuggestedSweep)
	}
}

// --- Criterion 6: unknown savings balance -> numeric, SavingsUnknown=true, does not affect arithmetic ---

func TestUnknownSavingsBalance(t *testing.T) {
	// Savings account identified but balance not reported by provider.
	// The result must still be numeric; CurrentSavings must be 0 and SavingsUnknown=true.
	// The arithmetic is unchanged: suggested_sweep = current_checking - reserve - margin.
	// reserve = max(0, 2000-800) + max(0, 200-0) = 1200 + 200 = 1400
	// suggested_sweep = 3000 - 1400 - 500 = 1100
	in := computeInput{
		checking:              ptr(3000),
		savingsUndetermined:   false,
		savingsBalance:        nil, // savings found but balance unknown
		totalSpendingBudget:   2000,
		savingsTarget:         200,
		mtdSpending:           800,
		mtdSavingsContributed: 0,
		fixedSafetyMargin:     500,
	}
	got := compute(in)
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric result even with unknown savings balance, got %s", got.Kind)
	}
	if !got.SavingsUnknown {
		t.Error("SavingsUnknown: want true, got false")
	}
	if got.CurrentSavings != 0 {
		t.Errorf("CurrentSavings: want 0 when unknown, got %v", got.CurrentSavings)
	}
	if got.SuggestedSweep != 1100 {
		t.Errorf("SuggestedSweep: want 1100 (savings balance does not affect arithmetic), got %v", got.SuggestedSweep)
	}
}

// --- Criterion 7: fixed_safety_margin default $500, configurable ---

func TestFixedSafetyMarginConfigurable(t *testing.T) {
	// Base: margin=$500, checking=$3000, reserve=0 (no budget, no MTD).
	// sweep = 3000 - 0 - 500 = 2500
	base := computeInput{
		checking:          ptr(3000),
		savingsBalance:    ptr(0),
		fixedSafetyMargin: 500, // default
	}
	gotBase := compute(base)
	if gotBase.SuggestedSweep != 2500 {
		t.Errorf("default margin $500: SuggestedSweep want 2500, got %v", gotBase.SuggestedSweep)
	}

	// Custom margin=$1000: sweep = 3000 - 0 - 1000 = 2000
	custom := computeInput{
		checking:          ptr(3000),
		savingsBalance:    ptr(0),
		fixedSafetyMargin: 1000,
	}
	gotCustom := compute(custom)
	if gotCustom.SuggestedSweep != 2000 {
		t.Errorf("custom margin $1000: SuggestedSweep want 2000, got %v", gotCustom.SuggestedSweep)
	}

	// Custom margin=$250: sweep = 3000 - 0 - 250 = 2750
	small := computeInput{
		checking:          ptr(3000),
		savingsBalance:    ptr(0),
		fixedSafetyMargin: 250,
	}
	gotSmall := compute(small)
	if gotSmall.SuggestedSweep != 2750 {
		t.Errorf("custom margin $250: SuggestedSweep want 2750, got %v", gotSmall.SuggestedSweep)
	}

	// Verify FixedSafetyMargin is carried in the result.
	if gotBase.FixedSafetyMargin != 500 {
		t.Errorf("FixedSafetyMargin in result: want 500, got %v", gotBase.FixedSafetyMargin)
	}
	if gotCustom.FixedSafetyMargin != 1000 {
		t.Errorf("FixedSafetyMargin in result: want 1000, got %v", gotCustom.FixedSafetyMargin)
	}
}

// --- Needs-attention: checking undetermined ---

func TestCheckingUndetermined(t *testing.T) {
	in := computeInput{
		checking:            nil, // undetermined
		savingsUndetermined: false,
		savingsBalance:      ptr(500),
		fixedSafetyMargin:   500,
	}
	got := compute(in)
	if got.Kind != KindNeedsAttention {
		t.Fatalf("expected needs_attention, got %s", got.Kind)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != ReasonCheckingUndetermined {
		t.Errorf("Reasons: want [checking_undetermined], got %v", got.Reasons)
	}
}

// --- Needs-attention: savings undetermined ---

func TestSavingsUndetermined(t *testing.T) {
	in := computeInput{
		checking:            ptr(3000),
		savingsUndetermined: true,
		fixedSafetyMargin:   500,
	}
	got := compute(in)
	if got.Kind != KindNeedsAttention {
		t.Fatalf("expected needs_attention, got %s", got.Kind)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != ReasonSavingsUndetermined {
		t.Errorf("Reasons: want [savings_undetermined], got %v", got.Reasons)
	}
}

// --- Needs-attention: both undetermined ---

func TestBothUndetermined(t *testing.T) {
	in := computeInput{
		checking:            nil,
		savingsUndetermined: true,
		fixedSafetyMargin:   500,
	}
	got := compute(in)
	if got.Kind != KindNeedsAttention {
		t.Fatalf("expected needs_attention, got %s", got.Kind)
	}
	if len(got.Reasons) != 2 {
		t.Errorf("Reasons: want 2 reasons, got %v", got.Reasons)
	}
}

// --- sweep can be negative (no floor) ---

func TestNegativeSweepNotFloored(t *testing.T) {
	// Checking is low relative to reserve: result should be negative (pull from savings).
	// reserve = max(0, 2000-0) + max(0, 200-0) = 2200
	// suggested_sweep = 500 - 2200 - 500 = -2200
	in := computeInput{
		checking:            ptr(500),
		savingsBalance:      ptr(5000),
		totalSpendingBudget: 2000,
		savingsTarget:       200,
		fixedSafetyMargin:   500,
	}
	got := compute(in)
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric, got %s", got.Kind)
	}
	if got.SuggestedSweep >= 0 {
		t.Errorf("SuggestedSweep: want negative, got %v", got.SuggestedSweep)
	}
	if got.Direction != DirectionSavingsToChecking {
		t.Errorf("Direction: want savings->checking, got %s", got.Direction)
	}
}
