package transactions

import (
	"database/sql"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// bankTxnCat builds a seam transaction with an explicit bank category primary and
// merchant, so a test can drive a specific rung of the categorization ladder. The
// amount keeps the seam's sign convention (outflow positive, inflow negative).
func bankTxnCat(id, providerAccountID, merchant string, day int, amount float64, primary string, pending bool) banking.Transaction {
	return banking.Transaction{
		ID:           id,
		AccountID:    providerAccountID,
		Date:         txnDate(day),
		Amount:       banking.Money{Amount: amount, Currency: "USD"},
		Merchant:     merchant,
		Counterparty: "RAW " + id,
		Category:     banking.Category{Primary: primary},
		Pending:      pending,
	}
}

// readCategorization reads a transaction's stored categorization facet directly,
// so a test can assert on what the sync / re-categorize path persisted.
func readCategorization(t *testing.T, database *db.DB, id string) (classification string, categoryID sql.NullString, overridden int) {
	t.Helper()
	row := database.Sql().QueryRow("SELECT classification, category_id, categorization_overridden FROM transactions WHERE id = ?", id)
	if err := row.Scan(&classification, &categoryID, &overridden); err != nil {
		t.Fatalf("read categorization for %q: %v", id, err)
	}
	return classification, categoryID, overridden
}

// wiredServices builds the accounts, transactions, and categorization services
// over a shared database, with the re-categorization seam wired to
// Transactions.ApplyCategorization — the same late-bound closure the composition
// root uses, so a Rule mutation drives the real re-categorizer.
func wiredServices(database *db.DB, provider banking.BankProvider) (*accounts.Service, *Service, *categorization.Service) {
	accountsSvc := accounts.NewService(database, provider, testKey)
	var txnSvc *Service
	catSvc := categorization.NewService(database, func(ctx contextx.ContextX, substrings []string) (int, error) {
		return txnSvc.ApplyCategorization(ctx, substrings)
	})
	txnSvc = NewService(database, provider, accountsSvc, catSvc, nil)
	return accountsSvc, txnSvc, catSvc
}

// --- Auto-categorize on sync ---

// TestSyncAutoCategorizesAcrossTheLadder proves a sync resolves each new row's
// classification by precedence: a clearly-spending bank category maps to Spending
// plus the mapped built-in Category; a transfer-signal primary maps to Transfer;
// and an unclassifiable inflow falls through to needs-review.
func TestSyncAutoCategorizesAcrossTheLadder(t *testing.T) {
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
					bankTxnCat("t-review", "p-check", "Mystery Deposit", 3, -150, "", false),
				},
				Cursor: "c1",
			}, nil
		}
		return banking.TransactionChanges{Cursor: cursor}, nil
	}

	accountsSvc, txnSvc, _ := wiredServices(database, provider)
	registerConnection(t, accountsSvc, token, "item-a")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("SyncTransactions: %v", err)
	}

	t.Run("a clearly-spending bank category becomes Spending with the mapped Category", func(t *testing.T) {
		class, categoryID, overridden := readCategorization(t, database, "t-spend")
		if class != string(categorization.Spending) {
			t.Errorf("classification = %q, want spending", class)
		}
		if !categoryID.Valid || categoryID.String != categorization.CategoryGeneralMerchandise {
			t.Errorf("category_id = %v, want %q", categoryID, categorization.CategoryGeneralMerchandise)
		}
		if overridden != 0 {
			t.Errorf("overridden = %d, want 0 (auto-categorize never sets the override)", overridden)
		}
	})

	t.Run("a transfer-signal primary becomes Transfer with no Category", func(t *testing.T) {
		class, categoryID, _ := readCategorization(t, database, "t-transfer")
		if class != string(categorization.Transfer) {
			t.Errorf("classification = %q, want transfer", class)
		}
		if categoryID.Valid {
			t.Errorf("category_id = %v, want NULL for a transfer", categoryID)
		}
	})

	t.Run("an unclassifiable inflow becomes needs-review", func(t *testing.T) {
		class, categoryID, _ := readCategorization(t, database, "t-review")
		if class != string(categorization.NeedsReview) {
			t.Errorf("classification = %q, want needs_review", class)
		}
		if categoryID.Valid {
			t.Errorf("category_id = %v, want NULL for needs-review", categoryID)
		}
	})
}

