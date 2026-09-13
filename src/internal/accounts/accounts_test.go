package accounts

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// BalanceStale is the one definition of "this balance is too old to trust",
// exported so callers outside this package (the sweep) ask the same question
// the overview does instead of carrying a second threshold.
func TestAccountBalanceStale(t *testing.T) {
	now := time.Date(2026, time.September, 8, 18, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time {
		t := now.Add(-d)
		return &t
	}

	cases := []struct {
		name         string
		lastSyncedAt *time.Time
		want         bool
	}{
		{"a balance refreshed on the last pass is current", ago(1 * time.Hour), false},
		{"a few missed passes are tolerated", ago(20 * time.Hour), false},
		{"exactly at the threshold is still current", ago(staleAfter), false},
		{"past the threshold is stale", ago(staleAfter + time.Minute), true},
		{"a long-failing connection is stale", ago(9 * 24 * time.Hour), true},
		{"a never-synced account is stale", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Account{LastSyncedAt: tc.lastSyncedAt}
			if got := a.BalanceStale(now); got != tc.want {
				t.Errorf("BalanceStale() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ActiveCreditAccounts is the read the sweep places card obligations on its
// timeline from (ADR-0024). Unlike the cash reads there is no single-account
// requirement — debt is additive, so every card counts.
func TestActiveCreditAccountsReturnsEveryActiveCard(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
		providerAccount("p-card-a", "Travel Card", banking.KindCredit, false, knownBalance("p-card-a", 450)),
		providerAccount("p-card-b", "Cashback Card", banking.KindCredit, false, knownBalance("p-card-b", 1200)),
		providerAccount("p-loan", "Car Loan", banking.KindOther, false, knownBalance("p-loan", 9000)),
	}}
	svc := NewService(database, provider, testKey)

	if _, err := svc.RegisterConnection(ctx, "tok", "item-123"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	got, err := svc.ActiveCreditAccounts(ctx)
	if err != nil {
		t.Fatalf("ActiveCreditAccounts: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("want both cards, got %d accounts", len(got))
	}
	var total float64
	for _, a := range got {
		if a.Kind != banking.KindCredit {
			t.Errorf("returned a %s account; credit only", a.Kind)
		}
		total += a.Balance.Money.Amount
	}
	if total != 1650 {
		t.Errorf("summed card balances = %v, want 1650 (debt is additive)", total)
	}
}

// A hidden card is out of the picture entirely, like every other hidden account.
func TestActiveCreditAccountsExcludesHidden(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-card-a", "Travel Card", banking.KindCredit, false, knownBalance("p-card-a", 450)),
	}}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	stored, err := svc.repo().ListAccountsByConnection(ctx, conn.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := svc.HideAccount(ctx, stored[0].ID); err != nil {
		t.Fatalf("HideAccount: %v", err)
	}

	got, err := svc.ActiveCreditAccounts(ctx)
	if err != nil {
		t.Fatalf("ActiveCreditAccounts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a hidden card must not count toward what is owed, got %d", len(got))
	}
}
