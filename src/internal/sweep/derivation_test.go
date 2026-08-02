package sweep

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
)

// cashAccount builds a minimal active cash Account for derivation tests.
func cashAccount(id string, countsAsSavings bool, balanceKnown bool, amount float64) accounts.Account {
	return accounts.Account{
		ID:              id,
		Kind:            banking.KindCash,
		State:           accounts.AccountActive,
		CountsAsSavings: countsAsSavings,
		Balance: banking.Balance{
			Known: balanceKnown,
			Money: banking.Money{Amount: amount, Currency: "USD"},
		},
	}
}

// --- Checking derivation ---

func TestDeriveCheckingNone(t *testing.T) {
	// No checking accounts → checking nil → needs-attention "checking_undetermined"
	d := deriveAccounts([]accounts.Account{
		cashAccount("sav1", true, true, 5000),
	})
	if d.checking != nil {
		t.Errorf("checking: want nil (no checking account), got %v", *d.checking)
	}
	got := compute(computeInput{checking: d.checking, savingsUndetermined: d.savingsUndetermined, fixedSafetyMargin: 500})
	assertNeedsAttention(t, got, ReasonCheckingUndetermined)
}

func TestDeriveCheckingMultiple(t *testing.T) {
	// More than one checking account → checking nil → needs-attention "checking_undetermined"
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, true, 3000),
		cashAccount("chk2", false, true, 1000),
		cashAccount("sav1", true, true, 5000),
	})
	if d.checking != nil {
		t.Errorf("checking: want nil (multiple checking accounts), got %v", *d.checking)
	}
	got := compute(computeInput{checking: d.checking, savingsUndetermined: d.savingsUndetermined, fixedSafetyMargin: 500})
	assertNeedsAttention(t, got, ReasonCheckingUndetermined)
}

func TestDeriveCheckingBalanceUnknown(t *testing.T) {
	// Exactly one checking account but balance unknown → checking nil → needs-attention "checking_undetermined"
	// This is the asymmetry: unknown checking blocks; unknown savings does not.
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, false, 0), // balance unknown
		cashAccount("sav1", true, true, 5000),
	})
	if d.checking != nil {
		t.Errorf("checking: want nil (balance unknown), got %v", *d.checking)
	}
	got := compute(computeInput{checking: d.checking, savingsUndetermined: d.savingsUndetermined, savingsBalance: d.savingsBalance, fixedSafetyMargin: 500})
	assertNeedsAttention(t, got, ReasonCheckingUndetermined)
}

func TestDeriveCheckingDetermined(t *testing.T) {
	// Exactly one checking account with known balance → checking set to that balance
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, true, 3000),
		cashAccount("sav1", true, true, 5000),
	})
	if d.checking == nil {
		t.Fatal("checking: want non-nil (single account with known balance), got nil")
	}
	if *d.checking != 3000 {
		t.Errorf("checking: want 3000, got %v", *d.checking)
	}
	if d.checkingID != "chk1" {
		t.Errorf("checkingID: want chk1, got %q", d.checkingID)
	}
}

// --- Savings derivation ---

func TestDeriveSavingsNone(t *testing.T) {
	// No savings accounts → savingsUndetermined → needs-attention "savings_undetermined"
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, true, 3000),
	})
	if !d.savingsUndetermined {
		t.Error("savingsUndetermined: want true (no savings account), got false")
	}
	got := compute(computeInput{checking: d.checking, savingsUndetermined: d.savingsUndetermined, fixedSafetyMargin: 500})
	assertNeedsAttention(t, got, ReasonSavingsUndetermined)
}

func TestDeriveSavingsMultiple(t *testing.T) {
	// More than one savings account → savingsUndetermined → needs-attention "savings_undetermined"
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, true, 3000),
		cashAccount("sav1", true, true, 5000),
		cashAccount("sav2", true, true, 2000),
	})
	if !d.savingsUndetermined {
		t.Error("savingsUndetermined: want true (multiple savings accounts), got false")
	}
	got := compute(computeInput{checking: d.checking, savingsUndetermined: d.savingsUndetermined, fixedSafetyMargin: 500})
	assertNeedsAttention(t, got, ReasonSavingsUndetermined)
}

func TestDeriveSavingsDetermined(t *testing.T) {
	// Exactly one savings account with known balance → determined, balance set
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, true, 3000),
		cashAccount("sav1", true, true, 5000),
	})
	if d.savingsUndetermined {
		t.Error("savingsUndetermined: want false, got true")
	}
	if d.savingsBalance == nil {
		t.Fatal("savingsBalance: want non-nil, got nil")
	}
	if *d.savingsBalance != 5000 {
		t.Errorf("savingsBalance: want 5000, got %v", *d.savingsBalance)
	}
}

