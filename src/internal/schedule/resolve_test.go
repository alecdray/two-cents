package schedule

import (
	"context"
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// fakeLedger stands in for the composition root's adapter over accounts +
// transactions. Resolution never sees a Transaction, only this module's own
// Candidate, which is what keeps the two modules from importing each other.
type fakeLedger struct {
	candidates []Candidate
	existing   map[string]bool
}

func (f *fakeLedger) CheckingCandidates(_ contextx.ContextX, from, to time.Time) ([]Candidate, error) {
	var out []Candidate
	for _, c := range f.candidates {
		if !f.holds(c.TransactionID) {
			continue
		}
		if !c.Date.Before(from) && !c.Date.After(to) {
			out = append(out, c)
		}
	}
	return out, nil
}

// holds reports whether the ledger still has the row. A transaction marked gone
// is gone everywhere - it is no more available as a candidate than it is as the
// referent of a stored match. `existing` nil means nothing has been deleted.
func (f *fakeLedger) holds(id string) bool {
	return f.existing == nil || f.existing[id]
}

// TransactionsByID answers from the same candidate set, skipping anything the
// test has marked gone. `existing` nil means every candidate is still there.
func (f *fakeLedger) TransactionsByID(_ contextx.ContextX, ids []string) ([]Candidate, error) {
	byID := map[string]Candidate{}
	for _, c := range f.candidates {
		byID[c.TransactionID] = c
	}
	var out []Candidate
	for _, id := range ids {
		c, ok := byID[id]
		if !ok {
			continue
		}
		if !f.holds(id) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// resolvingService builds a Service whose clock is pinned, so a test states the
// run instant rather than depending on when it runs.
func resolvingService(t *testing.T, ledger Ledger, now time.Time) (*Service, contextx.ContextX) {
	t.Helper()
	svc := NewService(newTestDB(t), ledger, time.UTC)
	svc.now = func() time.Time { return now }
	return svc, contextx.NewContextX(context.Background())
}

func outflow(id string, date time.Time, amount float64, merchant string) Candidate {
	return Candidate{TransactionID: id, Date: date, Amount: amount, Merchant: merchant}
}

func TestResolveOccurrenceMatches(t *testing.T) {
	now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

	t.Run("an outflow of the declared amount on the occurrence date is matched", func(t *testing.T) {
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

		settled, err := svc.SettledOccurrences(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		if !settled.Has(item.ID, sept(1)) {
			t.Error("the 1 September occurrence is not settled; an exact-amount outflow on the day should resolve")
		}
	})

	t.Run("two equally good candidates decline rather than guess", func(t *testing.T) {
		// A miss over-reserves visibly and is one click from settled; a false
		// match drops an obligation silently. Precision, never coverage.
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-a", sept(1), 2400, "ONLINE PAYMENT"),
			outflow("txn-b", sept(1), 2400, "ONLINE PAYMENT"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, rent())
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
			t.Error("resolution picked between two indistinguishable candidates; it must decline")
		}
	})

	t.Run("an inflow is never matched to an outflow's occurrence", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-refund", sept(1), -2400, "GREYSTONE PROPERTY"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, rent())
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
			t.Error("an inflow settled an outflow's occurrence; direction must agree")
		}
	})

	t.Run("a candidate too far from the occurrence is not admissible", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-far", sept(1).AddDate(0, 0, 9), 2400, "GREYSTONE PROPERTY"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, rent())
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
			t.Error("a candidate nine days away settled the occurrence; date proximity must bound it")
		}
	})
}

func TestResolveDropsOrphanedMatches(t *testing.T) {
	now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

	t.Run("a manual match whose transaction is gone is dropped and its occurrence returns", func(t *testing.T) {
		// Manual's guarantee is that automatic will not overwrite it, never that
		// the decision outlives the fact it was about. An orphaned match hides an
		// obligation behind a transaction that no longer exists - under-reserving,
		// silently, which is the one direction the model forbids.
		ledger := &fakeLedger{
			candidates: []Candidate{outflow("txn-gone", sept(1), 2400, "GREYSTONE PROPERTY")},
			existing:   map[string]bool{"txn-gone": false},
		}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, rent())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, item.ID, sept(1), "txn-gone"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("ResolveOccurrenceMatches: %v", err)
		}

		settled, err := svc.SettledOccurrences(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		if settled.Has(item.ID, sept(1)) {
			t.Error("the occurrence is still settled; a match whose transaction is gone must be dropped")
		}
	})
}

func aug(day int) time.Time {
	return time.Date(2026, time.August, day, 0, 0, 0, 0, time.UTC)
}

// utilities is declared at the conservative maximum a variable bill reaches, so
// the amount that actually lands is routinely below it - far enough below to
// clear the narrow band, which is what no amount-only rule can match.
func utilities() Item {
	return Item{
		Name:       "Utilities",
		Direction:  DirectionOut,
		Amount:     200,
		Cadence:    CadenceMonthly,
		DayOfMonth: 1,
		Active:     true,
	}
}

func TestResolveLearnedMerchant(t *testing.T) {
	now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

	t.Run("a merchant confirmed by hand matches an amount the narrow band rejects", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-util-aug", aug(1), 132, "CITY UTILITIES"),
			outflow("txn-util-sep", sept(1), 140, "CITY UTILITIES"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, utilities())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		// The user settled August by hand. That is what teaches the descriptor -
		// no new declared field, and the match is older than any window
		// resolution looks at.
		if err := svc.MatchOccurrence(ctx, item.ID, aug(1), "txn-util-aug"); err != nil {
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
			t.Error("the September occurrence is not settled; a learned merchant should carry $140 against a $200 declaration")
		}
	})

	t.Run("the same amount is rejected when no merchant has been confirmed", func(t *testing.T) {
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-util-sep", sept(1), 140, "CITY UTILITIES"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, utilities())
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
			t.Error("the occurrence was matched on amount alone; $140 against a $200 declaration is far outside the narrow band")
		}
	})
}

