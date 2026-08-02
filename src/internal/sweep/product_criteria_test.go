package sweep

// Product criteria cross-goal tests for the sweep recommendation feature.
//
// These tests span the compute + persistence layers to demonstrate
// whole-product properties that no single-layer test can assert alone.
// Each is keyed to one of the three product criteria:
//
//   PC1 — Self-consistent breakdown: every numeric recommendation persisted or
//          displayed satisfies the identities
//            suggested_sweep = current_checking - reserve - fixed_safety_margin
//            reserve = max(0, total_spending_budget - mtd_spending)
//                    + max(0, savings_target - mtd_savings_contributed)
//          and every component figure travels with the headline.
//
//   PC2 — Never moves money: the savings target is reserved inside checking
//          (it increases Reserve, reducing SuggestedSweep) — the user's
//          budgeted savings transfer is never folded into the swept amount.
//
// PC3 structural tests live in src/internal/architecture/product_criteria_test.go.

import "testing"

// --- PC1 cross-layer: arithmetic identities survive the compute→store→load cycle ---

// TestPC1_BreakdownIdentitiesHoldAfterRoundTrip computes a recommendation,
// persists it to the database, reloads it, then asserts both arithmetic
// identities on the retrieved value. Spanning compute + persistence proves the
// whole-product explainability invariant: any value a user sees satisfies its
// own arithmetic, even after round-tripping through storage.
func TestPC1_BreakdownIdentitiesHoldAfterRoundTrip(t *testing.T) {
	in := computeInput{
		checking:              ptr(4200),
		savingsBalance:        ptr(1500),
		totalSpendingBudget:   2000,
		savingsTarget:         300,
		mtdSpending:           600,
		mtdSavingsContributed: 100,
		fixedSafetyMargin:     500,
	}
	computed := compute(in)
	if computed.Kind != KindNumeric {
		t.Fatalf("expected numeric result from compute, got %s", computed.Kind)
	}

	repo := newTestRepo(t)
	ctx := testCtx()
	if err := repo.SaveLatest(ctx, computed); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("LoadLatest: found=false after save")
	}

	// Identity 1: suggested_sweep = current_checking - reserve - fixed_safety_margin
	wantSweep := got.CurrentChecking - got.Reserve - got.FixedSafetyMargin
	if got.SuggestedSweep != wantSweep {
		t.Errorf("PC1 identity 1 violated after round-trip: "+
			"SuggestedSweep(%v) != CurrentChecking(%v) - Reserve(%v) - FixedSafetyMargin(%v) = %v",
			got.SuggestedSweep, got.CurrentChecking, got.Reserve, got.FixedSafetyMargin, wantSweep)
	}

	// Identity 2: reserve = max(0, total_spending_budget - mtd_spending)
	//                      + max(0, savings_target - mtd_savings_contributed)
	spendRes := got.TotalSpendingBudget - got.MtdSpending
	if spendRes < 0 {
		spendRes = 0
	}
	savRes := got.SavingsTarget - got.MtdSavingsContributed
	if savRes < 0 {
		savRes = 0
	}
	wantReserve := spendRes + savRes
	if got.Reserve != wantReserve {
		t.Errorf("PC1 identity 2 violated after round-trip: "+
			"Reserve(%v) != max(0, TotalSpendingBudget(%v)-MtdSpending(%v)) + max(0, SavingsTarget(%v)-MtdSavingsContributed(%v)) = %v",
			got.Reserve, got.TotalSpendingBudget, got.MtdSpending,
			got.SavingsTarget, got.MtdSavingsContributed, wantReserve)
	}

	// Every component figure must be non-zero for this test's inputs (no silent
	// data loss through the persistence layer).
	if got.CurrentChecking == 0 {
		t.Error("PC1: CurrentChecking missing from retrieved recommendation")
	}
	if got.TotalSpendingBudget == 0 {
		t.Error("PC1: TotalSpendingBudget missing from retrieved recommendation")
	}
	if got.SavingsTarget == 0 {
		t.Error("PC1: SavingsTarget missing from retrieved recommendation")
	}
	if got.MtdSpending == 0 {
		t.Error("PC1: MtdSpending missing from retrieved recommendation")
	}
	if got.MtdSavingsContributed == 0 {
		t.Error("PC1: MtdSavingsContributed missing from retrieved recommendation")
	}
	if got.Reserve == 0 {
		t.Error("PC1: Reserve missing from retrieved recommendation")
	}
	if got.FixedSafetyMargin == 0 {
		t.Error("PC1: FixedSafetyMargin missing from retrieved recommendation")
	}
}

