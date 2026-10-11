package schedule

import "testing"

// TestMatchedItemNames exercises the batched counterpart to MatchedItem — the
// read the transactions list's per-row match badge takes over a whole page of
// rows in one call rather than one call per row ([ADR-0031]).
func TestMatchedItemNames(t *testing.T) {
	t.Run("a matched transaction's id maps to its item's name", func(t *testing.T) {
		svc, ctx := newService(t)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-rent"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		got, err := svc.MatchedItemNames(ctx, []string{"txn-rent"})
		if err != nil {
			t.Fatalf("MatchedItemNames: %v", err)
		}
		if got["txn-rent"] != "Rent" {
			t.Errorf("MatchedItemNames[txn-rent] = %q, want %q", got["txn-rent"], "Rent")
		}
	})

	t.Run("an unmatched transaction id in the batch is simply absent from the result", func(t *testing.T) {
		svc, ctx := newService(t)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-rent"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		got, err := svc.MatchedItemNames(ctx, []string{"txn-rent", "txn-unmatched"})
		if err != nil {
			t.Fatalf("MatchedItemNames: %v", err)
		}
		if _, ok := got["txn-unmatched"]; ok {
			t.Errorf("MatchedItemNames reported a match for txn-unmatched: %+v", got)
		}
		if len(got) != 1 {
			t.Errorf("MatchedItemNames = %+v, want exactly one entry", got)
		}
	})

	t.Run("a transaction id outside the requested batch is never reported, even if it is matched", func(t *testing.T) {
		svc, ctx := newService(t)
		rentItem, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create(rent): %v", err)
		}
		if err := svc.MatchOccurrence(ctx, rentItem.ID, sept(1), "txn-rent"); err != nil {
			t.Fatalf("MatchOccurrence(rent): %v", err)
		}
		paycheckItem, err := svc.Create(ctx, paycheck())
		if err != nil {
			t.Fatalf("Create(paycheck): %v", err)
		}
		if err := svc.MatchOccurrence(ctx, paycheckItem.ID, sept(4), "txn-paycheck"); err != nil {
			t.Fatalf("MatchOccurrence(paycheck): %v", err)
		}

		// Only txn-rent is asked about — txn-paycheck's match must not leak in,
		// the way it would if this read scanned every settled match regardless of
		// what the caller asked for.
		got, err := svc.MatchedItemNames(ctx, []string{"txn-rent"})
		if err != nil {
			t.Fatalf("MatchedItemNames: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("MatchedItemNames = %+v, want exactly the one requested id", got)
		}
		if _, ok := got["txn-paycheck"]; ok {
			t.Errorf("MatchedItemNames reported a match outside the requested batch: %+v", got)
		}
	})

	t.Run("an empty batch reports no matches and touches no storage", func(t *testing.T) {
		svc, ctx := newService(t)
		got, err := svc.MatchedItemNames(ctx, nil)
		if err != nil {
			t.Fatalf("MatchedItemNames: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("MatchedItemNames(nil) = %+v, want empty", got)
		}
	})
}
