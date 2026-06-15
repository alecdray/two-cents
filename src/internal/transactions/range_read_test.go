package transactions

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// rangeTxn builds a seam transaction with a chosen bank category so the sync's
// auto-categorization resolves a predictable classification/subtype for the
// range read to carry. The amount keeps the seam sign convention (outflow
// positive, inflow negative).
func rangeTxn(id, providerAccountID string, day int, amount float64, pending bool, primary string) banking.Transaction {
	return banking.Transaction{
		ID:           id,
		AccountID:    providerAccountID,
		Date:         txnDate(day),
		Amount:       banking.Money{Amount: amount, Currency: "USD"},
		Merchant:     "Merchant " + id,
		Counterparty: "RAW " + id,
		Category:     banking.Category{Primary: primary, Detailed: primary + "_OTHER"},
		Pending:      pending,
	}
}

// TestTransactionsInRangeReturnsInRangeRows seeds a spread of transactions
// across June and the adjacent month boundaries, then asserts the range read
// returns exactly the rows whose date falls in the half-open [start, end) June
// window — boundary row at start included, boundary row at end excluded — each
// carrying its signed amount, classification, transfer subtype, and pending flag.
func TestTransactionsInRangeReturnsInRangeRows(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	provider.syncByToken[token] = func(cursor string) (banking.TransactionChanges, error) {
		if cursor == "" {
			return banking.TransactionChanges{
				Added: []banking.Transaction{
					// Boundary at start (June 1, UTC midnight) — included.
					rangeTxn("t-start", "p-check", 1, 25.00, false, "FOOD_AND_DRINK"),
					// A pending spending row mid-month.
					rangeTxn("t-pending", "p-check", 10, 5.75, true, "GENERAL_MERCHANDISE"),
					// An income inflow mid-month.
					rangeTxn("t-income", "p-check", 15, -2000.00, false, "INCOME"),
					// A transfer outflow mid-month (becomes a transfer leg; with no
					// matching inflow its subtype resolves to plain).
					rangeTxn("t-transfer", "p-check", 20, 500.00, false, "TRANSFER_OUT"),
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

	// Insert two out-of-range rows directly so they exist regardless of the sync's
	// categorization: May 31 (just before the June window) and July 1 (the end
	// boundary, excluded by the half-open range).
	internal := providerToInternal(t, accountsSvc)
	insertRawTxn(t, database, "t-may", internal["p-check"], time.Date(2026, time.May, 31, 0, 0, 0, 0, time.UTC), 99)
	insertRawTxn(t, database, "t-july", internal["p-check"], time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), 99)

	start := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)

	rows, err := svc.TransactionsInRange(ctx, start, end)
	if err != nil {
		t.Fatalf("TransactionsInRange: %v", err)
	}

	t.Run("returns exactly the in-range rows (start included, end excluded)", func(t *testing.T) {
		gotIDs := map[string]ActivityRow{}
		for _, r := range rows {
			gotIDs[r.ID] = r
		}
		wantIDs := []string{"t-start", "t-pending", "t-income", "t-transfer"}
		if len(rows) != len(wantIDs) {
			t.Fatalf("got %d rows %v, want %d %v", len(rows), keys(gotIDs), len(wantIDs), wantIDs)
		}
		for _, id := range wantIDs {
			if _, ok := gotIDs[id]; !ok {
				t.Errorf("missing in-range row %q", id)
			}
		}
		if _, ok := gotIDs["t-may"]; ok {
			t.Errorf("row before the window (May 31) leaked into the range")
		}
		if _, ok := gotIDs["t-july"]; ok {
			t.Errorf("row on the end boundary (July 1) must be excluded by the half-open range")
		}
	})

	t.Run("rows are ordered by date then id", func(t *testing.T) {
		for i := 1; i < len(rows); i++ {
			if rows[i].Date.Before(rows[i-1].Date) {
				t.Errorf("rows not date-ascending at %d: %s before %s", i, rows[i].Date, rows[i-1].Date)
			}
		}
	})

	t.Run("carries the signed amount, pending flag, classification and subtype", func(t *testing.T) {
		byID := map[string]ActivityRow{}
		for _, r := range rows {
			byID[r.ID] = r
		}

		income := byID["t-income"]
		if income.Amount.Amount != -2000.00 {
			t.Errorf("income amount = %v, want -2000 (signed inflow preserved)", income.Amount.Amount)
		}
		if income.Classification != categorization.Income {
			t.Errorf("income classification = %q, want income", income.Classification)
		}

		pending := byID["t-pending"]
		if !pending.Pending {
			t.Errorf("t-pending should carry Pending=true")
		}
		if pending.Classification != categorization.Spending {
			t.Errorf("t-pending classification = %q, want spending", pending.Classification)
		}

		transfer := byID["t-transfer"]
		if transfer.Classification != categorization.Transfer {
			t.Errorf("t-transfer classification = %q, want transfer", transfer.Classification)
		}
		if transfer.TransferSubtype != categorization.SubtypePlain {
			t.Errorf("t-transfer subtype = %q, want plain (no matching inflow)", transfer.TransferSubtype)
		}

		start := byID["t-start"]
		if start.CategoryID == nil || *start.CategoryID != categorization.CategoryFoodAndDrink {
			t.Errorf("t-start category = %v, want food_and_drink", start.CategoryID)
		}
	})
}

// TestEarliestTransactionDate proves the earliest read returns the minimum
// stored transaction date, and reports (zero, false) — not an error — on an
// empty table.
func TestEarliestTransactionDate(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	const token = "tok-a"
	provider := newStub()
	provider.accountsByToken[token] = []banking.Account{cashAccount("p-check", "Checking")}
	accountsSvc := accounts.NewService(database, provider, testKey)
	registerConnection(t, accountsSvc, token, "item-a")
	svc := NewService(database, provider, accountsSvc, newCategorization(database))

	t.Run("false on an empty table (not an error)", func(t *testing.T) {
		_, ok, err := svc.EarliestTransactionDate(ctx)
		if err != nil {
			t.Fatalf("EarliestTransactionDate: %v", err)
		}
		if ok {
			t.Errorf("ok = true on an empty table, want false")
		}
	})

	internal := providerToInternal(t, accountsSvc)
	insertRawTxn(t, database, "t-late", internal["p-check"], time.Date(2026, time.June, 20, 0, 0, 0, 0, time.UTC), 10)
	insertRawTxn(t, database, "t-early", internal["p-check"], time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC), 10)
	insertRawTxn(t, database, "t-mid", internal["p-check"], time.Date(2026, time.May, 9, 0, 0, 0, 0, time.UTC), 10)

	t.Run("returns the minimum date", func(t *testing.T) {
		date, ok, err := svc.EarliestTransactionDate(ctx)
		if err != nil {
			t.Fatalf("EarliestTransactionDate: %v", err)
		}
		if !ok {
			t.Fatalf("ok = false with rows present, want true")
		}
		want := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.UTC)
		if !date.Equal(want) {
			t.Errorf("earliest = %s, want %s", date, want)
		}
	})
}

// insertRawTxn inserts a minimal transaction row directly, bypassing the sync, so
// a test can place rows at exact dates (including out-of-range and unsynced ones)
// without depending on the provider script. The categorization columns take their
// schema defaults.
func insertRawTxn(t *testing.T, database *db.DB, id, accountID string, date time.Time, amount float64) {
	t.Helper()
	_, err := database.Sql().Exec(
		`INSERT INTO transactions (id, account_id, date, amount_amount, amount_currency, merchant, counterparty, category_primary, category_detailed, status)
		 VALUES (?, ?, ?, ?, 'USD', '', '', '', '', 'posted')`,
		id, accountID, date, amount,
	)
	if err != nil {
		t.Fatalf("insert raw txn %q: %v", id, err)
	}
}

func keys(m map[string]ActivityRow) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
