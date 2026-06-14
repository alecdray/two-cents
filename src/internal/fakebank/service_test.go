package fakebank_test

import (
	"context"
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/fakebank"
)

// compile-time proof the stand-in satisfies the whole seam, from outside the
// package, depending only on banking + contextx.
var _ banking.BankProvider = (*fakebank.Service)(nil)

func testContext() contextx.ContextX {
	return contextx.NewContextX(context.Background())
}

// CreateLinkToken mints a non-empty token tagged "fake" so the front end opens
// the simulated connect flow, in both new-connect and update mode.
func TestCreateLinkTokenIsTaggedFake(t *testing.T) {
	svc := fakebank.NewService()

	for _, opts := range []banking.LinkOptions{
		{},                              // new connection
		{AccessToken: "existing-login"}, // update mode
	} {
		token, err := svc.CreateLinkToken(testContext(), opts)
		if err != nil {
			t.Fatalf("CreateLinkToken: %v", err)
		}
		if token.Mode != "fake" {
			t.Errorf("Mode = %q, want %q", token.Mode, "fake")
		}
		if token.Token == "" {
			t.Error("Token is empty; want a fixed non-empty value")
		}
	}
}

// ExchangePublicToken returns the canned durable bank login.
func TestExchangePublicTokenReturnsCannedItem(t *testing.T) {
	svc := fakebank.NewService()

	item, err := svc.ExchangePublicToken(testContext(), "any-public-token")
	if err != nil {
		t.Fatalf("ExchangePublicToken: %v", err)
	}
	if item.AccessToken == "" {
		t.Error("AccessToken is empty; want a fixed non-empty value")
	}
	if item.ProviderItemID == "" {
		t.Error("ProviderItemID is empty; want a fixed non-empty value")
	}
}

// RemoveItem is a no-op that reports success.
func TestRemoveItemSucceeds(t *testing.T) {
	svc := fakebank.NewService()

	if err := svc.RemoveItem(testContext(), "any-access-token"); err != nil {
		t.Errorf("RemoveItem returned %v, want nil", err)
	}
}

// ListAccounts returns exactly the fixed set: a checking and savings cash
// account and a credit card, with the documented names, kinds, and balances.
func TestListAccountsReturnsFixedSet(t *testing.T) {
	svc := fakebank.NewService()

	accounts, err := svc.ListAccounts(testContext(), "any-access-token")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}

	type want struct {
		name            string
		kind            banking.AccountKind
		accountType     string
		subtype         string
		amount          float64
		countsAsSavings bool
	}
	wants := []want{
		{"Everyday Checking", banking.KindCash, "depository", "checking", 1200.00, false},
		{"High-Yield Savings", banking.KindCash, "depository", "savings", 3400.00, true},
		{"Travel Rewards Card", banking.KindCredit, "credit", "credit card", 450.00, false},
	}

	if len(accounts) != len(wants) {
		t.Fatalf("got %d accounts, want %d", len(accounts), len(wants))
	}
	for i, w := range wants {
		a := accounts[i]
		if a.Name != w.name {
			t.Errorf("account[%d].Name = %q, want %q", i, a.Name, w.name)
		}
		if a.Kind != w.kind {
			t.Errorf("account[%d].Kind = %q, want %q", i, a.Kind, w.kind)
		}
		if a.Type != w.accountType {
			t.Errorf("account[%d].Type = %q, want %q", i, a.Type, w.accountType)
		}
		if a.Subtype != w.subtype {
			t.Errorf("account[%d].Subtype = %q, want %q", i, a.Subtype, w.subtype)
		}
		if a.CountsAsSavings != w.countsAsSavings {
			t.Errorf("account[%d].CountsAsSavings = %v, want %v", i, a.CountsAsSavings, w.countsAsSavings)
		}
		if !a.Balance.Known {
			t.Errorf("account[%d].Balance.Known = false, want true", i)
		}
		if a.Balance.Money.Amount != w.amount {
			t.Errorf("account[%d].Balance.Money.Amount = %v, want %v", i, a.Balance.Money.Amount, w.amount)
		}
		if a.Balance.Money.Currency != "USD" {
			t.Errorf("account[%d].Balance.Money.Currency = %q, want USD", i, a.Balance.Money.Currency)
		}
		if a.Balance.AccountID != a.ID {
			t.Errorf("account[%d].Balance.AccountID = %q, want %q", i, a.Balance.AccountID, a.ID)
		}
	}
}

// GetBalances reports a known balance per fixed account, matching the balances
// ListAccounts carries.
func TestGetBalancesMatchAccounts(t *testing.T) {
	svc := fakebank.NewService()
	ctx := testContext()

	accounts, err := svc.ListAccounts(ctx, "any-access-token")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	balances, err := svc.GetBalances(ctx, "any-access-token")
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}

	if len(balances) != len(accounts) {
		t.Fatalf("got %d balances, want %d (one per account)", len(balances), len(accounts))
	}
	byID := make(map[string]banking.Balance, len(balances))
	for _, b := range balances {
		byID[b.AccountID] = b
	}
	for _, a := range accounts {
		b, ok := byID[a.ID]
		if !ok {
			t.Errorf("no balance for account %q", a.ID)
			continue
		}
		if b != a.Balance {
			t.Errorf("balance for %q = %+v, want %+v", a.ID, b, a.Balance)
		}
	}
}

// SyncTransactions reports no transactions and settles immediately (the cursor
// does not advance), so a draining consumer terminates.
func TestSyncTransactionsReportsNoChanges(t *testing.T) {
	svc := fakebank.NewService()

	changes, err := svc.SyncTransactions(testContext(), "any-access-token", "")
	if err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	if len(changes.Added) != 0 || len(changes.Modified) != 0 || len(changes.RemovedIDs) != 0 {
		t.Errorf("expected no changes, got %+v", changes)
	}
	if changes.Cursor != "" {
		t.Errorf("Cursor = %q, want unchanged empty cursor", changes.Cursor)
	}
}
