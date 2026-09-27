package schedule

import (
	"testing"
	"time"
)

// TestCreateItemFromTransaction exercises the one path where a declaration and
// its first evidence arrive together: an item seeded straight from a
// transaction, matched to it in the same operation ([ADR-0031]).
func TestCreateItemFromTransaction(t *testing.T) {
	now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

	t.Run("a dated monthly item is seeded from the transaction and matched to it", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-rent", sept(1), 2400, "GREYSTONE PROPERTY"),
		}}
		svc, ctx := resolvingService(t, ledger, now)

		item, err := svc.CreateItemFromTransaction(ctx, "txn-rent", Item{
			Name:       "Rent",
			Cadence:    CadenceMonthly,
			DayOfMonth: 1,
		})
		if err != nil {
			t.Fatalf("CreateItemFromTransaction: %v", err)
		}
		if item.ID == "" {
			t.Fatal("no id was assigned")
		}
		if item.Direction != DirectionOut {
			t.Errorf("Direction = %s, want out — seeded from a positive (outflow) transaction", item.Direction)
		}
		if item.Amount != nil {
			t.Errorf("Amount = %v, want nil — left for the system to compute from this same match", *item.Amount)
		}
		if !item.Active {
			t.Error("a bootstrapped item should be active")
		}

		// The match was recorded in the same operation, and is a completely
		// ordinary manual match — indistinguishable in shape from one MatchOccurrence
		// would have written by hand.
		matches, err := svc.Matches(ctx, sept(1), sept(1))
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("matches = %+v, want exactly one, recorded in the same operation", matches)
		}
		if matches[0].ItemID != item.ID || matches[0].TransactionID != "txn-rent" {
			t.Errorf("match = %+v, want it to point at the new item and the seeding transaction", matches[0])
		}
		if matches[0].Source != MatchManual {
			t.Errorf("Source = %s, want an ordinary manual match", matches[0].Source)
		}
		if !matches[0].Occurrence.Equal(sept(1)) {
			t.Errorf("Occurrence = %s, want 1 September — the occurrence nearest the transaction's own date", matches[0].Occurrence)
		}

		// Reading the schedule back resolves the unset amount live from this
		// same match, exactly as any other undeclared item would.
		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(items) != 1 || items[0].Amount == nil || *items[0].Amount != 2400 {
			t.Errorf("List = %+v, want the amount to read back as 2400, computed from the seeding match", items)
		}
	})

	t.Run("an inflow seeds an income item, on its biweekly anchor occurrence", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-paycheck", sept(4), -3100, "EMPLOYER INC"),
		}}
		svc, ctx := resolvingService(t, ledger, now)

		item, err := svc.CreateItemFromTransaction(ctx, "txn-paycheck", Item{
			Name:       "Paycheck",
			Cadence:    CadenceBiweekly,
			AnchorDate: sept(4),
		})
		if err != nil {
			t.Fatalf("CreateItemFromTransaction: %v", err)
		}
		if item.Direction != DirectionIn {
			t.Errorf("Direction = %s, want in — seeded from a negative (inflow) transaction", item.Direction)
		}

		matches, err := svc.Matches(ctx, sept(4), sept(4))
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		if len(matches) != 1 || matches[0].TransactionID != "txn-paycheck" {
			t.Errorf("matches = %+v, want the anchor occurrence matched to the seeding transaction", matches)
		}
	})

	t.Run("a dateless monthly item is matched against the calendar month containing the transaction", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-water", sept(23), 90, "CITY WATER"),
		}}
		svc, ctx := resolvingService(t, ledger, now)

		item, err := svc.CreateItemFromTransaction(ctx, "txn-water", Item{
			Name:    "Water",
			Cadence: CadenceMonthly,
			// DayOfMonth left zero: dateless, no fixed real-world date.
		})
		if err != nil {
			t.Fatalf("CreateItemFromTransaction: %v", err)
		}
		if item.DayOfMonth != 0 {
			t.Errorf("DayOfMonth = %d, want 0 (dateless)", item.DayOfMonth)
		}

		matches, err := svc.Matches(ctx, sept(1), sept(1))
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("matches = %+v, want the match keyed to the 1st of September, standing in for the whole month", matches)
		}
		if !matches[0].Occurrence.Equal(sept(1)) {
			t.Errorf("Occurrence = %s, want 1 September — the calendar month containing the 23rd, not the 23rd itself", matches[0].Occurrence)
		}
	})

	t.Run("an unknown transaction id fails and creates nothing", func(t *testing.T) {
		ledger := &fakeLedger{}
		svc, ctx := resolvingService(t, ledger, now)

		_, err := svc.CreateItemFromTransaction(ctx, "txn-missing", Item{
			Name:       "Rent",
			Cadence:    CadenceMonthly,
			DayOfMonth: 1,
		})
		if err == nil {
			t.Fatal("CreateItemFromTransaction accepted an unknown transaction id")
		}
		if _, ok := IsValidationError(err); !ok {
			t.Errorf("error = %v, want a ValidationError the adapter can render inline", err)
		}

		items, _ := svc.List(ctx)
		if len(items) != 0 {
			t.Errorf("an item was stored despite the missing transaction: %+v", items)
		}
	})

	t.Run("an invalid declaration fails and creates nothing, even though the transaction exists", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-rent", sept(1), 2400, "GREYSTONE PROPERTY"),
		}}
		svc, ctx := resolvingService(t, ledger, now)

		_, err := svc.CreateItemFromTransaction(ctx, "txn-rent", Item{
			Name:    "  ",
			Cadence: CadenceMonthly,
		})
		if err == nil {
			t.Fatal("CreateItemFromTransaction accepted a nameless declaration")
		}
		if _, ok := IsValidationError(err); !ok {
			t.Errorf("error = %v, want a ValidationError", err)
		}

		items, _ := svc.List(ctx)
		if len(items) != 0 {
			t.Errorf("an item was stored despite the invalid declaration: %+v", items)
		}
		matches, _ := svc.Matches(ctx, sept(1), sept(1))
		if len(matches) != 0 {
			t.Errorf("a match was recorded despite the invalid declaration: %+v", matches)
		}
	})

	t.Run("a transaction already matched to a different item is refused, and nothing new is created or matched", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-shared", sept(1), 2400, "GREYSTONE PROPERTY"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		existing, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, existing.ID, sept(1), "txn-shared"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		_, err = svc.CreateItemFromTransaction(ctx, "txn-shared", Item{
			Name:       "Rent Again",
			Cadence:    CadenceMonthly,
			DayOfMonth: 1,
		})
		if err == nil {
			t.Fatal("CreateItemFromTransaction accepted a transaction already matched elsewhere")
		}

		items, _ := svc.List(ctx)
		if len(items) != 1 {
			t.Fatalf("items = %+v, want only the original item — nothing new stored", items)
		}
		matches, err := svc.Matches(ctx, sept(1), sept(1))
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		if len(matches) != 1 || matches[0].ItemID != existing.ID {
			t.Errorf("matches = %+v, want only the original match, untouched", matches)
		}
	})
}