// TestPC1_IdentitiesHoldAcrossNeedsBoundaryOnReplace saves a numeric
// recommendation, replaces it with a needs-attention result, then replaces it
// once more with a fresh numeric result and re-verifies the identities. The
// replace cycle exercises the idempotent-store behaviour the persistence layer
// guarantees; the identities must hold regardless of how many times the stored
// value has been replaced.
func TestPC1_IdentitiesHoldAcrossNeedsBoundaryOnReplace(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	// Round 1: numeric.
	rec1 := compute(computeInput{
		checking:            ptr(3000),
		savingsBalance:      ptr(500),
		totalSpendingBudget: 2000,
		savingsTarget:       200,
		mtdSpending:         800,
		fixedSafetyMargin:   500,
	})
	if err := repo.SaveLatest(ctx, rec1); err != nil {
		t.Fatalf("first SaveLatest: %v", err)
	}

	// Round 2: needs-attention (replaces numeric).
	na := Recommendation{Kind: KindNeedsAttention, Reasons: []NeedsAttentionReason{ReasonCheckingUndetermined}}
	if err := repo.SaveLatest(ctx, na); err != nil {
		t.Fatalf("needs-attention SaveLatest: %v", err)
	}

	// Round 3: fresh numeric (replaces needs-attention).
	rec3 := compute(computeInput{
		checking:              ptr(5000),
		savingsBalance:        ptr(2000),
		totalSpendingBudget:   3000,
		savingsTarget:         500,
		mtdSpending:           1200,
		mtdSavingsContributed: 250,
		fixedSafetyMargin:     500,
	})
	if err := repo.SaveLatest(ctx, rec3); err != nil {
		t.Fatalf("third SaveLatest: %v", err)
	}

	got, found, err := repo.LoadLatest(ctx)
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if !found {
		t.Fatal("LoadLatest: found=false after three saves")
	}
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric (third save), got %s", got.Kind)
	}

	// Identity 1.
	wantSweep := got.CurrentChecking - got.Reserve - got.FixedSafetyMargin
	if got.SuggestedSweep != wantSweep {
		t.Errorf("PC1 identity 1 after triple replace: SuggestedSweep(%v) != %v",
			got.SuggestedSweep, wantSweep)
	}

	// Identity 2.
	spendRes := got.TotalSpendingBudget - got.MtdSpending
	if spendRes < 0 {
		spendRes = 0
	}
	savRes := got.SavingsTarget - got.MtdSavingsContributed
	if savRes < 0 {
		savRes = 0
	}
	wantReserve := spendRes + savRes
	if got.Reserve != wantReserve {
		t.Errorf("PC1 identity 2 after triple replace: Reserve(%v) != %v", got.Reserve, wantReserve)
	}
}

// --- PC2: savings target reserves money in checking, never folds into the swept amount ---

// TestPC2_SavingsTargetRaisesReserveByExactAmount asserts that adding a savings
// target of $X raises Reserve by exactly $X (when not yet reached) and lowers
// SuggestedSweep by exactly $X. The user's planned savings transfer is reserved
// inside checking — the sweep recommendation never absorbs it into the swept
// figure.
func TestPC2_SavingsTargetRaisesReserveByExactAmount(t *testing.T) {
	base := computeInput{
		checking:            ptr(3000),
		savingsBalance:      ptr(500),
		totalSpendingBudget: 2000,
		savingsTarget:       0, // no savings target in base case
		mtdSpending:         600,
		fixedSafetyMargin:   500,
	}
	withSavings := base
	withSavings.savingsTarget = 400

	gotBase := compute(base)
	gotWith := compute(withSavings)

	if gotBase.Kind != KindNumeric || gotWith.Kind != KindNumeric {
		t.Fatalf("both scenarios must be numeric: base=%s with=%s", gotBase.Kind, gotWith.Kind)
	}

	reserveDelta := gotWith.Reserve - gotBase.Reserve
	sweepDelta := gotWith.SuggestedSweep - gotBase.SuggestedSweep

	if reserveDelta != 400 {
		t.Errorf("PC2: savings target $400 must raise Reserve by exactly $400 "+
			"(keeping it reserved in checking); got reserve delta %v", reserveDelta)
	}
	if sweepDelta != -400 {
		t.Errorf("PC2: savings target $400 must lower SuggestedSweep by exactly $400 "+
			"(not folded into the swept amount); got sweep delta %v", sweepDelta)
	}

	// The checking balance itself is unchanged: the reserve is a logical hold, not
	// a transfer. The recommendation observes; it never moves money.
	if gotBase.CurrentChecking != gotWith.CurrentChecking {
		t.Errorf("PC2: CurrentChecking must be identical regardless of savings target: %v vs %v",
			gotBase.CurrentChecking, gotWith.CurrentChecking)
	}
}

