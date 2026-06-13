package banking_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// fakeProvider is a hand-written, in-memory implementation of
// banking.BankProvider. It depends on nothing but the banking package and the
// shared contextx — no plaid, no network. Its existence is the proof: a
// downstream consumer can be built and exercised against the seam without the
// provider client anywhere in scope. This mirrors the "fake BankProvider"
// testing approach the project documents in docs/testing.md.
type fakeProvider struct {
	accounts []banking.Account
	balances []banking.Balance
	// pages is the sequence of change-sets SyncTransactions hands back, one per
	// call, modelling a provider that paginates and then settles to no changes.
	pages   []banking.TransactionChanges
	syncErr error
}

func (f *fakeProvider) ListAccounts(_ contextx.ContextX, _ string) ([]banking.Account, error) {
	return f.accounts, nil
}

func (f *fakeProvider) GetBalances(_ contextx.ContextX, _ string) ([]banking.Balance, error) {
	return f.balances, nil
}

func (f *fakeProvider) SyncTransactions(_ contextx.ContextX, _, cursor string) (banking.TransactionChanges, error) {
	if f.syncErr != nil {
		return banking.TransactionChanges{}, f.syncErr
	}
	// Resume from where the prior call left off: the cursor is the index into
	// the page sequence, encoded as the count of pages already consumed.
	idx := len(cursor) // empty cursor -> page 0; "x" -> page 1; ...
	if idx >= len(f.pages) {
		// No further changes; echo the cursor back unchanged, as a real
		// provider does on a no-change sync.
		return banking.TransactionChanges{Cursor: cursor}, nil
	}
	return f.pages[idx], nil
}

// compile-time proof that a consumer-defined fake satisfies the seam using only
// banking + contextx.
var _ banking.BankProvider = (*fakeProvider)(nil)

// spendableCash is a tiny consumer of the seam: it sums the known cash-account
// balances reachable through a BankProvider. It is the kind of code a domain
// module (e.g. accounts) would own. It names only banking types and the
// interface — never a provider.
func spendableCash(ctx contextx.ContextX, provider banking.BankProvider, accessToken string) (banking.Money, error) {
	accounts, err := provider.ListAccounts(ctx, accessToken)
	if err != nil {
		return banking.Money{}, err
	}
	balances, err := provider.GetBalances(ctx, accessToken)
	if err != nil {
		return banking.Money{}, err
	}

	kindByID := make(map[string]banking.AccountKind, len(accounts))
	for _, a := range accounts {
		kindByID[a.ID] = a.Kind
	}

	total := banking.Money{Currency: "USD"}
	for _, b := range balances {
		if !b.Known {
			continue
		}
		if kindByID[b.AccountID] != banking.KindCash {
			continue
		}
		total.Amount += b.Money.Amount
		total.Currency = b.Money.Currency
	}
	return total, nil
}

func testContext() contextx.ContextX {
	return contextx.NewContextX(context.Background())
}

// TestBankProviderUsableWithoutPlaid proves the seam is consumable end to end
// against a fake provider, with no dependency on the plaid client.
func TestBankProviderUsableWithoutPlaid(t *testing.T) {
	provider := &fakeProvider{
		accounts: []banking.Account{
			{ID: "chk", Name: "Checking", Kind: banking.KindCash},
			{ID: "sav", Name: "Savings", Kind: banking.KindCash, CountsAsSavings: true},
			{ID: "card", Name: "Card", Kind: banking.KindCredit},
		},
		balances: []banking.Balance{
			{AccountID: "chk", Known: true, Money: banking.Money{Amount: 110.0, Currency: "USD"}},
			{AccountID: "sav", Known: true, Money: banking.Money{Amount: 50.0, Currency: "USD"}},
			{AccountID: "card", Known: true, Money: banking.Money{Amount: 410.0, Currency: "USD"}},
			{AccountID: "mystery", Known: false},
		},
		pages: []banking.TransactionChanges{
			{
				Added: []banking.Transaction{
					{ID: "t1", AccountID: "chk", Date: time.Now(), Amount: banking.Money{Amount: 6.33, Currency: "USD"}, Merchant: "Coffee", Category: banking.Category{Primary: "FOOD_AND_DRINK", Detailed: "FOOD_AND_DRINK_COFFEE"}},
				},
				Modified:   nil,
				RemovedIDs: nil,
				Cursor:     "x", // length 1 -> next call reads page 1
			},
		},
	}

	t.Run("a consumer sums only known cash balances through the interface", func(t *testing.T) {
		got, err := spendableCash(testContext(), provider, "access-token")
		if err != nil {
			t.Fatalf("computing spendable cash: %v", err)
		}
		// chk (110) + sav (50); the credit card and the unknown balance are excluded.
		if got.Amount != 160.0 {
			t.Errorf("expected 160.0 spendable cash, got %v", got.Amount)
		}
		if got.Currency != "USD" {
			t.Errorf("expected USD, got %q", got.Currency)
		}
	})

	t.Run("a consumer drains the sync cursor to completion", func(t *testing.T) {
		ctx := testContext()
		var added []banking.Transaction
		cursor := ""
		for i := 0; i < 10; i++ { // bounded; the fake settles to no-change quickly
			changes, err := provider.SyncTransactions(ctx, "access-token", cursor)
			if err != nil {
				t.Fatalf("syncing: %v", err)
			}
			added = append(added, changes.Added...)
			if changes.Cursor == cursor {
				break // no advance => settled
			}
			cursor = changes.Cursor
		}
		if len(added) != 1 {
			t.Fatalf("expected 1 added transaction across the drain, got %d", len(added))
		}
		if added[0].Category.Detailed != "FOOD_AND_DRINK_COFFEE" {
			t.Errorf("expected the two-level category to carry through, got %+v", added[0].Category)
		}
	})

	t.Run("a sync error surfaces to the consumer unchanged", func(t *testing.T) {
		boom := errors.New("provider unavailable")
		failing := &fakeProvider{syncErr: boom}
		_, err := failing.SyncTransactions(testContext(), "access-token", "")
		if !errors.Is(err, boom) {
			t.Errorf("expected the provider error to surface, got %v", err)
		}
	})
}
