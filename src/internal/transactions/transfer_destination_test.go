package transactions

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// transferBankTxn builds a seam transaction carrying a transfer-signal bank
// category, so auto-categorization classifies it Transfer and the pairing pass
// considers it. A positive amount is an outflow source leg, a negative amount an
// inflow mirror leg.
func transferBankTxn(id, providerAccountID string, day int, amount float64) banking.Transaction {
	primary := "TRANSFER_OUT"
	if amount < 0 {
		primary = "TRANSFER_IN"
	}
	return banking.Transaction{
		ID:           id,
		AccountID:    providerAccountID,
		Date:         txnDate(day),
		Amount:       banking.Money{Amount: amount, Currency: "USD"},
		Merchant:     "Transfer " + id,
		Counterparty: "RAW " + id,
		Category:     banking.Category{Primary: primary, Detailed: primary + "_ACCOUNT_TRANSFER"},
	}
}

// savingsAccount builds a cash account flagged counts-as-savings, the destination
// kind that makes a paired transfer a savings contribution.
func savingsAccount(providerID, name string) banking.Account {
	a := cashAccount(providerID, name)
	a.Subtype = "savings"
	a.CountsAsSavings = true
	return a
}

// getTransferFacet reads back one transaction's stored transfer facet through the
// repo getter — the internal read the public read-model fields will later expose.
func getTransferFacet(t *testing.T, svc *Service, id string) transferDestination {
	t.Helper()
	td, err := svc.repo().GetTransferDestination(testCtx(), id)
	if err != nil {
		t.Fatalf("GetTransferDestination(%q): %v", id, err)
	}
	return td
}

// setTransferOverridden flips a row's transfer-destination override flag directly,
// standing in for the manual mark operation the next slice adds, so the pass's
// skip-overridden behaviour is testable here.
func setTransferOverridden(t *testing.T, database *db.DB, id string) {
	t.Helper()
	if _, err := database.Sql().Exec("UPDATE transactions SET transfer_destination_overridden = 1 WHERE id = ?", id); err != nil {
		t.Fatalf("mark %q overridden: %v", id, err)
	}
}

// TestSyncResolvesTransferDestinations exercises the auto-pairing pass over a
// connection holding a checking account and a counts-as-savings account: a $500
// outflow on checking and its matching $500 inflow on savings, within the window.
func TestSyncResolvesTransferDestinations(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{
		cashAccount("p-check", "Everyday Checking"),
		savingsAccount("p-save", "High-Yield Savings"),
	}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					transferBankTxn("t-out", "p-check", 4, 500.00),
					transferBankTxn("t-in", "p-save", 4, -500.00),
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	internal := providerToInternal(t, accountsSvc)

	t.Run("the outflow leg resolves to a savings contribution with the savings destination", func(t *testing.T) {
		facet := getTransferFacet(t, svc, "t-out")
		if facet.Subtype != categorization.SubtypeSavingsContribution {
			t.Errorf("subtype = %q, want %q", facet.Subtype, categorization.SubtypeSavingsContribution)
		}
		if facet.DestinationAccountID == nil {
			t.Fatalf("destination is nil, want the savings account id")
		}
		if *facet.DestinationAccountID != internal["p-save"] {
			t.Errorf("destination = %q, want savings id %q", *facet.DestinationAccountID, internal["p-save"])
		}
	})

	t.Run("the inflow mirror leg is never labeled", func(t *testing.T) {
		facet := getTransferFacet(t, svc, "t-in")
		if facet.Subtype != categorization.SubtypeNone {
			t.Errorf("mirror subtype = %q, want empty (the mirror is never the carrier)", facet.Subtype)
		}
		if facet.DestinationAccountID != nil {
			t.Errorf("mirror destination = %q, want nil", *facet.DestinationAccountID)
		}
	})

	t.Run("re-running sync leaves the resolved non-overridden leg unchanged", func(t *testing.T) {
		before := getTransferFacet(t, svc, "t-out")
		if err := svc.SyncTransactions(ctx); err != nil {
			t.Fatalf("SyncTransactions (re-run): %v", err)
		}
		after := getTransferFacet(t, svc, "t-out")
		if after.Subtype != before.Subtype {
			t.Errorf("subtype changed on re-sync: %q -> %q", before.Subtype, after.Subtype)
		}
		if !equalStringPtr(after.DestinationAccountID, before.DestinationAccountID) {
			t.Errorf("destination changed on re-sync")
		}
		if after.Overridden {
			t.Errorf("re-sync marked the leg overridden; the auto pass must never set the manual facet")
		}
	})
}

// TestSyncTransferDestinationUnknownWhenUnpaired drives an outflow transfer with no
// matching inflow leg — the conservative no-match case.
func TestSyncTransferDestinationUnknownWhenUnpaired(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{
		cashAccount("p-check", "Everyday Checking"),
		savingsAccount("p-save", "High-Yield Savings"),
	}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				// A lone outflow transfer; its $250 has no matching inflow leg.
				Added:  []banking.Transaction{transferBankTxn("t-lonely", "p-check", 4, 250.00)},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	t.Run("an unmatched outflow transfer is destination-unknown with subtype plain", func(t *testing.T) {
		facet := getTransferFacet(t, svc, "t-lonely")
		if facet.DestinationAccountID != nil {
			t.Errorf("destination = %q, want nil (no match)", *facet.DestinationAccountID)
		}
		if facet.Subtype != categorization.SubtypePlain {
			t.Errorf("subtype = %q, want %q (conservative: never counted as savings)", facet.Subtype, categorization.SubtypePlain)
		}
	})
}

// TestSyncSkipsOverriddenTransferLeg proves the pass respects the
// transfer-destination override facet the next slice will set: a row marked
// overridden keeps its facet and is not rewritten by a later sync.
func TestSyncSkipsOverriddenTransferLeg(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{
		cashAccount("p-check", "Everyday Checking"),
		savingsAccount("p-save", "High-Yield Savings"),
	}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					transferBankTxn("t-out", "p-check", 4, 500.00),
					transferBankTxn("t-in", "p-save", 4, -500.00),
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	// Stand in for a manual mark that pinned the destination to unknown / plain,
	// against the auto pass that would otherwise resolve it to the savings pair.
	if _, err := database.Sql().Exec(
		"UPDATE transactions SET transfer_destination_account_id = NULL, transfer_subtype = ? WHERE id = ?",
		string(categorization.SubtypePlain), "t-out",
	); err != nil {
		t.Fatalf("seed manual facet: %v", err)
	}
	setTransferOverridden(t, database, "t-out")

	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (re-run): %v", err)
	}

	t.Run("an overridden leg is skipped by the pass and keeps its manual facet", func(t *testing.T) {
		facet := getTransferFacet(t, svc, "t-out")
		if !facet.Overridden {
			t.Fatalf("override flag was cleared; the pass must not touch overridden rows")
		}
		if facet.DestinationAccountID != nil {
			t.Errorf("destination = %q, want nil (manual facet preserved, not re-paired to savings)", *facet.DestinationAccountID)
		}
		if facet.Subtype != categorization.SubtypePlain {
			t.Errorf("subtype = %q, want %q (manual facet preserved)", facet.Subtype, categorization.SubtypePlain)
		}
	})
}