// TestPC2_AlreadyMetSavingsTargetDoesNotReduceReserveBelow0 asserts that when
// the user has already saved more than the target (MtdSavingsContributed >=
// SavingsTarget), the savings reserve term is floored at 0 — the already-met
// target does not inflate the swept amount or create a negative savings reserve
// that would mask the spending reserve. The user's past savings transfer is
// theirs; it is never reclaimed by the sweep arithmetic.
func TestPC2_AlreadyMetSavingsTargetDoesNotReduceReserveBelow0(t *testing.T) {
	// Savings target $200, contributed $350 — over target by $150.
	// Savings reserve = max(0, 200 - 350) = 0 (not -150).
	// Spending reserve = max(0, 2000 - 600) = 1400.
	// Reserve = 1400 + 0 = 1400.
	// SuggestedSweep = 3000 - 1400 - 500 = 1100.
	in := computeInput{
		checking:              ptr(3000),
		savingsBalance:        ptr(500),
		totalSpendingBudget:   2000,
		savingsTarget:         200,
		mtdSpending:           600,
		mtdSavingsContributed: 350, // over target
		fixedSafetyMargin:     500,
	}
	got := compute(in)
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric, got %s", got.Kind)
	}
	// Savings reserve is 0 — past-target savings must not inflate the sweep.
	savingsReserve := got.SavingsTarget - got.MtdSavingsContributed
	if savingsReserve > 0 {
		t.Errorf("PC2: savings reserve should be floored at 0 when target is met; got implicit savingsReserve %v", savingsReserve)
	}
	// Reserve must equal the spending reserve only.
	if got.Reserve != 1400 {
		t.Errorf("PC2: Reserve want 1400 (spending reserve only, savings term floored); got %v", got.Reserve)
	}
	// SuggestedSweep satisfies identity 1.
	wantSweep := got.CurrentChecking - got.Reserve - got.FixedSafetyMargin
	if got.SuggestedSweep != wantSweep {
		t.Errorf("PC2: identity 1 violated: SuggestedSweep(%v) != CurrentChecking(%v)-Reserve(%v)-FixedSafetyMargin(%v)=%v",
			got.SuggestedSweep, got.CurrentChecking, got.Reserve, got.FixedSafetyMargin, wantSweep)
	}
}

// TestPC2_SavingsReserveIsAdditiveNotSubtractiveFromSweep asserts the
// structural property: SavingsTarget enters the formula via Reserve (where it
// protects money in checking), never directly into SuggestedSweep with the
// opposite sign (which would add to what the user sweeps). A sweep feature that
// folds savings into the swept amount would silently move the user's planned
// savings transfer out of the checking account; this test proves that does not
// happen.
func TestPC2_SavingsReserveIsAdditiveNotSubtractiveFromSweep(t *testing.T) {
	// Two inputs that differ only in SavingsTarget: $0 vs $500.
	// With $0 savings target the sweep is larger (more can be sent to savings).
	// With $500 savings target the sweep is smaller (that $500 stays in checking).
	noSavings := computeInput{
		checking:            ptr(4000),
		savingsBalance:      ptr(1000),
		totalSpendingBudget: 1500,
		savingsTarget:       0,
		fixedSafetyMargin:   500,
	}
	withSavings := noSavings
	withSavings.savingsTarget = 500

	gotNoSav := compute(noSavings)
	gotWithSav := compute(withSavings)

	if gotNoSav.Kind != KindNumeric || gotWithSav.Kind != KindNumeric {
		t.Fatalf("both must be numeric")
	}

	// Adding a savings target must strictly reduce SuggestedSweep (the money is
	// reserved in checking, not added to the sweep).
	if gotWithSav.SuggestedSweep >= gotNoSav.SuggestedSweep {
		t.Errorf("PC2: savings target $500 must reduce SuggestedSweep "+
			"(reserved in checking for user's own transfer); "+
			"without savings: %v, with savings: %v",
			gotNoSav.SuggestedSweep, gotWithSav.SuggestedSweep)
	}

	// The reduction in SuggestedSweep equals the savings target exactly (since no
	// prior contributions — the full $500 is still owed to the savings reserve).
	reduction := gotNoSav.SuggestedSweep - gotWithSav.SuggestedSweep
	if reduction != 500 {
		t.Errorf("PC2: SuggestedSweep reduction must equal savings target $500; got %v", reduction)
	}

	// Reserve in the with-savings case is larger than without-savings by exactly $500.
	if gotWithSav.Reserve-gotNoSav.Reserve != 500 {
		t.Errorf("PC2: Reserve delta must equal savings target $500; got delta %v",
			gotWithSav.Reserve-gotNoSav.Reserve)
	}
}
