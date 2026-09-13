package sweep

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/schedule"
)

// Service computes the cash-sweep Recommendation from live account balances and
// the declared schedule, and appends each run to an immutable snapshot history
// other surfaces read without re-computing.
type Service struct {
	accounts *accounts.Service
	schedule *schedule.Service
	db       *db.DB
	location *time.Location
	margin   float64
	now      func() time.Time
}

// NewService builds a sweep Service. margin is the fixed safety margin in
// dollars (typically loaded from cfg.FixedSafetyMargin); location is the app
// timezone the timeline's calendar dates are reckoned in.
func NewService(
	accountsSvc *accounts.Service,
	scheduleSvc *schedule.Service,
	database *db.DB,
	location *time.Location,
	margin float64,
) *Service {
	return &Service{
		accounts: accountsSvc,
		schedule: scheduleSvc,
		db:       database,
		location: location,
		margin:   margin,
		now:      time.Now,
	}
}

// repo binds a Repo to the global (non-transactional) query handle.
func (s *Service) repo() *Repo {
	return NewRepo(s.db.Queries())
}

// Save appends rec to the snapshot history. Never replaces a previous snapshot,
// so calling it repeatedly accumulates rather than overwrites.
func (s *Service) Save(ctx contextx.ContextX, rec Recommendation) error {
	if err := s.repo().Save(ctx, rec); err != nil {
		return fmt.Errorf("sweep: save: %w", err)
	}
	return nil
}

// Run computes a recommendation and appends it as a snapshot, returning what was
// stored. It is the one path that produces a snapshot: the monthly job and the
// user's on-demand action both call it, and neither can tell the other apart.
func (s *Service) Run(ctx contextx.ContextX) (Recommendation, error) {
	rec, err := s.Compute(ctx)
	if err != nil {
		return Recommendation{}, err
	}
	rec.ID = uuid.NewString()
	if err := s.Save(ctx, rec); err != nil {
		return Recommendation{}, err
	}
	return rec, nil
}

// Snapshot returns one stored snapshot positioned in the history, so a caller
// can render it and offer the steps away from it. An empty id selects the newest
// snapshot, which is what a plain page load wants; found is false for an empty
// history and for an id that is not in it.
//
// Instants come back in the configured app timezone ([ADR-0004]): a snapshot's
// label and its timeline's dates are read by the one person the app belongs to,
// in the zone the rest of the app reckons months and schedules in.
func (s *Service) Snapshot(ctx contextx.ContextX, id string) (Snapshot, bool, error) {
	all, err := s.List(ctx)
	if err != nil {
		return Snapshot{}, false, err
	}
	snap, found := neighbors(all, id)
	if !found {
		return Snapshot{}, false, nil
	}
	snap.Recommendation = localized(snap.Recommendation, s.location)
	return snap, true, nil
}

// List returns every stored snapshot, newest first.
func (s *Service) List(ctx contextx.ContextX) ([]Recommendation, error) {
	recs, err := s.repo().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("sweep: list: %w", err)
	}
	return recs, nil
}

// localized moves a snapshot's instants into loc for display. The timeline's
// dates go with it: an event's calendar date is what the reader is looking at,
// and a UTC-rendered midnight would show the wrong day.
func localized(rec Recommendation, loc *time.Location) Recommendation {
	rec.ComputedAt = rec.ComputedAt.In(loc)
	for i := range rec.Timeline {
		rec.Timeline[i].Date = rec.Timeline[i].Date.In(loc)
	}
	return rec
}

