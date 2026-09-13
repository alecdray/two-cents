package schedule

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// MatchSource records who decided an occurrence's match. It is the whole basis
// of precedence: a manual decision is never overwritten by an automatic one,
// the same grain the app already uses for a categorization override.
type MatchSource string

const (
	// MatchManual is the user's own decision — an association they made, or their
	// assertion that nothing satisfied the occurrence.
	MatchManual MatchSource = "manual"
	// MatchAuto is best-effort resolution's decision, re-resolved from scratch on
	// every pass and freely superseded by a later automatic one.
	MatchAuto MatchSource = "auto"
)

// Match is the stored decision about one occurrence, identified by its item and
// the calendar date the occurrence falls on.
//
// TransactionID is empty when the decision is that *nothing* satisfied the
// occurrence — a manual clear. That is a different fact from no record at all:
// the occurrence is still placed on the timeline either way, but a stored clear
// also tells automatic resolution to leave it alone, without which the next pass
// would simply re-make the match the user just rejected.
type Match struct {
	ItemID        string
	Occurrence    time.Time
	TransactionID string
	Source        MatchSource
}

// Settled reports whether this decision takes the occurrence off the timeline.
// A clear does not: the user said nothing satisfied it, so it is still owed.
func (m Match) Settled() bool { return m.TransactionID != "" }

// occurrenceKey renders an occurrence as the calendar date it falls on. An
// occurrence is a day, not an instant, so the key must not carry a zone that
// could shift it across a boundary on the way to or from storage.
func occurrenceKey(occurrence time.Time) string {
	return occurrence.Format("2006-01-02")
}

// SettledSet reports which occurrences carry a match that settles them, keyed by
// item. It deliberately carries no transaction ids: the sweep is told *which*
// occurrences are settled and never *what* settled them, because taking the id
// would hand it a ledger reference it must then be trusted not to follow
// ([ADR-0027]).
type SettledSet struct {
	byItem map[string]map[string]struct{}
}

// Has reports whether the given occurrence of the given item is settled.
func (s SettledSet) Has(itemID string, occurrence time.Time) bool {
	dates, ok := s.byItem[itemID]
	if !ok {
		return false
	}
	_, settled := dates[occurrenceKey(occurrence)]
	return settled
}

// MatchOccurrence records that a transaction satisfied an occurrence, as the
// user's own decision — the guaranteed path, which automatic resolution may
// never overwrite.
func (s *Service) MatchOccurrence(ctx contextx.ContextX, itemID string, occurrence time.Time, transactionID string) error {
	return s.repo().UpsertManualMatch(ctx, Match{
		ItemID:        itemID,
		Occurrence:    occurrence,
		TransactionID: transactionID,
	})
}

// SettledOccurrences reports the occurrences settled by a match inside the
// window, inclusive at both ends.
func (s *Service) SettledOccurrences(ctx contextx.ContextX, from, to time.Time) (SettledSet, error) {
	matches, err := s.repo().MatchesInRange(ctx, from, to)
	if err != nil {
		return SettledSet{}, err
	}
	set := SettledSet{byItem: make(map[string]map[string]struct{})}
	for _, m := range matches {
		if !m.Settled() {
			continue
		}
		if set.byItem[m.ItemID] == nil {
			set.byItem[m.ItemID] = make(map[string]struct{})
		}
		set.byItem[m.ItemID][occurrenceKey(m.Occurrence)] = struct{}{}
	}
	return set, nil
}

// ClearOccurrence records the user's assertion that nothing satisfied this
// occurrence: it stays on the timeline, and resolution leaves it alone.
//
// This is a decision, not the removal of one. Deleting the row instead would
// leave the occurrence indistinguishable from one nothing had been decided
// about, and the next pass would re-make the very match being rejected.
func (s *Service) ClearOccurrence(ctx contextx.ContextX, itemID string, occurrence time.Time) error {
	return s.repo().UpsertManualMatch(ctx, Match{ItemID: itemID, Occurrence: occurrence})
}

// RecordAutoMatches writes best-effort resolution's decisions, never disturbing
// a manual one. Each is applied independently: one occurrence the user has
// already decided must not stop the rest of a pass being recorded.
func (s *Service) RecordAutoMatches(ctx contextx.ContextX, decisions []Match) error {
	for _, d := range decisions {
		if err := s.repo().UpsertAutoMatch(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// Matches returns every decision recorded for an occurrence inside the window,
// inclusive — what the management surface renders beside each item.
func (s *Service) Matches(ctx contextx.ContextX, from, to time.Time) ([]Match, error) {
	return s.repo().MatchesInRange(ctx, from, to)
}
