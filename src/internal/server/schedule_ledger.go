package server

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// scheduleLedger is the composition root's adapter behind schedule.Ledger. It is
// the only code holding both accounts and transactions, which is what lets the
// schedule reconcile its declarations against the ledger while importing
// neither ([ADR-0027]).
//
// It translates into schedule's own Candidate rather than handing over a
// transactions read model, so nothing of this module's vocabulary crosses the
// port.
type scheduleLedger struct {
	accounts     *accounts.Service
	transactions *transactions.Service
}

// CheckingCandidates returns the checking transactions dated in [from, to].
//
// Scoping to checking is this adapter's job and it asks accounts rather than
// re-deriving the rule, because counts-as-savings is that module's flag. An
// undetermined checking account yields no candidates: nothing can be matched,
// every occurrence stays on the timeline, and the sweep names the fix.
func (l scheduleLedger) CheckingCandidates(ctx contextx.ContextX, from, to time.Time) ([]schedule.Candidate, error) {
	checkingID, determined, err := l.accounts.CheckingAccountID(ctx)
	if err != nil {
		return nil, err
	}
	if !determined {
		return nil, nil
	}

	// The ledger read is half-open; an occurrence window includes its last day.
	rows, err := l.transactions.MonthTransactions(ctx, from, to.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}

	out := make([]schedule.Candidate, 0, len(rows))
	for _, row := range rows {
		if row.AccountID != checkingID {
			continue
		}
		out = append(out, candidateFrom(row))
	}
	return out, nil
}

// TransactionsByID returns the given transactions, skipping any the ledger no
// longer holds — the absence a stored match is dropped on.
func (l scheduleLedger) TransactionsByID(ctx contextx.ContextX, ids []string) ([]schedule.Candidate, error) {
	rows, err := l.transactions.TransactionsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]schedule.Candidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, candidateFrom(row))
	}
	return out, nil
}

// candidateFrom narrows a ledger row to what matching reasons about. Amount
// keeps the app-wide outflow-positive convention, so direction crosses the port
// as a sign rather than as a vocabulary.
func candidateFrom(row transactions.RecentTransaction) schedule.Candidate {
	return schedule.Candidate{
		TransactionID: row.ID,
		Date:          row.Date,
		Amount:        row.Amount.Amount,
		Merchant:      row.Merchant,
	}
}