// TestSyncIsIdempotentForCategorization proves re-syncing unchanged data neither
// duplicates rows nor drifts their categorization.
func TestSyncIsIdempotentForCategorization(t *testing.T) {
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

	accountsSvc, txnSvc, _ := wiredServices(database, provider)
	registerConnection(t, accountsSvc, token, "item-a")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	classBefore, catBefore, _ := readCategorization(t, database, "t-spend")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if got := countTransactions(t, database); got != 1 {
		t.Errorf("row count after re-sync = %d, want 1 (no duplicates)", got)
	}
	classAfter, catAfter, _ := readCategorization(t, database, "t-spend")
	if classAfter != classBefore || catAfter.String != catBefore.String {
		t.Errorf("categorization drifted on re-sync: before (%q,%v) after (%q,%v)", classBefore, catBefore, classAfter, catAfter)
	}
}

// --- Manual re-categorize (sticky across re-sync) ---

// TestReCategorizeIsStickyAcrossModifyingSync proves a manual override survives a
// later sync that modifies the row (e.g. pending→posted): the bank fields update,
// but the overridden categorization facet is preserved and never re-derived.
func TestReCategorizeIsStickyAcrossModifyingSync(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		switch cursor {
		case "":
			return banking.TransactionChanges{
				Added:  []banking.Transaction{bankTxnCat("t1", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", true)}, // pending
				Cursor: "c1",
			}, nil
		case "c1":
			return banking.TransactionChanges{
				Modified: []banking.Transaction{bankTxnCat("t1", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", false)}, // posted
				Cursor:   "c2",
			}, nil
		default:
			return banking.TransactionChanges{Cursor: cursor}, nil
		}
	}

	accountsSvc, txnSvc, _ := wiredServices(database, provider)
	registerConnection(t, accountsSvc, token, "item-a")
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// The user overrides the auto Spending → Transfer (which clears the Category).
	if err := txnSvc.ReCategorize(ctx, "t1", categorization.Transfer, nil); err != nil {
		t.Fatalf("ReCategorize: %v", err)
	}

	// A later sync moves the row pending→posted.
	if err := txnSvc.SyncTransactions(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	class, categoryID, overridden := readCategorization(t, database, "t1")
	if class != string(categorization.Transfer) {
		t.Errorf("classification = %q, want transfer to survive the modifying sync", class)
	}
	if categoryID.Valid {
		t.Errorf("category_id = %v, want NULL (transfer clears the Category)", categoryID)
	}
	if overridden != 1 {
		t.Errorf("overridden = %d, want 1 (the sticky facet must survive)", overridden)
	}
	var status string
	if err := database.Sql().QueryRow("SELECT status FROM transactions WHERE id = ?", "t1").Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != string(StatusPosted) {
		t.Errorf("status = %q, want posted (bank fields still update under a sticky override)", status)
	}
}

// TestReCategorizeEnforcesCoupling proves the Classification/Category coupling: a
// Spending pick sets a Category and marks the row overridden; an Income pick
// clears the Category; and a Spending pick with no Category is a recoverable
// validation error that changes nothing.
func TestReCategorizeEnforcesCoupling(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					bankTxnCat("t-a", "p-check", "Whole Foods", 1, 84.32, "GENERAL_MERCHANDISE", false),
					bankTxnCat("t-b", "p-check", "Acme Payroll", 2, -2400, "INCOME", false),
				},
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

	t.Run("a Spending pick sets the Category and marks the row overridden", func(t *testing.T) {
		want := categorization.CategoryFoodAndDrink
		if err := txnSvc.ReCategorize(ctx, "t-a", categorization.Spending, &want); err != nil {
			t.Fatalf("ReCategorize spending: %v", err)
		}
		class, categoryID, overridden := readCategorization(t, database, "t-a")
		if class != string(categorization.Spending) || !categoryID.Valid || categoryID.String != want {
			t.Errorf("got (%q,%v), want spending + %q", class, categoryID, want)
		}
		if overridden != 1 {
			t.Errorf("overridden = %d, want 1", overridden)
		}
	})

	t.Run("an Income pick clears the Category", func(t *testing.T) {
		// Even if a stray Category id is supplied, an income outcome carries none.
		stray := categorization.CategoryFoodAndDrink
		if err := txnSvc.ReCategorize(ctx, "t-a", categorization.Income, &stray); err != nil {
			t.Fatalf("ReCategorize income: %v", err)
		}
		class, categoryID, _ := readCategorization(t, database, "t-a")
		if class != string(categorization.Income) {
			t.Errorf("classification = %q, want income", class)
		}
		if categoryID.Valid {
			t.Errorf("category_id = %v, want NULL after an income pick", categoryID)
		}
	})

	t.Run("a Spending pick with no Category is a recoverable validation error that changes nothing", func(t *testing.T) {
		before, beforeCat, beforeOverridden := readCategorization(t, database, "t-b")
		err := txnSvc.ReCategorize(ctx, "t-b", categorization.Spending, nil)
		if err == nil {
			t.Fatalf("ReCategorize spending with no category returned nil, want a validation error")
		}
		if _, ok := IsValidationError(err); !ok {
			t.Errorf("error %v is not a ValidationError", err)
		}
		after, afterCat, afterOverridden := readCategorization(t, database, "t-b")
		if after != before || afterCat.String != beforeCat.String || afterOverridden != beforeOverridden {
			t.Errorf("invalid submit changed the row: before (%q,%v,%d) after (%q,%v,%d)", before, beforeCat, beforeOverridden, after, afterCat, afterOverridden)
		}
	})
}

// --- Reapply on rule change ---

// TestApplyCategorizationReResolvesMatchingNonOverridden proves the rule-change
// re-categorizer re-resolves matching non-overridden rows, skips overridden rows,
// and returns the count actually changed — and that a Rule create drives it
// through the server-wired seam with the right count.
func TestApplyCategorizationReResolvesMatchingNonOverridden(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					// Two rows whose merchant a rule can match; one will be overridden.
					bankTxnCat("t-match", "p-check", "Side Hustle Co", 1, -150, "", false),
					bankTxnCat("t-override", "p-check", "Side Hustle Co", 2, -200, "", false),
					// A row the rule's substring does not match.
					bankTxnCat("t-other", "p-check", "Whole Foods", 3, 84.32, "GENERAL_MERCHANDISE", false),
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

	// Both Side Hustle rows start needs-review (inflow, no usable bank category).
	if class, _, _ := readCategorization(t, database, "t-match"); class != string(categorization.NeedsReview) {
		t.Fatalf("t-match starts %q, want needs_review", class)
	}
	// Override one of them to Income, so the rule must skip it.
	if err := txnSvc.ReCategorize(ctx, "t-override", categorization.Income, nil); err != nil {
		t.Fatalf("ReCategorize override: %v", err)
	}

	// Creating a Rule fires the server-wired seam → ApplyCategorization.
	_, count, err := catSvc.CreateRule(ctx, "Side Hustle", categorization.Income, nil)
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	t.Run("the rule re-categorizes the matching non-overridden row and reports a count of one", func(t *testing.T) {
		if count != 1 {
			t.Errorf("re-categorized count = %d, want 1 (only the non-overridden match changed)", count)
		}
		if class, _, overridden := readCategorization(t, database, "t-match"); class != string(categorization.Income) || overridden != 0 {
			t.Errorf("t-match = (%q, overridden=%d), want income, overridden 0", class, overridden)
		}
	})

	t.Run("the overridden match is left untouched", func(t *testing.T) {
		if class, _, overridden := readCategorization(t, database, "t-override"); class != string(categorization.Income) || overridden != 1 {
			t.Errorf("t-override = (%q, overridden=%d), want income, overridden 1 (its sticky facet)", class, overridden)
		}
	})

	t.Run("a non-matching row is not re-categorized", func(t *testing.T) {
		class, categoryID, _ := readCategorization(t, database, "t-other")
		if class != string(categorization.Spending) || !categoryID.Valid || categoryID.String != categorization.CategoryGeneralMerchandise {
			t.Errorf("t-other = (%q,%v), want it to keep its bank-category Spending", class, categoryID)
		}
	})

	t.Run("invoking apply directly returns the count changed and skips overridden rows", func(t *testing.T) {
		// With the rule already applied, a second apply over the same substring
		// changes nothing (the non-overridden match already resolves to income).
		n, err := txnSvc.ApplyCategorization(ctx, []string{"Side Hustle"})
		if err != nil {
			t.Fatalf("ApplyCategorization: %v", err)
		}
		if n != 0 {
			t.Errorf("second apply changed %d rows, want 0 (already settled, overridden skipped)", n)
		}
	})
}
