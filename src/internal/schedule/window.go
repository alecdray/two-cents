package schedule

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// OccurrenceState is one dated instance of a declared item together with
// whatever has been decided about it — the read model the matching surface
// renders.
//
// The three states are the three the record can hold: nothing decided, settled
// by a transaction, or manually cleared. Only the middle one leaves the
// timeline.
type OccurrenceState struct {
	Occurrence time.Time
	// Decision is the source of the stored decision, empty when there is none.
	Decision MatchSource
	// SettledBy is the transaction the decision points at, empty for a clear.
	// What the decision *says* is separate from what can be shown of it: between
	// a provider `removed` and the sync that drops the match, this still names a
	// row the ledger no longer holds.
	SettledBy string
	// Transaction is that row, carried for display and zero when the ledger no
	// longer holds it — the user confirms a match against what they can see of
	// it, not against an id.
	Transaction Candidate
	// Candidates are the rows this occurrence could be settled by, filled only
	// while it is unsettled. A settled occurrence offers none: the way to change
	// one is to clear it first, which is a decision the user states rather than
	// one a stray click makes.
	Candidates OccurrenceCandidates
}

// Settled reports whether a transaction satisfied this occurrence.
func (o OccurrenceState) Settled() bool { return o.SettledBy != "" }

// Cleared reports the user's assertion that nothing satisfied this occurrence.
// It is a decision, not the absence of one, which is why it is distinguishable
// from an outstanding occurrence.
func (o OccurrenceState) Cleared() bool {
	return o.Decision == MatchManual && o.SettledBy == ""
}

// Outstanding reports an occurrence nothing has been decided about — the one
// state that offers candidates.
func (o OccurrenceState) Outstanding() bool { return o.Decision == "" }

// ItemOccurrences is one declared item beside the occurrences of it that fall in
// the matching window.
type ItemOccurrences struct {
	Item        Item
	Occurrences []OccurrenceState
}

// MatchingWindow reports every active item's occurrences that could already have
// been satisfied — one cadence interval back through today — with the decision
// standing against each.
//
// It stops at today because a future occurrence has no transaction to find, and
// offering to settle one would invite the user to record something that has not
// happened.
func (s *Service) MatchingWindow(ctx contextx.ContextX) ([]ItemOccurrences, error) {
	today := startOfDay(s.now().In(s.location))

	items, err := s.ActiveItems(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}

	matches, err := s.Matches(ctx, earliestLookback(items, today), today)
	if err != nil {
		return nil, err
	}
	decided := make(map[string]Match, len(matches))
	for _, m := range matches {
		decided[m.ItemID+"|"+occurrenceKey(m.Occurrence)] = m
	}

	settledRows, err := s.settledTransactions(ctx, matches)
	if err != nil {
		return nil, err
	}

	// One ledger read for the whole page: a window per occurrence would re-read
	// the same span once for every row rendered.
	offers, err := s.offerContext(ctx, items, today)
	if err != nil {
		return nil, err
	}

	out := make([]ItemOccurrences, 0, len(items))
	for _, item := range items {
		occurrences := item.Occurrences(item.OneCadenceBefore(today), today)
		states := make([]OccurrenceState, 0, len(occurrences))
		for _, occurrence := range occurrences {
			state := OccurrenceState{Occurrence: occurrence}
			if m, ok := decided[item.ID+"|"+occurrenceKey(occurrence)]; ok {
				state.Decision = m.Source
				state.SettledBy = m.TransactionID
				state.Transaction = settledRows[m.TransactionID]
			}
			if !state.Settled() {
				state.Candidates = offers.offer(item, occurrence)
			}
			states = append(states, state)
		}
		out = append(out, ItemOccurrences{Item: item, Occurrences: states})
	}
	return out, nil
}

// offerContext is everything the candidate split needs, read once: the checking
// rows across the widest window any item looks back over, which of them are
// already spoken for, and each item's learned merchant.
type offerContext struct {
	rows    []Candidate
	claimed map[string]bool
	learned map[string]string
}

func (s *Service) offerContext(ctx contextx.ContextX, items []Item, today time.Time) (offerContext, error) {
	if s.ledger == nil {
		return offerContext{}, nil
	}

	from := earliestLookback(items, today).AddDate(0, 0, -matchDateToleranceDays)
	rows, err := s.ledger.CheckingCandidates(ctx, from, today.AddDate(0, 0, matchDateToleranceDays))
	if err != nil {
		return offerContext{}, err
	}

	spokenFor, err := s.repo().SettledTransactionIDs(ctx)
	if err != nil {
		return offerContext{}, err
	}
	claimed := make(map[string]bool, len(spokenFor))
	for _, id := range spokenFor {
		claimed[id] = true
	}

	learned, err := s.learnedMerchants(ctx, items)
	if err != nil {
		return offerContext{}, err
	}
	return offerContext{rows: rows, claimed: claimed, learned: learned}, nil
}

// offer splits the read rows for one occurrence.
func (c offerContext) offer(item Item, occurrence time.Time) OccurrenceCandidates {
	var offered OccurrenceCandidates
	for _, row := range c.rows {
		if c.claimed[row.TransactionID] {
			continue
		}
		if admissible(item, occurrence, row, c.learned[item.ID]) {
			offered.Admissible = append(offered.Admissible, row)
			continue
		}
		offered.Other = append(offered.Other, row)
	}
	return offered
}

// settledTransactions resolves the rows behind a set of decisions, keyed by id.
// A decision whose row the ledger no longer holds resolves to nothing and reads
// as outstanding until the next sync drops it, which is the safe direction.
func (s *Service) settledTransactions(ctx contextx.ContextX, matches []Match) (map[string]Candidate, error) {
	if s.ledger == nil {
		return nil, nil
	}
	var ids []string
	for _, m := range matches {
		if m.Settled() {
			ids = append(ids, m.TransactionID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.ledger.TransactionsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Candidate, len(rows))
	for _, c := range rows {
		byID[c.TransactionID] = c
	}
	return byID, nil
}

// OccurrenceCandidates is what an outstanding occurrence offers the user: the
// rows resolution judged admissible, and every other checking row it could have
// been.
//
// The split is presentational, not a rule. Manual association is the guaranteed
// path and may point at any checking row — but the admissible set is usually
// right, so it leads.
type OccurrenceCandidates struct {
	Admissible []Candidate
	Other      []Candidate
}

// CandidatesFor offers the checking rows one occurrence could be settled by —
// the single-occurrence entry point onto the same split MatchingWindow applies
// to a whole page.
func (s *Service) CandidatesFor(ctx contextx.ContextX, itemID string, occurrence time.Time) (OccurrenceCandidates, error) {
	items, err := s.ActiveItems(ctx)
	if err != nil {
		return OccurrenceCandidates{}, err
	}
	var item Item
	for _, candidate := range items {
		if candidate.ID == itemID {
			item = candidate
		}
	}
	if item.ID == "" {
		return OccurrenceCandidates{}, nil
	}

	today := startOfDay(s.now().In(s.location))
	offers, err := s.offerContext(ctx, items, today)
	if err != nil {
		return OccurrenceCandidates{}, err
	}
	return offers.offer(item, occurrence), nil
}
