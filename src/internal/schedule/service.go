package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// ValidationError is a recoverable, user-facing input error (a nameless item, a
// cadence with no date to go with it). Adapters surface its Message inline
// rather than treating it as a server failure, the same shape budget uses.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// maxNameLen bounds a declared item's name so a stray paste cannot make the
// schedule unreadable. It matches the account custom-name bound.
const maxNameLen = 60

// Service owns the sweep schedule — the declared recurring checking activity the
// cash-flow timeline is built from. It writes only its own tables, and reads the
// ledger only through the injected Ledger port, in its own vocabulary, which is
// what lets it reconcile declarations against reality while remaining an import
// leaf ([ADR-0027]).
type Service struct {
	db *db.DB
	// ledger is the port automatic resolution reads candidates through. It may be
	// nil — a Service built without one simply resolves nothing, which is what the
	// surfaces that only read the declared schedule need.
	ledger   Ledger
	location *time.Location
	now      func() time.Time
}

// NewService builds a schedule Service over the database, the ledger port
// automatic resolution reconciles against (nil to resolve nothing), and the app
// timezone occurrences are dated in.
func NewService(d *db.DB, ledger Ledger, location *time.Location) *Service {
	return &Service{
		db:       d,
		ledger:   ledger,
		location: location,
		now:      time.Now,
	}
}

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}

// List returns the whole declared schedule, inactive items included — what the
// management surface renders.
func (s *Service) List(ctx contextx.ContextX) ([]Item, error) {
	items, err := s.repo().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("schedule: list: %w", err)
	}
	return items, nil
}

// ActiveItems returns only the items that currently belong on the timeline. It
// is the read seam the sweep consumes: an inactive item is kept in storage but
// contributes nothing, so deactivating is how a user retires a commitment
// without losing what they declared.
func (s *Service) ActiveItems(ctx contextx.ContextX) ([]Item, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	active := make([]Item, 0, len(all))
	for _, item := range all {
		if item.Active {
			active = append(active, item)
		}
	}
	return active, nil
}

// Create validates and stores a new Item, returning it with the id it was
// assigned. An invalid declaration is a ValidationError and nothing is stored.
func (s *Service) Create(ctx contextx.ContextX, item Item) (Item, error) {
	normalized, err := normalize(item)
	if err != nil {
		return Item{}, err
	}
	normalized.ID = uuid.NewString()
	if err := s.repo().Insert(ctx, normalized); err != nil {
		return Item{}, fmt.Errorf("schedule: create: %w", err)
	}
	return normalized, nil
}

// Update validates and overwrites the stored Item with the same id. As with
// Create, an invalid declaration changes nothing.
func (s *Service) Update(ctx contextx.ContextX, item Item) error {
	normalized, err := normalize(item)
	if err != nil {
		return err
	}
	if item.ID == "" {
		return ValidationError{Message: "That scheduled item can't be found."}
	}
	normalized.ID = item.ID
	if err := s.repo().Update(ctx, normalized); err != nil {
		return fmt.Errorf("schedule: update: %w", err)
	}
	return nil
}

// Delete removes the Item with the given id from the schedule, along with every
// decision recorded against its occurrences.
//
// The decisions go first, and explicitly. The table declares ON DELETE CASCADE,
// but foreign keys are not enabled on this connection (docs/architecture/known-gaps.md),
// so the cascade never fires and an orphaned decision would outlive the item it
// was about - and be handed back by any window query that overlapped it.
func (s *Service) Delete(ctx contextx.ContextX, id string) error {
	if err := s.repo().DeleteMatchesForItem(ctx, id); err != nil {
		return fmt.Errorf("schedule: delete occurrence decisions: %w", err)
	}
	if err := s.repo().Delete(ctx, id); err != nil {
		return fmt.Errorf("schedule: delete: %w", err)
	}
	return nil
}

