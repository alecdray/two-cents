package transactions

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
)

// syncedSavingsPair stands up a connection with a checking and a counts-as-savings
// account and syncs a single $500 outflow→inflow transfer pair, so the outflow leg
// t-out is auto-resolved to a savings contribution into the savings account and the
// inflow mirror t-in is left unlabeled. It returns the wired transactions service
// and the provider→internal account-id map, the surface the mark tests assert over.
func syncedSavingsPair(t *testing.T) (*Service, map[string]string) {
	t.Helper()
	database := newTestDB(t)

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
	svc := NewService(database, provider, accountsSvc, newCategorization(database), nil)
	if err := svc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	return svc, providerToInternal(t, accountsSvc)
}

// getCategorizationFacet reads back one transaction's stored categorization facet
// through the repo getter — the other sticky facet, used here to prove a transfer
// mark leaves it untouched.
func getCategorizationFacet(t *testing.T, svc *Service, id string) categorizationRow {
	t.Helper()
	row, err := svc.repo().GetCategorizationRow(testCtx(), id)
	if err != nil {
		t.Fatalf("GetCategorizationRow(%q): %v", id, err)
	}
	return row
}

// TestMarkTransferDestinationPersists proves a manual mark of a destination +
// subtype on an outflow Transfer leg writes through and is reflected when the row's
// transfer facet is read back, marking the facet overridden. It marks an
// auto-unknown transfer to a savings contribution into the connected savings
// account — the "attribute this transfer to savings" path.
func TestMarkTransferDestinationPersists(t *testing.T) {
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
			// A lone outflow transfer: no matching inflow, so the auto pass leaves it
			// destination-unknown (subtype plain) — the user marks it by hand.
			return banking.TransactionChanges{
				Added:  []banking.Transaction{transferBankTxn("t-out", "p-check", 4, 500.00)},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database), nil)
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	internal := providerToInternal(t, accountsSvc)

	// The auto pass left it unknown / plain, not overridden.
	if before := getTransferFacet(t, svc, "t-out"); before.Overridden || before.DestinationAccountID != nil {
		t.Fatalf("precondition: t-out should start auto-unknown and not overridden, got %+v", before)
	}

	savingsID := internal["p-save"]
	if err := svc.MarkTransferDestination(ctx, "t-out", &savingsID, categorization.SubtypeSavingsContribution); err != nil {
		t.Fatalf("MarkTransferDestination: %v", err)
	}

	t.Run("the marked destination + subtype persist and mark the facet overridden", func(t *testing.T) {
		facet := getTransferFacet(t, svc, "t-out")
		if !facet.Overridden {
			t.Errorf("override flag = false, want true after a manual mark")
		}
		if facet.Subtype != categorization.SubtypeSavingsContribution {
			t.Errorf("subtype = %q, want %q", facet.Subtype, categorization.SubtypeSavingsContribution)
		}
		if facet.DestinationAccountID == nil {
			t.Fatalf("destination is nil, want the marked savings account id %q", savingsID)
		}
		if *facet.DestinationAccountID != savingsID {
			t.Errorf("destination = %q, want %q", *facet.DestinationAccountID, savingsID)
		}
	})
}

// TestMarkTransferDestinationStickyAcrossSync proves a manual mark is sticky: a
// later sync's auto-pairing pass does not revert it, even when the pass WOULD
// resolve the leg differently. The savings pair would auto-resolve t-out to a
// savings contribution; the user instead marks it plain with no destination, and
// that choice survives a re-sync.
func TestMarkTransferDestinationStickyAcrossSync(t *testing.T) {
	svc, _ := syncedSavingsPair(t)
	ctx := testCtx()

	// The auto pass resolved it to a savings contribution.
	if before := getTransferFacet(t, svc, "t-out"); before.Subtype != categorization.SubtypeSavingsContribution {
		t.Fatalf("precondition: t-out should auto-resolve to a savings contribution, got %+v", before)
	}

	// The user corrects it: not a savings contribution, no destination.
	if err := svc.MarkTransferDestination(ctx, "t-out", nil, categorization.SubtypePlain); err != nil {
		t.Fatalf("MarkTransferDestination: %v", err)
	}

	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions (re-run): %v", err)
	}

	t.Run("the marked facet survives a re-sync that would have resolved it to savings", func(t *testing.T) {
		facet := getTransferFacet(t, svc, "t-out")
		if !facet.Overridden {
			t.Fatalf("override flag was cleared by the re-sync; a manual mark must stay sticky")
		}
		if facet.Subtype != categorization.SubtypePlain {
			t.Errorf("subtype = %q, want %q (manual choice preserved, not re-paired to savings)", facet.Subtype, categorization.SubtypePlain)
		}
		if facet.DestinationAccountID != nil {
			t.Errorf("destination = %q, want nil (manual choice preserved)", *facet.DestinationAccountID)
		}
	})
}

