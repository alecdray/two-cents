package adapters_test

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/fakebank"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// TestSyncReCategorizesUncategorizedStragglers is the regression test for the
// deployed-app bug where spending transactions stayed stuck on "Choose a category"
// and only a full DB wipe (re-backfill) fixed them. Root cause: a sync only
// categorized the rows in the current cursor delta, so any row left at
// classification='' (synced before categorization ran, or after a categorize error
// that still advanced the cursor) was never revisited — incremental sync won't
// re-deliver it.
//
// The fix makes categorization self-healing: every sync sweeps the non-overridden
// uncategorized rows and resolves them, mirroring the transfer-pairing pass. This
// test reproduces a straggler (a row forced back to classification='' as if it had
// been synced uncategorized) and asserts the next sync — which carries an EMPTY
// provider delta — re-categorizes it without any re-backfill.
func TestSyncReCategorizesUncategorizedStragglers(t *testing.T) {
	database := newTestDB(t)
	accountsSvc, txnSvc, _ := newServices(t, database, fakebank.NewService())
	registerConnection(t, accountsSvc)
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("initial SyncTransactions: %v", err)
	}

	// The Whole Foods row auto-categorizes to Spending + General Merchandise; grab
	// its id and confirm the healthy baseline.
	id := transactionIDByMerchant(t, txnSvc, "Whole Foods")
	baseline, err := txnSvc.RecentTransaction(testCtx(), id)
	if err != nil {
		t.Fatalf("read baseline row: %v", err)
	}
	if baseline.Classification != categorization.Spending || baseline.CategoryID == nil {
		t.Fatalf("baseline not categorized: classification=%q hasCategory=%v", baseline.Classification, baseline.CategoryID != nil)
	}

	// Force it back to the synced-but-uncategorized state a straggler is in
	// (classification='', no Category, not overridden). The bank-sync upsert never
	// writes these columns, so '' is the genuine "synced but never categorized" shape.
	repo := transactions.NewRepo(database.Queries())
	if err := repo.SetCategorization(testCtx(), id, "", nil); err != nil {
		t.Fatalf("reset row to uncategorized: %v", err)
	}

	// A later sync: the fake provider reports an empty delta (idempotent), so the
	// straggler is NOT in the pull. The self-healing sweep must still re-categorize it.
	if err := txnSvc.SyncTransactions(testCtx()); err != nil {
		t.Fatalf("healing SyncTransactions: %v", err)
	}

	healed, err := txnSvc.RecentTransaction(testCtx(), id)
	if err != nil {
		t.Fatalf("read healed row: %v", err)
	}
	if healed.Classification != categorization.Spending {
		t.Errorf("straggler not re-categorized: classification=%q, want %q", healed.Classification, categorization.Spending)
	}
	if healed.CategoryID == nil {
		t.Errorf("straggler re-categorized but its spending Category was not restored")
	}
}

// transactionIDByMerchant finds the stored transaction id for a merchant in the
// recent-activity read, failing the test if absent.
func transactionIDByMerchant(t *testing.T, txnSvc *transactions.Service, merchant string) string {
	t.Helper()
	rows, err := txnSvc.RecentTransactions(testCtx(), 100)
	if err != nil {
		t.Fatalf("RecentTransactions: %v", err)
	}
	for _, r := range rows {
		if r.Merchant == merchant {
			return r.ID
		}
	}
	t.Fatalf("no transaction found for merchant %q", merchant)
	return ""
}
