package schedule

import (
	"testing"
	"time"
)

// undeclaredUtilities is a bill left for the system to compute: no amount
// typed at all. Leaving it unset is as ordinary as leaving any other optional
// field unset — there is nothing special the caller has to do.
func undeclaredUtilities() Item {
	return Item{
		Name:       "Utilities",
		Direction:  DirectionOut,
		Cadence:    CadenceMonthly,
		DayOfMonth: 1,
		Active:     true,
	}
}

// undeclaredPaycheck is income left for the system to compute.
func undeclaredPaycheck() Item {
	return Item{
		Name:       "Paycheck",
		Direction:  DirectionIn,
		Cadence:    CadenceBiweekly,
		AnchorDate: time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC),
		Active:     true,
	}
}

// jul and feb round out the months TestComputedAmount reaches across; sept
// and aug already exist for the rest of this package's tests.
func jul(day int) time.Time {
	return time.Date(2026, time.July, day, 0, 0, 0, 0, time.UTC)
}

func feb(day int) time.Time {
	return time.Date(2026, time.February, day, 0, 0, 0, 0, time.UTC)
}

func TestComputedAmount(t *testing.T) {
	// The trailing window this "now" reads over is July 1 through the end of
	// September: the current calendar month (September) plus the two before it.
	now := time.Date(2026, time.September, 20, 14, 30, 0, 0, time.UTC)

	t.Run("an outflow item with no declared amount reads back the maximum matched in the trailing window", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-jul", jul(15), 132, "CITY UTILITIES"),
			outflow("txn-sep", sept(1), 140, "CITY UTILITIES"),
			// Dated before the trailing window (more than two months back): a
			// bigger figure here must not win, or the window would not be one.
			outflow("txn-old", time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC), 999, "CITY UTILITIES"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, undeclaredUtilities())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		for occurrence, txn := range map[time.Time]string{jul(1): "txn-jul", sept(1): "txn-sep", feb(1): "txn-old"} {
			if err := svc.MatchOccurrence(ctx, item.ID, occurrence, txn); err != nil {
				t.Fatalf("MatchOccurrence %s: %v", txn, err)
			}
		}

		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got := items[0]
		if got.Amount == nil {
			t.Fatal("Amount is nil, want the computed maximum of the in-window matches")
		}
		if *got.Amount != 140 {
			t.Errorf("Amount = %v, want 140 — the maximum matched amount inside the trailing window, not the $999 match from June", *got.Amount)
		}
	})

	t.Run("an inflow item with no declared amount reads back the minimum matched in the trailing window", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-jul", jul(10), -3100, "EMPLOYER"),
			outflow("txn-sep", sept(4), -2950, "EMPLOYER"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, undeclaredPaycheck())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, jul(10), "txn-jul"); err != nil {
			t.Fatalf("MatchOccurrence jul: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(4), "txn-sep"); err != nil {
			t.Fatalf("MatchOccurrence sep: %v", err)
		}

		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got := items[0]
		if got.Amount == nil {
			t.Fatal("Amount is nil, want the computed minimum of the in-window matches")
		}
		if *got.Amount != 2950 {
			t.Errorf("Amount = %v, want 2950 — the minimum expected paycheck is the conservative reading for an inflow", *got.Amount)
		}
	})

	t.Run("evidence is filtered by the matched transaction's own date, not the occurrence it settles", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			// Matched to an occurrence inside the window, but the transaction
			// itself is an old row - a manual match to something from February.
			outflow("txn-old-row", feb(1), 500, "CITY UTILITIES"),
			// Matched to an occurrence outside the window (February), but the
			// transaction itself posted in September.
			outflow("txn-recent-row", sept(15), 175, "CITY UTILITIES"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, undeclaredUtilities())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-old-row"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, feb(1), "txn-recent-row"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got := items[0]
		if got.Amount == nil || *got.Amount != 175 {
			t.Errorf("Amount = %v, want 175 - the row actually dated inside the window, regardless of which occurrence it was matched to", got.Amount)
		}
	})

	t.Run("an item with no declared amount and nothing matched at all reads back no figure", func(t *testing.T) {
		svc, ctx := resolvingService(t, &fakeLedger{}, now)
		if _, err := svc.Create(ctx, undeclaredUtilities()); err != nil {
			t.Fatalf("Create: %v", err)
		}

		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if got := items[0]; got.Amount != nil {
			t.Errorf("Amount = %v, want nil - no figure is a distinct state, never zero", *got.Amount)
		}
	})

	t.Run("matches that exist but fall outside the trailing window still read back as no figure", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-ancient", time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC), 500, "CITY UTILITIES"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, undeclaredUtilities())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-ancient"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if got := items[0]; got.Amount != nil {
			t.Errorf("Amount = %v, want nil - the only match is over a year old, well outside the trailing window", *got.Amount)
		}
	})

	t.Run("a declared amount reads back exactly as declared regardless of match history", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-sep", sept(1), 999, "CITY UTILITIES"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, utilities()) // declared at 200
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-sep"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got := items[0]
		if got.Amount == nil || *got.Amount != 200 {
			t.Errorf("Amount = %v, want the declared 200 - a match history suggesting $999 must never override it", got.Amount)
		}
	})

	t.Run("resolving automatic matches never rewrites a declared amount", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-rent", sept(1), 2400, "GREYSTONE PROPERTY"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		if _, err := svc.Create(ctx, rent()); err != nil { // declared at 2400
			t.Fatalf("Create: %v", err)
		}

		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("ResolveOccurrenceMatches: %v", err)
		}

		items, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		got := items[0]
		if got.Amount == nil || *got.Amount != 2400 {
			t.Errorf("Amount = %v after resolution, want the declared 2400 untouched - no automatic process may alter it", got.Amount)
		}
	})
}

// TestResolveUsesComputedAmount demonstrates the boundary automatic matching
// keeps: it needs a real, usable number to test a candidate's amount against,
// whether that number was declared or computed, and declines outright when
// there is neither - the same precision-over-coverage discipline as any other
// ambiguous case, not a special one.
func TestResolveUsesComputedAmount(t *testing.T) {
	now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

	t.Run("automatic resolution tests a candidate against the computed amount when none is declared", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-aug", aug(1), 140, "CITY UTILITIES"),
			outflow("txn-sep", sept(1), 140, "CITY UTILITIES"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, undeclaredUtilities())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		// The manual August match is the item's first evidence - what an
		// undeclared amount computes from, and what an item bootstrapped from a
		// transaction would already carry.
		if err := svc.MatchOccurrence(ctx, item.ID, aug(1), "txn-aug"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("ResolveOccurrenceMatches: %v", err)
		}

		settled, err := svc.SettledOccurrences(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		if !settled.Has(item.ID, sept(1)) {
			t.Error("the September occurrence did not settle; resolution should test $140 against the $140 computed from the item's own match")
		}
	})

	t.Run("an item with no declared amount and no matches yet declines every candidate rather than guessing", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-sep", sept(1), 140, "CITY UTILITIES"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, undeclaredUtilities())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("ResolveOccurrenceMatches: %v", err)
		}

		settled, err := svc.SettledOccurrences(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		if settled.Has(item.ID, sept(1)) {
			t.Error("an item with no real number at all matched a candidate; with nothing to test against it must decline, not guess")
		}
	})
}