// internet is a second declared bill that leaves checking the same way utilities
// does, which is what makes one shared descriptor ambiguous between them.
func internet() Item {
	return Item{
		Name:       "Internet",
		Direction:  DirectionOut,
		Amount:     150,
		Cadence:    CadenceMonthly,
		DayOfMonth: 1,
		Active:     true,
	}
}

func TestResolveDistinctiveMerchantGuard(t *testing.T) {
	now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

	t.Run("a descriptor two items have learned is evidence for neither", func(t *testing.T) {
		// The failure this guards is the severe one: several bills leaving through
		// one bill-pay share a descriptor, so learning it identifies the wrong
		// obligation *and* widens the amount test at the same moment. Both items
		// fall back to the amount alone, which rejects $140.
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-util-aug", aug(1), 132, "ONLINE BILLPAY"),
			outflow("txn-net-aug", aug(1), 148, "ONLINE BILLPAY"),
			outflow("txn-sep", sept(1), 140, "ONLINE BILLPAY"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		util, err := svc.Create(ctx, utilities())
		if err != nil {
			t.Fatalf("Create utilities: %v", err)
		}
		net, err := svc.Create(ctx, internet())
		if err != nil {
			t.Fatalf("Create internet: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, util.ID, aug(1), "txn-util-aug"); err != nil {
			t.Fatalf("MatchOccurrence utilities: %v", err)
		}
		if err := svc.MatchOccurrence(ctx, net.ID, aug(1), "txn-net-aug"); err != nil {
			t.Fatalf("MatchOccurrence internet: %v", err)
		}

		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("ResolveOccurrenceMatches: %v", err)
		}

		settled, err := svc.SettledOccurrences(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		if settled.Has(util.ID, sept(1)) {
			t.Error("utilities matched on a descriptor internet has also learned; a shared descriptor identifies neither")
		}
		if settled.Has(net.ID, sept(1)) {
			t.Error("internet matched on a descriptor utilities has also learned; a shared descriptor identifies neither")
		}
	})
}

func TestResolveExcludesAlreadyMatched(t *testing.T) {
	now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

	t.Run("a transaction settling one occurrence is not offered to another", func(t *testing.T) {
		// A transaction settles at most one obligation - the partial unique index
		// says so, and resolution must not hand it to a second occurrence and
		// find that out from the database.
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-shared", sept(1), 150, "ONLINE PAYMENT"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		util, err := svc.Create(ctx, utilities())
		if err != nil {
			t.Fatalf("Create utilities: %v", err)
		}
		net, err := svc.Create(ctx, internet())
		if err != nil {
			t.Fatalf("Create internet: %v", err)
		}
		// The user has already said this row settled utilities, whatever its
		// amount suggests. It is spoken for.
		if err := svc.MatchOccurrence(ctx, util.ID, sept(1), "txn-shared"); err != nil {
			t.Fatalf("MatchOccurrence: %v", err)
		}

		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("ResolveOccurrenceMatches: %v", err)
		}

		settled, err := svc.SettledOccurrences(ctx, sept(1), sept(30))
		if err != nil {
			t.Fatalf("SettledOccurrences: %v", err)
		}
		if settled.Has(net.ID, sept(1)) {
			t.Error("internet took a transaction already settling utilities; a transaction settles at most one obligation")
		}
		if !settled.Has(util.ID, sept(1)) {
			t.Error("the manual match on utilities did not survive the pass")
		}
	})
}

func TestResolveSupersedesWorseAutomaticMatch(t *testing.T) {
	now := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.UTC)

	t.Run("a nearer candidate arriving later replaces the automatic match", func(t *testing.T) {
		// Resolution re-resolves its window from scratch every pass, so a row that
		// posts late supersedes the weaker decision made without it. No stale
		// automatic match survives on the strength of having been made first.
		ledger := &fakeLedger{candidates: []Candidate{
			outflow("txn-far", sept(4), 200, "ONLINE PAYMENT"),
		}}
		svc, ctx := resolvingService(t, ledger, now)
		item, err := svc.Create(ctx, utilities())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("first pass: %v", err)
		}
		if got := matchedTransaction(t, svc, ctx, item.ID, sept(1)); got != "txn-far" {
			t.Fatalf("first pass matched %q, want txn-far", got)
		}

		ledger.candidates = append(ledger.candidates, outflow("txn-near", sept(1), 200, "ONLINE PAYMENT"))

		if err := svc.ResolveOccurrenceMatches(ctx); err != nil {
			t.Fatalf("second pass: %v", err)
		}
		if got := matchedTransaction(t, svc, ctx, item.ID, sept(1)); got != "txn-near" {
			t.Errorf("second pass left %q standing, want txn-near; a candidate on the occurrence date beats one three days off", got)
		}
	})
}

// matchedTransaction reads back which transaction a decision points at, which
// SettledOccurrences deliberately does not say - the sweep is told only *that*
// an occurrence is settled.
func matchedTransaction(t *testing.T, svc *Service, ctx contextx.ContextX, itemID string, occurrence time.Time) string {
	t.Helper()
	matches, err := svc.Matches(ctx, occurrence, occurrence)
	if err != nil {
		t.Fatalf("Matches: %v", err)
	}
	for _, m := range matches {
		if m.ItemID == itemID {
			return m.TransactionID
		}
	}
	return ""
}
