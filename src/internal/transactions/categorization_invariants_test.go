package transactions

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
)

// These tests assert the emergent, whole-product properties of the assembled
// categorization feature — invariants that span both the categorization decider
// and the transactions writer and so cannot be proven by either module alone.
// They run through the real, server-wired services (wiredServices) over a
// migrated temp SQLite database, exactly as the running app composes them: the
// rule-change re-categorization seam drives the real Transactions.ApplyCategorization.

// --- An override outlives every re-sync and rule change ---

// TestOverrideOutlivesResyncsAndRuleChurn proves a manual override is sticky in
// the strong sense: once a user fixes a transaction's categorization, that choice
// survives any number of re-syncs (including ones that keep modifying the row) and
// any rule create/edit/delete whose substring matches the row's merchant. Neither
// auto-categorization nor the rule-change re-categorizer ever reverts it.
func TestOverrideOutlivesResyncsAndRuleChurn(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxnCat("t1", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", true)}, // pending
				Cursor: "c1",
			}, nil
		}
		// Every later pull keeps re-modifying the same row (pending→posted and on),
		// so each re-sync re-touches it — the override must still hold.
		return banking.TransactionChanges{
			Modified: []banking.Transaction{bankTxnCat("t1", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", false)},
			Cursor:   "c1",
		}, nil
	}

	accountsSvc, txnSvc, catSvc := wiredServices(database, provider)
	registerConnection(t, accountsSvc, token, "item-a")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// The user pins the row to Spending → Food & Drink (distinct from the auto
	// General Merchandise the bank category would assign).
	pinned := categorization.CategoryFoodAndDrink
	if err := txnSvc.ReCategorize(ctx, "t1", categorization.Spending, &pinned); err != nil {
		t.Fatalf("ReCategorize: %v", err)
	}

	assertPinned := func(t *testing.T, when string) {
		t.Helper()
		class, categoryID, overridden := readCategorization(t, database, "t1")
		if class != string(categorization.Spending) {
			t.Errorf("%s: classification = %q, want spending (the override must hold)", when, class)
		}
		if !categoryID.Valid || categoryID.String != pinned {
			t.Errorf("%s: category_id = %v, want %q (the override must hold)", when, categoryID, pinned)
		}
		if overridden != 1 {
			t.Errorf("%s: overridden = %d, want 1 (the sticky facet must persist)", when, overridden)
		}
	}

	assertPinned(t, "right after the override")

	// Several re-syncs, each re-modifying the row.
	for i := 0; i < 3; i++ {
		if err := txnSvc.SyncTransactions(ctx); err != nil {
			t.Fatalf("re-sync %d: %v", i, err)
		}
	}
	assertPinned(t, "after repeated re-syncs")

	// A rule whose substring matches the row's merchant — create, edit, delete —
	// must never revert the override, and the re-categorizer must report zero
	// changes since the only match is overridden.
	rule, count, err := catSvc.CreateRule(ctx, "Whole Foods", categorization.Income, nil)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if count != 0 {
		t.Errorf("rule create re-categorized %d rows, want 0 (the only match is overridden)", count)
	}
	assertPinned(t, "after a matching rule is created")

	travel := categorization.CategoryTravel
	if _, count, err = catSvc.EditRule(ctx, rule.ID, "Whole Foods", categorization.Spending, &travel); err != nil {
		t.Fatalf("EditRule: %v", err)
	}
	if count != 0 {
		t.Errorf("rule edit re-categorized %d rows, want 0 (the only match is overridden)", count)
	}
	assertPinned(t, "after the matching rule is edited")

	if count, err = catSvc.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
	if count != 0 {
		t.Errorf("rule delete re-categorized %d rows, want 0 (the only match is overridden)", count)
	}
	assertPinned(t, "after the matching rule is deleted")

	// And one final re-sync for good measure.
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("final re-sync: %v", err)
	}
	assertPinned(t, "after a final re-sync")
}

// --- Archiving a Category never rewrites history ---