// normalize trims the name and validates the declaration, returning the Item as
// it should be stored. The date fields are cleared for the cadence that does not
// use them, so a stored row never describes a cadence it has no date for.
//
// The amount is required to be positive: direction carries the sign, and a
// negative amount paired with a direction would let one declaration mean two
// opposite things.
func normalize(item Item) (Item, error) {
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		return Item{}, ValidationError{Message: "Give the scheduled item a name."}
	}
	if len(item.Name) > maxNameLen {
		return Item{}, ValidationError{Message: fmt.Sprintf("Keep the name to %d characters or fewer.", maxNameLen)}
	}
	if item.Direction != DirectionOut && item.Direction != DirectionIn {
		return Item{}, ValidationError{Message: "Choose whether the money goes out or comes in."}
	}
	if item.Amount <= 0 {
		return Item{}, ValidationError{Message: "Enter an amount greater than zero."}
	}

	switch item.Cadence {
	case CadenceMonthly:
		if item.DayOfMonth < 1 || item.DayOfMonth > 31 {
			return Item{}, ValidationError{Message: "Choose a day of the month between 1 and 31."}
		}
		item.AnchorDate = time.Time{}
	case CadenceBiweekly:
		if item.AnchorDate.IsZero() {
			return Item{}, ValidationError{Message: "Give a date the item last fell on, so the every-14-days cadence has an anchor."}
		}
		item.DayOfMonth = 0
	default:
		return Item{}, ValidationError{Message: "Choose a monthly or every-two-weeks cadence."}
	}

	return item, nil
}

// IsValidationError reports whether err is (or wraps) a ValidationError, so
// adapters can render its message inline instead of returning a server error.
func IsValidationError(err error) (ValidationError, bool) {
	var ve ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return ValidationError{}, false
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

// ConfirmOccurrence promotes the standing decision about an occurrence to the
// user's own, freezing the transaction it already points at.
//
// It is how a guess becomes a decision: an automatic match is re-resolved from
// scratch every pass and freely replaced, so one the user has looked at and
// accepted needs saying so, or the next better-scoring candidate takes its
// place. Confirming an occurrence nothing settles is a no-op — there is nothing
// to freeze, and writing an empty decision would be a clear, which is a
// different assertion entirely.
func (s *Service) ConfirmOccurrence(ctx contextx.ContextX, itemID string, occurrence time.Time) error {
	matches, err := s.repo().MatchesInRange(ctx, occurrence, occurrence)
	if err != nil {
		return err
	}
	for _, m := range matches {
		if m.ItemID != itemID || !m.Settled() {
			continue
		}
		return s.MatchOccurrence(ctx, itemID, occurrence, m.TransactionID)
	}
	return nil
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

	// A transaction settles at most one obligation, so the claimed set starts
	// from every decision already stored - not just this pass's - because the
	// constraint is global and the alternative is discovering it as a failed
	// write partway through the pass.
	//
	// It records *which* occurrence holds each transaction, not merely that one
	// does. An occurrence re-scoring its own standing match must still see that
	// match as a candidate: excluded from its own candidate set, it would hand
	// the decision to whatever else was admissible and flip back the next pass,
	// which is the opposite of "a better candidate supersedes a worse one".
	settled, err := s.repo().SettledMatches(ctx)
	if err != nil {
		return err
	}
	claimedBy := make(map[string]string, len(settled))
	for _, m := range settled {
		claimedBy[m.TransactionID] = occurrenceHolder(m.ItemID, m.Occurrence)
	}

	var decisions []Match
	for _, item := range items {
		from := item.OneCadenceBefore(today)
		candidates, err := s.ledger.CheckingCandidates(ctx, from.AddDate(0, 0, -matchDateToleranceDays), today.AddDate(0, 0, matchDateToleranceDays))
		if err != nil {
			return err
		}
		for _, occurrence := range item.Occurrences(from, today) {
			holder := occurrenceHolder(item.ID, occurrence)
			winner, ok := bestCandidate(item, occurrence, candidates, learned[item.ID], holder, claimedBy)
			if !ok {
				continue
			}
			claimedBy[winner.TransactionID] = holder
			decisions = append(decisions, Match{
				ItemID:        item.ID,
				Occurrence:    occurrence,
				TransactionID: winner.TransactionID,
			})
		}
	}
	return s.RecordAutoMatches(ctx, decisions)
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

func (s *Service) offerContext(ctx contextx.ContextX, items []Item, today time.Time) (offerContext, error) {
	if s.ledger == nil {
		return offerContext{}, nil
	}

	from := earliestLookback(items, today).AddDate(0, 0, -matchDateToleranceDays)
	rows, err := s.ledger.CheckingCandidates(ctx, from, today.AddDate(0, 0, matchDateToleranceDays))
	if err != nil {
		return offerContext{}, err
	}

	settled, err := s.repo().SettledMatches(ctx)
	if err != nil {
		return offerContext{}, err
	}
	claimed := make(map[string]bool, len(settled))
	for _, m := range settled {
		claimed[m.TransactionID] = true
	}

	learned, err := s.learnedMerchants(ctx, items)
	if err != nil {
		return offerContext{}, err
	}
	return offerContext{rows: rows, claimed: claimed, learned: learned}, nil
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
