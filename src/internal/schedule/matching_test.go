package schedule

import (
	"testing"
	"time"
)

// An occurrence is identified by its item and a calendar date, so the tests
// speak in dates rather than instants.
func sept(day int) time.Time {
	return time.Date(2026, time.September, day, 0, 0, 0, 0, time.UTC)
}

func TestSettledOccurrences(t *testing.T) {
	t.Run("a manually matched occurrence is settled", func(t *testing.T) {
		svc, ctx := newService(t)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-rent"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		settled, err := svc.SettledOccurrences(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		if !settled.Has(item.ID, sept(1)) {
			t.Errorf("settled = %+v, want the 1 September occurrence of %s", settled, item.ID)
		}
	})
}

func TestMatchPrecedence(t *testing.T) {
	t.Run("an automatic match does not overwrite a manual one", func(t *testing.T) {
		svc, ctx := newService(t)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-user-picked"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		err = svc.RecordAutoMatches(ctx, []Match{{
			ItemID: item.ID, Occurrence: sept(1), TransactionID: "txn-resolver-picked",
		}})
		if err != nil {
			t.Fatalf("RecordAutoMatches: %v", err)
		}

		matches, err := svc.Matches(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("matches = %+v, want exactly one", matches)
		}
		if matches[0].TransactionID != "txn-user-picked" || matches[0].Source != MatchManual {
			t.Errorf("match = %+v, want the user's decision untouched", matches[0])
		}
	})

	t.Run("an automatic match supersedes an earlier automatic one", func(t *testing.T) {
		// Resolution re-resolves its window from scratch every pass, so a better
		// candidate arriving later must win — no automatic decision survives on
		// the strength of having been made first.
		svc, ctx := newService(t)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		first := []Match{{ItemID: item.ID, Occurrence: sept(1), TransactionID: "txn-first"}}
		if err := svc.RecordAutoMatches(ctx, first); err != nil {
			t.Fatalf("RecordAutoMatches: %v", err)
		}
		second := []Match{{ItemID: item.ID, Occurrence: sept(1), TransactionID: "txn-better"}}
		if err := svc.RecordAutoMatches(ctx, second); err != nil {
			t.Fatalf("RecordAutoMatches: %v", err)
		}

		matches, err := svc.Matches(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		if len(matches) != 1 || matches[0].TransactionID != "txn-better" {
			t.Errorf("matches = %+v, want the later automatic decision to win", matches)
		}
	})

	t.Run("a cleared occurrence is left alone by automatic resolution", func(t *testing.T) {
		// Clearing is a stored decision, not the absence of one: without the
		// record, the next pass would simply re-make the match the user rejected.
		svc, ctx := newService(t)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.ClearOccurrence(ctx, item.ID, sept(1)); err != nil {
			t.Fatalf("ClearOccurrence: %v", err)
		}

		err = svc.RecordAutoMatches(ctx, []Match{{
			ItemID: item.ID, Occurrence: sept(1), TransactionID: "txn-rejected-again",
		}})
		if err != nil {
			t.Fatalf("RecordAutoMatches: %v", err)
		}

		matches, err := svc.Matches(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		if len(matches) != 1 {
			t.Fatalf("matches = %+v, want the stored clear to still be there", matches)
		}
		if matches[0].Source != MatchManual || matches[0].TransactionID != "" {
			t.Errorf("match = %+v, want the manual clear intact", matches[0])
		}

		settled, err := svc.SettledOccurrences(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		if settled.Has(item.ID, sept(1)) {
			t.Error("the cleared occurrence reads as settled; a manual clear must survive resolution and keep the occurrence on the timeline")
		}
	})
}

func TestDeletingAnItemRemovesItsDecisions(t *testing.T) {
	// The table declares ON DELETE CASCADE, but this connection does not enable
	// foreign keys (see docs/architecture/known-gaps.md), so the cascade never
	// fires. Left behind, an orphaned decision would be handed back by any
	// window query that overlapped it.
	svc, ctx := newService(t)
	item, err := svc.Create(ctx, rent())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-rent"); err != nil {
		t.Fatalf("MatchOccurrence: %v", err)
	}

	if err := svc.Delete(ctx, item.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	matches, err := svc.Matches(ctx, sept(1), sept(30))
	if err != nil {
		t.Fatalf("Matches: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("matches = %+v, want none once the item they were about is gone", matches)
	}
}
