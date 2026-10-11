package sweep

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/core/timex"
	"github.com/alecdray/two-cents/src/internal/schedule"

	_ "github.com/mattn/go-sqlite3"
)

// This file closes the one seam every other sweep test in this package leaves
// untouched: every other test hands compute() a schedule.Item literal it built
// by hand ([[monthly]], [datelessMonthly], [noFigureItem] above), so nothing
// here has ever run the *real* schedule.Service pipeline — bootstrapping an
// Item from a transaction and resolving its amount live from matches
// ([ADR-0031]) — into sweep's own computation. The boundary between the two
// modules is just Item.Amount (already resolved by the time sweep sees it), so
// the risk is narrow, but it is real: this proves the resolved figure and a
// settled/unsettled read straight off schedule.Service still respect the
// invariants unchanged by that build — blocks only on a missing dollar value,
// and the existing outflow/inflow missing-date asymmetry, now exercised on a
// dateless item too.

// bootstrapTestDB migrates a temp SQLite DB — the one thing every other test in
// this package never needs, since they all construct a schedule.Item directly
// rather than going through a live schedule.Service.
func bootstrapTestDB(t *testing.T) *db.DB {
	t.Helper()
	migrationsDir, err := filepath.Abs("../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return db.WrapSqlDB(sqlDB)
}

// bootstrapLedger is this test's stand-in for the composition root's adapter
// over accounts + transactions ([ADR-0027]) — CreateItemFromTransaction's only
// dependency, holding the one seeding candidate a bootstrap needs.
type bootstrapLedger struct {
	candidate schedule.Candidate
}

func (l bootstrapLedger) CheckingCandidates(_ contextx.ContextX, from, to time.Time) ([]schedule.Candidate, error) {
	if l.candidate.Date.Before(from) || l.candidate.Date.After(to) {
		return nil, nil
	}
	return []schedule.Candidate{l.candidate}, nil
}

func (l bootstrapLedger) TransactionsByID(_ contextx.ContextX, ids []string) ([]schedule.Candidate, error) {
	for _, id := range ids {
		if id == l.candidate.TransactionID {
			return []schedule.Candidate{l.candidate}, nil
		}
	}
	return nil, nil
}

// resolvedItemFromBootstrap drives the real schedule.Service — the same
// CreateItemFromTransaction + ActiveItems path the sweep composition root
// wires — over its own migrated DB, and returns the one resolved Item it
// produced plus the SettledSet the same way Service.Compute itself reads it
// (schedule.SettledOccurrences over one cadence interval back through the
// horizon). now is real wall-clock, threaded through everywhere a date is
// reckoned so the schedule.Service's own uninjectable clock (unexported,
// package-private) never disagrees with what this test hands compute().
func resolvedItemFromBootstrap(t *testing.T, now time.Time, txnID string, seedDate time.Time, seedAmount float64, declaration schedule.Item) (schedule.Item, schedule.SettledSet) {
	t.Helper()
	ctx := contextx.NewContextX(context.Background())
	ledger := bootstrapLedger{candidate: schedule.Candidate{
		TransactionID: txnID,
		Date:          seedDate,
		Amount:        seedAmount,
		Merchant:      declaration.Name,
	}}
	scheduleSvc := schedule.NewService(bootstrapTestDB(t), ledger, now.Location())

	if _, err := scheduleSvc.CreateItemFromTransaction(ctx, txnID, declaration); err != nil {
		t.Fatalf("CreateItemFromTransaction: %v", err)
	}

	items, err := scheduleSvc.ActiveItems(ctx)
	if err != nil {
		t.Fatalf("ActiveItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ActiveItems = %+v, want exactly the one bootstrapped item", items)
	}

	settled, err := scheduleSvc.SettledOccurrences(ctx, timex.AddMonthsClamped(startOfDay(now), -1), horizonFrom(now))
	if err != nil {
		t.Fatalf("SettledOccurrences: %v", err)
	}
	return items[0], settled
}

// TestBootstrappedItemResolvedAmountRespectsSweepInvariants bootstraps an Item
// from a seeding transaction dated last calendar month — so the bootstrap
// match settles *that* month's occurrence and this month's is left open and
// unmatched, regardless of what day of the month the suite happens to run on
// — and feeds the real resolved Item straight into compute(), the same
// unexported function every other test in this package already exercises
// with hand-built items.
func TestBootstrappedItemResolvedAmountRespectsSweepInvariants(t *testing.T) {
	now := time.Now().UTC()
	checking := []accounts.Account{cashAccount("Everyday", false, 5000, true, freshAgo, now)}

	// A date safely in the previous calendar month regardless of where "today"
	// falls in its own month — the 10th of the month before this one.
	lastMonthTenth := time.Date(now.Year(), now.Month(), 10, 12, 0, 0, 0, now.Location()).AddDate(0, -1, 0)

	computeFromBootstrap := func(declaration schedule.Item) Recommendation {
		item, settled := resolvedItemFromBootstrap(t, now, "txn-seed", lastMonthTenth, 45, declaration)
		in := deriveAccounts(checking, nil, now)
		in.items = []schedule.Item{item}
		in.settled = settled.Has
		in.fixedSafetyMargin = 500
		return compute(in, now)
	}

	t.Run("a dated monthly item's resolved amount lands overdue at the run instant", func(t *testing.T) {
		rec := computeFromBootstrap(schedule.Item{
			Name:       "Water",
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: 1,
		})

		if rec.Kind != KindNumeric {
			t.Fatalf("kind = %s reasons = %v, want numeric — a resolved figure is a known dollar value", rec.Kind, rec.Reasons)
		}
		event, ok := findEvent(rec.Timeline, "Water")
		if !ok {
			t.Fatalf("timeline = %v, want a Water event — the bootstrap match settled last month, leaving this month's occurrence open and overdue", summaries(rec.Timeline))
		}
		if event.Amount != 45 {
			t.Errorf("amount = %v, want 45 — the one match's amount, read live through EffectiveAmount rather than typed", event.Amount)
		}
		if event.Date.Year() != now.Year() || event.Date.YearDay() != now.YearDay() {
			t.Errorf("date = %s, want the run instant %s — an outflow already due lands immediately", event.Date, now)
		}
	})

	t.Run("a dateless monthly item's resolved amount takes the same overdue branch for its whole open month", func(t *testing.T) {
		rec := computeFromBootstrap(schedule.Item{
			Name:    "Cable",
			Cadence: schedule.CadenceMonthly,
			// DayOfMonth left at zero: dateless ([ADR-0031]).
		})

		if rec.Kind != KindNumeric {
			t.Fatalf("kind = %s reasons = %v, want numeric", rec.Kind, rec.Reasons)
		}
		event, ok := findEvent(rec.Timeline, "Cable")
		if !ok {
			t.Fatalf("timeline = %v, want a Cable event — dateless has no \"still ahead\": unmatched, it always takes the overdue branch", summaries(rec.Timeline))
		}
		if event.Amount != 45 {
			t.Errorf("amount = %v, want 45 — resolved from the same bootstrap match, dateless changes nothing about how the amount reads", event.Amount)
		}
		if event.Date.Year() != now.Year() || event.Date.YearDay() != now.YearDay() {
			t.Errorf("date = %s, want the run instant %s", event.Date, now)
		}
	})

	t.Run("an item with no override and no matches at all still contributes nothing, real bootstrap or not", func(t *testing.T) {
		// The no-figure exception ([ADR-0031]) holds even once the amount comes
		// from a live resolution path rather than a hand-built nil: bootstrap a
		// second, unrelated item from a transaction the ledger reports nothing
		// useful for by asking about a window that excludes the seed entirely.
		ctx := contextx.NewContextX(context.Background())
		ledger := bootstrapLedger{candidate: schedule.Candidate{TransactionID: "txn-seed", Date: lastMonthTenth, Amount: 45, Merchant: "Gym"}}
		scheduleSvc := schedule.NewService(bootstrapTestDB(t), ledger, now.Location())
		if _, err := scheduleSvc.CreateItemFromTransaction(ctx, "txn-seed", schedule.Item{Name: "Gym", Cadence: schedule.CadenceMonthly, DayOfMonth: 1}); err != nil {
			t.Fatalf("CreateItemFromTransaction: %v", err)
		}
		// Deleting the match strips the item back to no evidence at all, the
		// same "declared but nothing yet matched" state a brand-new item without
		// this path would be in — resolveAmounts must still read it as no figure,
		// never zero.
		items, err := scheduleSvc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		// The bootstrap match landed on the 1st of the seed transaction's own
		// month — the item's declared day, clamped to nothing since 1 always
		// exists — the same date occurrenceForBootstrap resolved to when
		// CreateItemFromTransaction recorded it.
		bootstrapOccurrence := time.Date(lastMonthTenth.Year(), lastMonthTenth.Month(), 1, 0, 0, 0, 0, now.Location())
		if err := scheduleSvc.ClearOccurrence(ctx, items[0].ID, bootstrapOccurrence); err != nil {
			t.Fatalf("ClearOccurrence: %v", err)
		}
		active, err := scheduleSvc.ActiveItems(ctx)
		if err != nil {
			t.Fatalf("ActiveItems: %v", err)
		}
		if len(active) != 1 || active[0].Amount != nil {
			t.Fatalf("ActiveItems = %+v, want the one item with Amount still nil — no figure, not zero", active)
		}

		settled, err := scheduleSvc.SettledOccurrences(ctx, timex.AddMonthsClamped(startOfDay(now), -1), horizonFrom(now))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		in := deriveAccounts(checking, nil, now)
		in.items = active
		in.settled = settled.Has
		in.fixedSafetyMargin = 500
		rec := compute(in, now)

		if rec.Kind != KindNumeric {
			t.Fatalf("kind = %s reasons = %v, want numeric — a missing dollar value on a schedule item is the one stated exception, not a block", rec.Kind, rec.Reasons)
		}
		if _, ok := findEvent(rec.Timeline, "Gym"); ok {
			t.Errorf("timeline = %v, want no Gym event — nothing to reserve without a figure", summaries(rec.Timeline))
		}
		if rec.SuggestedSweep != 4500 {
			t.Errorf("sweep = %v, want 5000 - 0 - 500 — the no-figure item reads as though never declared", rec.SuggestedSweep)
		}
	})
}

// findEvent locates the timeline event with the given label, if any.
func findEvent(events []TimelineEvent, label string) (TimelineEvent, bool) {
	for _, e := range events {
		if e.Label == label {
			return e, true
		}
	}
	return TimelineEvent{}, false
}