// TestArchivingACategoryPreservesExistingAssignments proves archiving a Category
// is non-destructive to the ledger: a transaction already assigned to it keeps
// that assignment untouched (archive removes the Category only from pickers and
// future auto-assignment, never from rows that already carry it), and unarchiving
// restores the Category to the active list with the assignment still intact.
func TestArchivingACategoryPreservesExistingAssignments(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxnCat("t-spend", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", false)},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc, txnSvc, catSvc := wiredServices(database, provider)
	registerConnection(t, accountsSvc, token, "item-a")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// The row auto-categorizes to the General Merchandise built-in.
	const assigned = categorization.CategoryGeneralMerchandise
	if class, categoryID, _ := readCategorization(t, database, "t-spend"); class != string(categorization.Spending) || !categoryID.Valid || categoryID.String != assigned {
		t.Fatalf("precondition: row not assigned to %q (got %q, %v)", assigned, class, categoryID)
	}

	if _, err := catSvc.ArchiveCategory(ctx, assigned); err != nil {
		t.Fatalf("ArchiveCategory: %v", err)
	}

	t.Run("the existing assignment is untouched by the archive", func(t *testing.T) {
		class, categoryID, _ := readCategorization(t, database, "t-spend")
		if class != string(categorization.Spending) || !categoryID.Valid || categoryID.String != assigned {
			t.Errorf("archive rewrote the row: got (%q, %v), want it to keep %q", class, categoryID, assigned)
		}
	})

	t.Run("the archived Category drops out of the active list", func(t *testing.T) {
		active, err := catSvc.ListCategories(ctx, false)
		if err != nil {
			t.Fatalf("ListCategories(active): %v", err)
		}
		if categoriesContain(active, assigned) {
			t.Errorf("archived category %q still in the active list", assigned)
		}
	})

	if _, err := catSvc.UnarchiveCategory(ctx, assigned); err != nil {
		t.Fatalf("UnarchiveCategory: %v", err)
	}

	t.Run("unarchive restores the Category to the active list, assignment still intact", func(t *testing.T) {
		active, err := catSvc.ListCategories(ctx, false)
		if err != nil {
			t.Fatalf("ListCategories(active): %v", err)
		}
		if !categoriesContain(active, assigned) {
			t.Errorf("unarchive did not restore %q to the active list", assigned)
		}
		class, categoryID, _ := readCategorization(t, database, "t-spend")
		if class != string(categorization.Spending) || !categoryID.Valid || categoryID.String != assigned {
			t.Errorf("assignment changed across archive/unarchive: got (%q, %v), want %q", class, categoryID, assigned)
		}
	})
}

// categoriesContain reports whether the given category id is present in the list.
func categoriesContain(categories []categorization.Category, id string) bool {
	for _, c := range categories {
		if c.ID == id {
			return true
		}
	}
	return false
}

// --- A Category accompanies Spending, and only Spending ---

// TestCategoryAccompaniesSpendingOnEveryPath proves the Classification/Category
// coupling holds no matter which path set the row: a Category is only ever
// present on a Spending row, and Income / Transfer / needs-review never carry one.
// It exercises all three paths that write the facet — the auto path (sync), the
// rule path, and the manual ReCategorize path. (The one designed asymmetry —
// uncategorized Spending from the outflow fallback, Spending with no Category — is
// covered separately and is consistent with this coupling, which forbids only a
// Category on a non-Spending row, never a Category-less Spending row.)
func TestCategoryAccompaniesSpendingOnEveryPath(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					bankTxnCat("auto-spend", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", false),
					bankTxnCat("auto-income", "p-check", "Acme Payroll", 2, -2400, "INCOME", false),
					bankTxnCat("auto-transfer", "p-check", "Savings Move", 3, 500, "TRANSFER_OUT", false),
					bankTxnCat("auto-review", "p-check", "Mystery Deposit", 4, -150, "", false),
					// Two rows a rule will later claim.
					bankTxnCat("rule-spend", "p-check", "Corner Cafe", 5, 12.50, "", false),
					bankTxnCat("rule-income", "p-check", "Side Hustle Co", 6, -300, "", false),
					// A row the user will hand-categorize through the picker.
					bankTxnCat("manual", "p-check", "Anything", 7, 60, "", false),
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc, txnSvc, catSvc := wiredServices(database, provider)
	registerConnection(t, accountsSvc, token, "item-a")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// assertCoupling enforces the invariant for one row: a Category is present iff
	// the row is Spending — i.e. a Category never rides on a non-Spending row, and
	// (for the rows we steer to a categorized outcome) a Spending row carries one.
	assertCoupling := func(t *testing.T, id string, wantHasCategory bool) {
		t.Helper()
		class, categoryID, _ := readCategorization(t, database, id)
		if categoryID.Valid && class != string(categorization.Spending) {
			t.Errorf("%s: carries category %q on a %q row; a Category may ride only on Spending", id, categoryID.String, class)
		}
		switch categorization.Classification(class) {
		case categorization.Income, categorization.Transfer, categorization.NeedsReview:
			if categoryID.Valid {
				t.Errorf("%s: %q row carries category %v, want none", id, class, categoryID)
			}
		}
		if wantHasCategory && !categoryID.Valid {
			t.Errorf("%s: expected a Category on this Spending row, got none", id)
		}
		if wantHasCategory && class != string(categorization.Spending) {
			t.Errorf("%s: expected Spending, got %q", id, class)
		}
	}

	t.Run("auto path", func(t *testing.T) {
		assertCoupling(t, "auto-spend", true)
		assertCoupling(t, "auto-income", false)
		assertCoupling(t, "auto-transfer", false)
		assertCoupling(t, "auto-review", false)
	})

	t.Run("rule path", func(t *testing.T) {
		food := categorization.CategoryFoodAndDrink
		if _, _, err := catSvc.CreateRule(ctx, "Corner Cafe", categorization.Spending, &food); err != nil {
			t.Fatalf("CreateRule(spending): %v", err)
		}
		if _, _, err := catSvc.CreateRule(ctx, "Side Hustle", categorization.Income, nil); err != nil {
			t.Fatalf("CreateRule(income): %v", err)
		}
		assertCoupling(t, "rule-spend", true)
		assertCoupling(t, "rule-income", false)
	})

	t.Run("manual ReCategorize path", func(t *testing.T) {
		travel := categorization.CategoryTravel
		if err := txnSvc.ReCategorize(ctx, "manual", categorization.Spending, &travel); err != nil {
			t.Fatalf("ReCategorize spending: %v", err)
		}
		assertCoupling(t, "manual", true)

		if err := txnSvc.ReCategorize(ctx, "manual", categorization.Income, nil); err != nil {
			t.Fatalf("ReCategorize income: %v", err)
		}
		assertCoupling(t, "manual", false)

		if err := txnSvc.ReCategorize(ctx, "manual", categorization.Transfer, nil); err != nil {
			t.Fatalf("ReCategorize transfer: %v", err)
		}
		assertCoupling(t, "manual", false)

		if err := txnSvc.ReCategorize(ctx, "manual", categorization.NeedsReview, nil); err != nil {
			t.Fatalf("ReCategorize needs-review: %v", err)
		}
		assertCoupling(t, "manual", false)
	})
}

// --- A refund is negative Spending, never Income ---

// TestSpendingRefundStaysNegativeSpending proves a refund — an inflow whose bank
// category maps to a spending Category — is recorded as Spending with its Category
// and a negative amount, never re-routed to Income by the inflow sign.
func TestSpendingRefundStaysNegativeSpending(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				// A General Merchandise inflow: a returned purchase / refund.
				Added:  []banking.Transaction{bankTxnCat("t-refund", "p-check", "Whole Foods", 1, -25.00, "GENERAL_MERCHANDISE", false)},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc, txnSvc, _ := wiredServices(database, provider)
	registerConnection(t, accountsSvc, token, "item-a")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	class, categoryID, _ := readCategorization(t, database, "t-refund")
	if class != string(categorization.Spending) {
		t.Errorf("classification = %q, want spending (a refund is negative spending, never income)", class)
	}
	if !categoryID.Valid || categoryID.String != categorization.CategoryGeneralMerchandise {
		t.Errorf("category_id = %v, want %q (the spending category still applies)", categoryID, categorization.CategoryGeneralMerchandise)
	}

	var amount float64
	if err := database.Sql().QueryRow("SELECT amount_amount FROM transactions WHERE id = ?", "t-refund").Scan(&amount); err != nil {
		t.Fatalf("read amount: %v", err)
	}
	if amount >= 0 {
		t.Errorf("amount = %v, want a negative value (the refund's inflow sign is preserved)", amount)
	}
}

// --- Re-syncing unchanged data changes nothing ---

// TestUnchangedResyncNeitherDuplicatesNorRecategorizes proves the assembled sync
// is idempotent on unchanged provider data: re-running it over the same delta
// creates no duplicate rows and leaves every already-correct row's categorization
// exactly as it was — across the full ladder (spending, transfer, income,
// needs-review).
func TestUnchangedResyncNeitherDuplicatesNorRecategorizes(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					bankTxnCat("t-spend", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", false),
					bankTxnCat("t-transfer", "p-check", "Savings Move", 2, 500, "TRANSFER_OUT", false),
					bankTxnCat("t-income", "p-check", "Acme Payroll", 3, -2400, "INCOME", false),
					bankTxnCat("t-review", "p-check", "Mystery Deposit", 4, -150, "", false),
				},
				Cursor: "c1",
			}, nil
		}
		// Unchanged provider data on every resume.
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc, txnSvc, _ := wiredServices(database, provider)
	registerConnection(t, accountsSvc, token, "item-a")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	ids := []string{"t-spend", "t-transfer", "t-income", "t-review"}
	type facet struct {
		class string
		cat   string
	}
	before := map[string]facet{}
	for _, id := range ids {
		class, categoryID, _ := readCategorization(t, database, id)
		before[id] = facet{class: class, cat: categoryID.String}
	}

	// Two further syncs over the unchanged data.
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("third sync: %v", err)
	}

	if got := countTransactions(t, database); got != len(ids) {
		t.Errorf("row count after re-syncs = %d, want %d (no duplicates)", got, len(ids))
	}
	for _, id := range ids {
		class, categoryID, _ := readCategorization(t, database, id)
		if got := (facet{class: class, cat: categoryID.String}); got != before[id] {
			t.Errorf("%s categorization drifted on re-sync: before %+v, after %+v", id, before[id], got)
		}
	}
}
