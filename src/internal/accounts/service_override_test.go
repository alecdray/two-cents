package accounts

import (
	"errors"
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// setupOverrideAccounts registers one connection exposing the canonical trio — a
// cash checking account (savings off), a cash savings account (savings on), and a
// credit card — and returns the service plus the stored accounts keyed by their
// provider id, so an override test can target a specific account.
func setupOverrideAccounts(t *testing.T) (*Service, contextx.ContextX, map[string]Account) {
	t.Helper()
	database := newTestDB(t)
	ctx := testCtx()
	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
		providerAccount("p-save", "Savings", banking.KindCash, true, knownBalance("p-save", 1500)),
		providerAccount("p-card", "Card", banking.KindCredit, false, knownBalance("p-card", 300)),
	}}
	svc := NewService(database, provider, testKey)
	if _, err := svc.RegisterConnection(ctx, "tok", "item-1"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	all, err := svc.repo().ListAccounts(ctx)
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	byProvider := make(map[string]Account, len(all))
	for _, a := range all {
		byProvider[a.ProviderAccountID] = a
	}
	return svc, ctx, byProvider
}

func getStoredAccount(t *testing.T, svc *Service, ctx contextx.ContextX, id string) Account {
	t.Helper()
	a, err := svc.repo().GetAccount(ctx, id)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	return a
}

func TestSetAccountKind(t *testing.T) {
	t.Run("override to other marks overridden and leaves counts-as-savings untouched", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"]

		changed, err := svc.SetAccountKind(ctx, check.ID, banking.KindOther)
		if err != nil {
			t.Fatalf("SetAccountKind: %v", err)
		}
		if changed {
			t.Errorf("savingsChanged = true, want false for a cash→other override")
		}
		got := getStoredAccount(t, svc, ctx, check.ID)
		if got.Kind != banking.KindOther {
			t.Errorf("Kind = %q, want other", got.Kind)
		}
		if !got.KindOverridden {
			t.Errorf("KindOverridden = false, want true after an override")
		}
		if got.CountsAsSavings {
			t.Errorf("CountsAsSavings flipped on a cash→other override")
		}
	})

	t.Run("override to credit force-clears counts-as-savings and reports the change", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		save := accts["p-save"]
		if !save.CountsAsSavings {
			t.Fatalf("precondition: the savings account should seed counts-as-savings on")
		}

		changed, err := svc.SetAccountKind(ctx, save.ID, banking.KindCredit)
		if err != nil {
			t.Fatalf("SetAccountKind: %v", err)
		}
		if !changed {
			t.Errorf("savingsChanged = false, want true when a credit override clears a set flag")
		}
		got := getStoredAccount(t, svc, ctx, save.ID)
		if got.Kind != banking.KindCredit {
			t.Errorf("Kind = %q, want credit", got.Kind)
		}
		if got.CountsAsSavings {
			t.Errorf("CountsAsSavings = true, want cleared by the credit override")
		}
		if !got.SavingsOverridden {
			t.Errorf("SavingsOverridden = false, want true after the force-clear")
		}
	})

	t.Run("override to credit on an already-non-savings account reports no change", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"]

		changed, err := svc.SetAccountKind(ctx, check.ID, banking.KindCredit)
		if err != nil {
			t.Fatalf("SetAccountKind: %v", err)
		}
		if changed {
			t.Errorf("savingsChanged = true, want false when the flag was already off")
		}
	})

	t.Run("an invalid kind is rejected and leaves the account untouched", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"]

		_, err := svc.SetAccountKind(ctx, check.ID, banking.AccountKind("bogus"))
		if !errors.Is(err, ErrInvalidKind) {
			t.Fatalf("err = %v, want ErrInvalidKind", err)
		}
		got := getStoredAccount(t, svc, ctx, check.ID)
		if got.KindOverridden || got.Kind != banking.KindCash {
			t.Errorf("account mutated despite an invalid kind: %+v", got)
		}
	})
}

func TestToggleCountsAsSavings(t *testing.T) {
	t.Run("flips the flag in both directions and marks it overridden", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		check := accts["p-check"]

		changed, err := svc.ToggleCountsAsSavings(ctx, check.ID)
		if err != nil {
			t.Fatalf("ToggleCountsAsSavings: %v", err)
		}
		if !changed {
			t.Errorf("savingsChanged = false, want true")
		}
		got := getStoredAccount(t, svc, ctx, check.ID)
		if !got.CountsAsSavings {
			t.Errorf("CountsAsSavings = false, want flipped on")
		}
		if !got.SavingsOverridden {
			t.Errorf("SavingsOverridden = false, want true")
		}

		if _, err := svc.ToggleCountsAsSavings(ctx, check.ID); err != nil {
			t.Fatalf("second ToggleCountsAsSavings: %v", err)
		}
		got = getStoredAccount(t, svc, ctx, check.ID)
		if got.CountsAsSavings {
			t.Errorf("CountsAsSavings = true after a second toggle, want off")
		}
	})

	t.Run("rejected on a credit account", func(t *testing.T) {
		svc, ctx, accts := setupOverrideAccounts(t)
		card := accts["p-card"]

		_, err := svc.ToggleCountsAsSavings(ctx, card.ID)
		if !errors.Is(err, ErrSavingsNotApplicable) {
			t.Errorf("err = %v, want ErrSavingsNotApplicable", err)
		}
	})
}
