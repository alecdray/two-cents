package schedule

import (
	"testing"
	"time"
)

// TestMatchedItem exercises MatchedItem — the read transactions consumes to
// answer which item, if any, it is matched to. Every path that can leave a
// stored match (an ordinary manual match, automatic resolution, and a
// bootstrap creation) must be found the same way, and a transaction with no
// match must answer cleanly rather than erroring or guessing ([ADR-0031]).
func TestMatchedItem(t *testing.T) {
	t.Run("a transaction settled by an ordinary manual match reports that item", func(t *testing.T) {
		svc, ctx := newService(t)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-rent"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		got, ok, err := svc.MatchedItem(ctx, "txn-rent")
		if err != nil {
			t.Fatalf("MatchedItem: %v", err)
		}
		if !ok {
			t.Fatal("MatchedItem reported no match for a transaction that was manually matched")
		}
		if got.ID != item.ID {
			t.Errorf("MatchedItem = %+v, want item %s", got, item.ID)
		}
	})

	t.Run("a transaction settled by automatic resolution reports that item", func(t *testing.T) {
		now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-rent", sept(1), 2400, "GREYSTONE PROPERTY"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("ResolveOccurrenceMatches: %v", err)
		}

		got, ok, err := svc.MatchedItem(ctx, "txn-rent")
		if err != nil {
			t.Fatalf("MatchedItem: %v", err)
		}
		if !ok {
			t.Fatal("MatchedItem reported no match for a transaction automatic resolution settled")
		}
		if got.ID != item.ID {
			t.Errorf("MatchedItem = %+v, want item %s", got, item.ID)
		}
	})

	t.Run("the transaction that bootstrapped an item reports that item", func(t *testing.T) {
		now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)
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

		got, ok, err := svc.MatchedItem(ctx, "txn-rent")
		if err != nil {
			t.Fatalf("MatchedItem: %v", err)
		}
		if !ok {
			t.Fatal("MatchedItem reported no match for the transaction that bootstrapped an item")
		}
		if got.ID != item.ID {
			t.Errorf("MatchedItem = %+v, want the bootstrapped item %s", got, item.ID)
		}
	})

	t.Run("a transaction matched to nothing reports no match, and no error", func(t *testing.T) {
		svc, ctx := newService(t)
		if _, err := svc.Create(ctx, rent()); err != nil {
			t.Fatalf("Create: %v", err)
		}

		got, ok, err := svc.MatchedItem(ctx, "txn-unmatched")
		if err != nil {
			t.Fatalf("MatchedItem returned an error for an ordinary unmatched transaction: %v", err)
		}
		if ok {
			t.Errorf("MatchedItem = %+v, ok = true, want no match", got)
		}
		if got != (Item{}) {
			t.Errorf("MatchedItem returned a non-zero item %+v for no match", got)
		}
	})

	t.Run("a transaction id nothing declared at all reports no match, and no error", func(t *testing.T) {
		svc, ctx := newService(t)

		_, ok, err := svc.MatchedItem(ctx, "txn-never-seen")
		if err != nil {
			t.Fatalf("MatchedItem: %v", err)
		}
		if ok {
			t.Error("MatchedItem reported a match against an empty schedule")
		}
	})

	t.Run("the unique index a transaction can settle at most one occurrence with is what keeps the answer unambiguous", func(t *testing.T) {
		svc, ctx := newService(t)
		first, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		second, err := svc.Create(ctx, paycheck())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, first.ID, sept(1), "txn-shared"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		// A second item claiming the same transaction is refused by storage — the
		// unique index on the match's transaction id, not this lookup's own logic,
		// is what guarantees a transaction never settles two obligations.
		err = svc.MatchOccurrence(ctx, second.ID, sept(4), "txn-shared")
		if err == nil {
			t.Fatal("a second item's claim on an already-settled transaction was accepted; want the unique index to refuse it")
		}

		got, ok, err := svc.MatchedItem(ctx, "txn-shared")
		if err != nil {
			t.Fatalf("MatchedItem: %v", err)
		}
		if !ok || got.ID != first.ID {
			t.Errorf("MatchedItem = %+v, ok = %v, want exactly the original item %s and nothing else", got, ok, first.ID)
		}
	})
}
