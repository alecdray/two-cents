package schedule

import (
	"math"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// How far from an occurrence a candidate may fall, and still be admissible. Five
// days is comfortably inside the fourteen a biweekly cadence allows, so the
// windows of adjacent occurrences never overlap and a candidate can only be
// near-enough to one of them.
const matchDateToleranceDays = 5

// The narrow amount test: a candidate stands on its amount alone only when it is
// this close to the declared figure. The dollar floor keeps a small declaration
// from being matched by a percentage band too tight to be useful.
const (
	matchAmountTolerancePercent = 0.05
	matchAmountToleranceFloor   = 5.00
)

// The wide test, reached only with a distinctive learned merchant: a sanity
// bound rather than a measurement. `Amount` is the *maximum* expected for an
// outflow, so the real figure is routinely well under it, and anything inside
// half-to-double is plausibly the same obligation.
const (
	matchLearnedLowerFactor = 0.5
	matchLearnedUpperFactor = 2.0
)

// Candidate is one checking transaction that could satisfy an occurrence, in
// this module's own vocabulary.
//
// Deliberately not a transactions.Transaction: the adapter at the composition
// root translates into this shape, which is what lets resolution reconcile
// against the ledger without either module importing the other ([ADR-0027]).
// Amount follows the app-wide convention, outflow positive.
type Candidate struct {
	TransactionID string
	Date          time.Time
	Amount        float64
	Merchant      string
}

// Ledger is the port resolution reads the ledger through. Its adapter is the
// only code holding both `accounts` (to identify checking) and `transactions`
// (to query the range), and it lives at the composition root.
type Ledger interface {
	// CheckingCandidates returns the checking transactions dated in [from, to].
	// Scoping to checking is the adapter's job, and it is not an optimisation:
	// the schedule is checking activity by definition, so a match anywhere else
	// would drop an occurrence against money that never left the account the
	// sweep reasons about.
	CheckingCandidates(ctx contextx.ContextX, from, to time.Time) ([]Candidate, error)
	// TransactionsByID returns the given transactions, skipping any that no
	// longer exist. Absence is itself an answer: a provider `removed` deletes
	// rows, so a stored match can be orphaned by an ordinary sync and an id that
	// comes back missing is one whose match must be dropped. The merchant it
	// carries is the other half — the learned signal is read from the
	// transaction behind a match the user already made, which may be older than
	// any resolution window.
	TransactionsByID(ctx contextx.ContextX, ids []string) ([]Candidate, error)
}

// ResolveOccurrenceMatches re-resolves automatic matches across the window an
// occurrence could already have been satisfied in: one cadence interval back
// through today, per item. A future occurrence has no transaction to find.
//
// It re-resolves from scratch rather than working a delta - the same self-healing
// shape as the categorization sweep - so a match a failed pass never made resolves
// on the next one instead of waiting for a backfill.
func (s *Service) ResolveOccurrenceMatches(ctx contextx.ContextX) error {
	if s.ledger == nil {
		return nil
	}
	today := startOfDay(s.now().In(s.location))

	items, err := s.ActiveItems(ctx)
	if err != nil {
		return err
	}
	if err := s.dropOrphanedMatches(ctx, items, today); err != nil {
		return err
	}

	learned, err := s.learnedMerchants(ctx, items)
	if err != nil {
		return err
	}

	// A transaction settles at most one obligation. The claimed set starts from
	// every decision already stored - not just this pass's - because the
	// constraint is global and the alternative is discovering it as a failed
	// write partway through the pass.
	spokenFor, err := s.repo().SettledTransactionIDs(ctx)
	if err != nil {
		return err
	}
	claimed := make(map[string]bool, len(spokenFor))
	for _, id := range spokenFor {
		claimed[id] = true
	}

	var decisions []Match
	for _, item := range items {
		from := item.OneCadenceBefore(today)
		candidates, err := s.ledger.CheckingCandidates(ctx, from.AddDate(0, 0, -matchDateToleranceDays), today.AddDate(0, 0, matchDateToleranceDays))
		if err != nil {
			return err
		}
		for _, occurrence := range item.Occurrences(from, today) {
			winner, ok := bestCandidate(item, occurrence, candidates, learned[item.ID], claimed)
			if !ok {
				continue
			}
			claimed[winner.TransactionID] = true
			decisions = append(decisions, Match{
				ItemID:        item.ID,
				Occurrence:    occurrence,
				TransactionID: winner.TransactionID,
			})
		}
	}
	return s.RecordAutoMatches(ctx, decisions)
}

// bestCandidate picks the one candidate that satisfies an occurrence, or reports
// false.
//
// Ambiguity declines. Two admissible candidates the tie-breaks cannot separate
// produce no match rather than a guess: a miss leaves the occurrence on the
// timeline and over-reserves, which is visible and correctable, while a false
// match drops an obligation silently and under-reserves - the one direction the
// model forbids.
func bestCandidate(item Item, occurrence time.Time, candidates []Candidate, learnedMerchant string, claimed map[string]bool) (Candidate, bool) {
	var best Candidate
	var bestFound, tied bool
	for _, c := range candidates {
		if claimed[c.TransactionID] || !admissible(item, occurrence, c, learnedMerchant) {
			continue
		}
		if !bestFound {
			best, bestFound = c, true
			continue
		}
		switch compareCandidates(item, occurrence, c, best) {
		case -1:
			best, tied = c, false
		case 0:
			tied = true
		}
	}
	if !bestFound || tied {
		return Candidate{}, false
	}
	return best, true
}

// admissible applies every criterion a candidate must clear.
func admissible(item Item, occurrence time.Time, c Candidate, learnedMerchant string) bool {
	if !directionAgrees(item, c) {
		return false
	}
	if daysApart(c.Date, occurrence) > matchDateToleranceDays {
		return false
	}
	return amountAdmissible(item, c, learnedMerchant)
}

// directionAgrees tests the sign, in the app-wide outflow-positive convention. A
// zero-amount candidate agrees with nothing.
func directionAgrees(item Item, c Candidate) bool {
	if item.Direction == DirectionOut {
		return c.Amount > 0
	}
	return c.Amount < 0
}

// amountAdmissible runs the two tests an amount may clear.
//
// The learned test exists because `Amount` is the *maximum* expected for an
// outflow: a $200 declaration against a $63 bill is the declaration working
// exactly as intended, and no amount-only rule accepts it without accepting far
// too much. A merchant the user has already confirmed for this item is evidence
// of a different kind - but only while it is distinctive, which is the caller's
// to establish (see learnedMerchants).
func amountAdmissible(item Item, c Candidate, learnedMerchant string) bool {
	actual := math.Abs(c.Amount)
	if learnedMerchant != "" && c.Merchant == learnedMerchant {
		return actual >= item.Amount*matchLearnedLowerFactor && actual <= item.Amount*matchLearnedUpperFactor
	}
	tolerance := math.Max(item.Amount*matchAmountTolerancePercent, matchAmountToleranceFloor)
	return math.Abs(actual-item.Amount) <= tolerance
}

// compareCandidates orders two admissible candidates: nearer the occurrence
// wins, then nearer the declared amount. It reports 0 when nothing separates
// them, which is what makes the caller decline.
func compareCandidates(item Item, occurrence time.Time, a, b Candidate) int {
	switch da, db := daysApart(a.Date, occurrence), daysApart(b.Date, occurrence); {
	case da < db:
		return -1
	case da > db:
		return 1
	}
	switch aa, ab := math.Abs(math.Abs(a.Amount)-item.Amount), math.Abs(math.Abs(b.Amount)-item.Amount); {
	case aa < ab:
		return -1
	case aa > ab:
		return 1
	}
	return 0
}

// daysApart counts whole calendar days between two dates, either direction.
func daysApart(a, b time.Time) int {
	diff := startOfDay(a).Sub(startOfDay(b)).Hours() / 24
	return int(math.Abs(math.Round(diff)))
}

// startOfDay reduces an instant to the calendar day it falls on. An occurrence
// is a day, not a moment, and every comparison here is between days.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// dropOrphanedMatches removes every stored decision whose transaction no longer
// exists, so its occurrence returns to the timeline.
//
// Manual decisions are dropped too. Manual's guarantee is that automatic will
// not overwrite it, never that the decision outlives the fact it was about: a
// provider `removed` deletes rows, and a match left pointing at one hides an
// obligation behind a transaction that is gone - under-reserving silently,
// which is the one direction this model may not degrade in ([ADR-0027]).
func (s *Service) dropOrphanedMatches(ctx contextx.ContextX, items []Item, today time.Time) error {
	matches, err := s.Matches(ctx, earliestLookback(items, today), today)
	if err != nil {
		return err
	}

	var ids []string
	for _, m := range matches {
		if m.Settled() {
			ids = append(ids, m.TransactionID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	held, err := s.ledger.TransactionsByID(ctx, ids)
	if err != nil {
		return err
	}
	alive := make(map[string]bool, len(held))
	for _, c := range held {
		alive[c.TransactionID] = true
	}

	for _, m := range matches {
		if !m.Settled() || alive[m.TransactionID] {
			continue
		}
		if err := s.repo().DeleteMatch(ctx, m.ItemID, m.Occurrence); err != nil {
			return err
		}
	}
	return nil
}

// earliestLookback is the start of the widest window any item looks back over.
// Each item's lookback is its own cadence interval, so the set's earliest is
// what a single range query has to reach to cover all of them.
func earliestLookback(items []Item, today time.Time) time.Time {
	from := today
	for _, item := range items {
		if start := item.OneCadenceBefore(today); start.Before(from) {
			from = start
		}
	}
	return from
}

// learnedMerchants maps each item to the merchant it has learned: the one behind
// the most recent occurrence the user settled by hand.
//
// Only a manual decision teaches. The merchant is read from the match the user
// already made rather than declared on the item, because a field the user must
// fill in to make matching work is a field that will be empty ([ADR-0027]) - and
// it is read through the ledger port rather than stored on the match row, since
// a confirmed match may be older than any window resolution looks at.
func (s *Service) learnedMerchants(ctx contextx.ContextX, items []Item) (map[string]string, error) {
	active := make(map[string]bool, len(items))
	for _, item := range items {
		active[item.ID] = true
	}

	confirmed, err := s.repo().LatestManualMatches(ctx)
	if err != nil {
		return nil, err
	}
	itemByTransaction := make(map[string]string, len(confirmed))
	var ids []string
	for _, m := range confirmed {
		if !active[m.ItemID] {
			continue
		}
		itemByTransaction[m.TransactionID] = m.ItemID
		ids = append(ids, m.TransactionID)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	held, err := s.ledger.TransactionsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	learned := make(map[string]string, len(held))
	claimants := map[string]int{}
	for _, c := range held {
		if c.Merchant == "" {
			continue
		}
		learned[itemByTransaction[c.TransactionID]] = c.Merchant
		claimants[c.Merchant]++
	}

	// A descriptor two items have both learned identifies neither, and both fall
	// back to the amount alone. Dropping it is not a refinement: learning a
	// shared bill-pay descriptor points at the wrong obligation and widens the
	// amount test in the same moment, and the ambiguity rule does not catch that
	// when the right row posts late ([ADR-0027]).
	for itemID, merchant := range learned {
		if claimants[merchant] > 1 {
			delete(learned, itemID)
		}
	}
	return learned, nil
}