// Compute reads live account balances and the declared schedule, builds the
// cash-flow timeline, and produces a Recommendation. The result is numeric
// whenever every dollar value it needs is known and fresh; otherwise it is a
// needs-attention result naming every reason.
func (s *Service) Compute(ctx contextx.ContextX) (Recommendation, error) {
	// The clock is read exactly once per run and threaded through everything
	// below. A snapshot's stamped instant, the horizon it projects to, and the
	// balance-freshness check must all be the same moment, or the snapshot
	// describes a state of the world that never existed.
	//
	// It is read in the app timezone because the timeline is reckoned in calendar
	// dates: which day "the 1st" is, and whether an occurrence has passed, are
	// questions only a zone can answer ([ADR-0004]).
	now := s.now().In(s.location)

	cashAccounts, err := s.accounts.ActiveCashAccounts(ctx)
	if err != nil {
		return Recommendation{}, fmt.Errorf("sweep: failed to list active cash accounts: %w", err)
	}
	creditAccounts, err := s.accounts.ActiveCreditAccounts(ctx)
	if err != nil {
		return Recommendation{}, fmt.Errorf("sweep: failed to list active credit accounts: %w", err)
	}
	items, err := s.schedule.ActiveItems(ctx)
	if err != nil {
		return Recommendation{}, fmt.Errorf("sweep: failed to load the schedule: %w", err)
	}

	in := deriveAccounts(cashAccounts, creditAccounts, now)
	in.items = items
	in.fixedSafetyMargin = s.margin

	return compute(in, now), nil
}

// deriveAccounts resolves the active account lists into the figures the
// computation needs, applying the determination and freshness rules. It is a
// pure function; all I/O is resolved by the caller before calling it.
//
// Checking rule: exactly one active cash account with CountsAsSavings=false,
// whose balance is known and fresh. Each failure is reported as its own reason —
// designating an account, getting a bank to report a balance, and getting a sync
// working are three different fixes, and naming them apart is what tells the
// user which one they have.
//
// Savings rule: savings is not a term in the formula, so nothing about it
// blocks. Anything short of one active counts-as-savings account with a known,
// fresh balance simply reads as unknown.
//
// Cards: every active one counts, with no single-account requirement — debt is
// additive, so no number of cards is ambiguous. Each contributes its own
// timeline row, and a card we cannot stand behind blocks rather than being
// silently read as nothing owed.
func deriveAccounts(cashAccounts, creditAccounts []accounts.Account, now time.Time) computeInput {
	var checkingAccounts, savingsAccounts []accounts.Account
	for _, a := range cashAccounts {
		if a.CountsAsSavings {
			savingsAccounts = append(savingsAccounts, a)
		} else {
			checkingAccounts = append(checkingAccounts, a)
		}
	}

	var in computeInput

	switch {
	case len(checkingAccounts) != 1:
		in.checkingUndetermined = true
	case !checkingAccounts[0].Balance.Known:
		in.checkingBalanceUnknown = true
	case checkingAccounts[0].BalanceStale(now):
		in.checkingStale = true
	default:
		bal := checkingAccounts[0].Balance.Money.Amount
		in.checking = &bal
	}

	if len(savingsAccounts) == 1 {
		sav := savingsAccounts[0]
		if sav.Balance.Known && !sav.BalanceStale(now) {
			bal := sav.Balance.Money.Amount
			in.savingsBalance = &bal
		}
	}

	for _, c := range creditAccounts {
		switch {
		case !c.Balance.Known:
			in.cardBalanceUnknown = true
		case c.BalanceStale(now):
			in.cardBalanceStale = true
		default:
			in.cards = append(in.cards, cardObligation{
				label:     c.DisplayName(),
				balance:   c.Balance.Money.Amount,
				statement: statementFor(c),
			})
		}
	}

	return in
}

// statementFor resolves a card's stored billing-cycle detail into the figure
// and date the timeline needs, or nil when either is unreported.
//
// Both are required together: a billed figure with no resolvable payment date
// cannot be placed, and a date with no figure says nothing about how much. In
// either case nil sends the card down the worst-case path — the whole balance
// at the run instant — which is the conservative reading and the reason a bank
// that reports nothing needs no special case.
//
// The date comes from accounts, which owns the payment schedule; asking rather
// than reimplementing keeps one definition of when a card is paid.
func statementFor(c accounts.Account) *cardStatement {
	if c.Statement == nil || c.Statement.Balance == nil {
		return nil
	}
	due, ok := c.PaymentDate()
	if !ok {
		return nil
	}
	return &cardStatement{balance: *c.Statement.Balance, due: due}
}