func TestDeriveSavingsBalanceUnknown(t *testing.T) {
	// One savings account with unknown balance → determined (not needs-attention),
	// savingsBalance nil; result is numeric with SavingsUnknown=true.
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, true, 3000),
		cashAccount("sav1", true, false, 0), // balance unknown
	})
	if d.savingsUndetermined {
		t.Error("savingsUndetermined: want false (savings identified, just balance unknown), got true")
	}
	if d.savingsBalance != nil {
		t.Errorf("savingsBalance: want nil (balance unknown), got %v", *d.savingsBalance)
	}
	// The numeric result must still proceed and carry SavingsUnknown=true.
	got := compute(computeInput{checking: d.checking, savingsUndetermined: d.savingsUndetermined, savingsBalance: d.savingsBalance, fixedSafetyMargin: 500})
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric result when savings balance unknown, got %s", got.Kind)
	}
	if !got.SavingsUnknown {
		t.Error("SavingsUnknown: want true, got false")
	}
}

// --- Both undetermined ---

func TestDeriveBothUndetermined(t *testing.T) {
	// Empty account list → both checking nil and savings undetermined.
	// compute must list BOTH reasons, not just one.
	d := deriveAccounts([]accounts.Account{})
	got := compute(computeInput{checking: d.checking, savingsUndetermined: d.savingsUndetermined, fixedSafetyMargin: 500})
	if got.Kind != KindNeedsAttention {
		t.Fatalf("expected needs_attention, got %s", got.Kind)
	}
	if len(got.Reasons) != 2 {
		t.Errorf("Reasons: want 2 (both conditions), got %v", got.Reasons)
	}
	hasChecking := false
	hasSavings := false
	for _, r := range got.Reasons {
		switch r {
		case ReasonCheckingUndetermined:
			hasChecking = true
		case ReasonSavingsUndetermined:
			hasSavings = true
		}
	}
	if !hasChecking {
		t.Error("Reasons: missing checking_undetermined")
	}
	if !hasSavings {
		t.Error("Reasons: missing savings_undetermined")
	}
}

func TestDeriveCheckingUnknownSavingsMultiple(t *testing.T) {
	// Checking balance unknown + multiple savings → both reasons must appear.
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, false, 0), // unknown balance
		cashAccount("sav1", true, true, 5000),
		cashAccount("sav2", true, true, 2000),
	})
	got := compute(computeInput{checking: d.checking, savingsUndetermined: d.savingsUndetermined, fixedSafetyMargin: 500})
	if got.Kind != KindNeedsAttention {
		t.Fatalf("expected needs_attention, got %s", got.Kind)
	}
	if len(got.Reasons) != 2 {
		t.Errorf("Reasons: want 2, got %v", got.Reasons)
	}
}

// --- Missing budget is NOT needs-attention ---

func TestDeriveMissingBudgetIsNumeric(t *testing.T) {
	// With accounts fully derived but no budget (zero totals), the result is
	// numeric — a missing budget is not a needs-attention condition.
	d := deriveAccounts([]accounts.Account{
		cashAccount("chk1", false, true, 3000),
		cashAccount("sav1", true, true, 5000),
	})
	in := computeInput{
		checking:            d.checking,
		savingsUndetermined: d.savingsUndetermined,
		savingsBalance:      d.savingsBalance,
		totalSpendingBudget: 0, // no budget
		savingsTarget:       0, // no budget
		fixedSafetyMargin:   500,
	}
	got := compute(in)
	if got.Kind != KindNumeric {
		t.Fatalf("expected numeric result with no budget, got %s", got.Kind)
	}
	if got.TotalSpendingBudget != 0 {
		t.Errorf("TotalSpendingBudget: want 0, got %v", got.TotalSpendingBudget)
	}
	if got.SavingsTarget != 0 {
		t.Errorf("SavingsTarget: want 0, got %v", got.SavingsTarget)
	}
}

// assertNeedsAttention checks that got is needs-attention containing the given
// reason. It does not require it to be the only reason (use for single-condition
// assertions; for multi-reason checks use len(got.Reasons) directly).
func assertNeedsAttention(t *testing.T, got Recommendation, reason NeedsAttentionReason) {
	t.Helper()
	if got.Kind != KindNeedsAttention {
		t.Fatalf("expected needs_attention, got %s", got.Kind)
	}
	for _, r := range got.Reasons {
		if r == reason {
			return
		}
	}
	t.Errorf("Reasons: want %s in %v, not found", reason, got.Reasons)
}