// TestMarkTransferDestinationFacetIndependence proves the two sticky facets are
// independent in both directions: marking a transfer's destination leaves the
// categorization facet untouched, and re-categorizing a row leaves its transfer
// facet untouched.
func TestMarkTransferDestinationFacetIndependence(t *testing.T) {
	ctx := testCtx()

	t.Run("marking a transfer destination does not change the categorization facet", func(t *testing.T) {
		svc, internal := syncedSavingsPair(t)

		before := getCategorizationFacet(t, svc, "t-out")
		if before.Classification != categorization.Transfer || before.Overridden {
			t.Fatalf("precondition: t-out should be auto-classified transfer and not categorization-overridden, got %+v", before)
		}

		savingsID := internal["p-save"]
		if err := svc.MarkTransferDestination(ctx, "t-out", &savingsID, categorization.SubtypeSavingsContribution); err != nil {
			t.Fatalf("MarkTransferDestination: %v", err)
		}

		after := getCategorizationFacet(t, svc, "t-out")
		if after.Classification != before.Classification {
			t.Errorf("classification changed by a transfer mark: %q -> %q", before.Classification, after.Classification)
		}
		if !equalStringPtr(after.CategoryID, before.CategoryID) {
			t.Errorf("category_id changed by a transfer mark")
		}
		if after.Overridden {
			t.Errorf("categorization_overridden was set by a transfer mark; the two facets must stay independent")
		}
		// Sanity: the transfer facet itself was overridden.
		if !getTransferFacet(t, svc, "t-out").Overridden {
			t.Errorf("the transfer mark did not set its own override flag")
		}
	})

	t.Run("re-categorizing a row that stays a Transfer does not change its transfer facet", func(t *testing.T) {
		svc, _ := syncedSavingsPair(t)

		before := getTransferFacet(t, svc, "t-out")
		if before.Subtype != categorization.SubtypeSavingsContribution || before.DestinationAccountID == nil {
			t.Fatalf("precondition: t-out should auto-resolve to a savings contribution with a destination, got %+v", before)
		}

		// Re-categorize while keeping the row a Transfer: the transfer facet stays
		// independent of the categorization write. (Moving OFF Transfer deliberately
		// clears the transfer facet, covered by
		// TestReCategorizingOffTransferClearsTheTransferFacet.)
		if err := svc.ReCategorize(ctx, "t-out", categorization.Transfer, nil); err != nil {
			t.Fatalf("ReCategorize: %v", err)
		}

		after := getTransferFacet(t, svc, "t-out")
		if after.Subtype != before.Subtype {
			t.Errorf("transfer subtype changed by a re-categorize: %q -> %q", before.Subtype, after.Subtype)
		}
		if !equalStringPtr(after.DestinationAccountID, before.DestinationAccountID) {
			t.Errorf("transfer destination changed by a re-categorize")
		}
		if after.Overridden {
			t.Errorf("transfer_destination_overridden was set by a re-categorize; the two facets must stay independent")
		}
		// Sanity: the categorization facet itself was overridden.
		if cat := getCategorizationFacet(t, svc, "t-out"); !cat.Overridden || cat.Classification != categorization.Transfer {
			t.Errorf("the re-categorize did not record its own override (got overridden=%v classification=%q)", cat.Overridden, cat.Classification)
		}
	})
}

// TestMarkTransferDestinationRejectsIneligibleRows proves the validation guard: a
// non-transfer row, an inflow Transfer leg (the excluded mirror), and an invalid
// subtype each return the module's ValidationError and write nothing.
func TestMarkTransferDestinationRejectsIneligibleRows(t *testing.T) {
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
					bankTxn("t-spend", "p-check", 1, 50.00, false), // a non-transfer spending row
					transferBankTxn("t-out", "p-check", 4, 250.00), // an outflow transfer, lonely (auto plain/unknown)
					transferBankTxn("t-in", "p-save", 4, -999.00),  // an inflow mirror leg (won't pair: amount differs)
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database), nil)
	if err := svc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}
	internal := providerToInternal(t, accountsSvc)
	savingsID := internal["p-save"]

	assertRejectedAndUnwritten := func(t *testing.T, id string, dest *string, subtype categorization.TransferSubtype, wantSubtype categorization.TransferSubtype) {
		t.Helper()
		err := svc.MarkTransferDestination(ctx, id, dest, subtype)
		if err == nil {
			t.Fatalf("MarkTransferDestination(%q) returned nil, want a validation error", id)
		}
		if _, ok := IsValidationError(err); !ok {
			t.Fatalf("MarkTransferDestination(%q) error = %v, want a ValidationError", id, err)
		}
		facet := getTransferFacet(t, svc, id)
		if facet.Overridden {
			t.Errorf("%q: override flag was set by a rejected mark; nothing should be written", id)
		}
		if facet.DestinationAccountID != nil {
			t.Errorf("%q: destination = %q was written by a rejected mark", id, *facet.DestinationAccountID)
		}
		if facet.Subtype != wantSubtype {
			t.Errorf("%q: subtype = %q, want %q (unchanged by a rejected mark)", id, facet.Subtype, wantSubtype)
		}
	}

	t.Run("a non-transfer row is rejected and nothing is written", func(t *testing.T) {
		assertRejectedAndUnwritten(t, "t-spend", &savingsID, categorization.SubtypeSavingsContribution, categorization.SubtypeNone)
	})

	t.Run("an inflow transfer leg is rejected and nothing is written", func(t *testing.T) {
		assertRejectedAndUnwritten(t, "t-in", &savingsID, categorization.SubtypePlain, categorization.SubtypeNone)
	})

	t.Run("an invalid subtype on a valid outflow transfer is rejected and nothing is written", func(t *testing.T) {
		// t-out is a valid outflow transfer the auto pass left plain/unknown; an
		// empty subtype is not an allowed mark, so the manual write is refused and
		// the auto value stands.
		assertRejectedAndUnwritten(t, "t-out", &savingsID, categorization.SubtypeNone, categorization.SubtypePlain)
	})
}
