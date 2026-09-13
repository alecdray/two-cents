package plaid

import (
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// liabilitiesResponse is /liabilities/get. Only the credit array is decoded:
// the student and mortgage arrays and every APR field are left undecoded on
// purpose, which is what keeps the narrowed liabilities non-goal narrow
// ([ADR-0024]) rather than reopened by a field nobody asked for.
type liabilitiesResponse struct {
	Liabilities struct {
		Credit []creditLiability `json:"credit"`
	} `json:"liabilities"`
}

// creditLiability is one credit card's billing-cycle facts. Every figure is a
// pointer because Plaid reports null for a card it has no cycle data for, and
// that distinction has to survive into the domain.
type creditLiability struct {
	AccountID              string   `json:"account_id"`
	LastStatementBalance   *float64 `json:"last_statement_balance"`
	LastStatementIssueDate *string  `json:"last_statement_issue_date"`
	NextPaymentDueDate     *string  `json:"next_payment_due_date"`
}

// toCardStatements maps the credit array onto the seam's shape. A card with no
// billed figure is Known false rather than a zero balance — downstream that is
// the difference between "nothing is due" and "we do not know", and only the
// second degrades to the safe worst case.
func (r liabilitiesResponse) toCardStatements() []banking.CardStatement {
	out := make([]banking.CardStatement, 0, len(r.Liabilities.Credit))
	for _, c := range r.Liabilities.Credit {
		statement := banking.CardStatement{
			AccountID: c.AccountID,
			IssuedAt:  parseLiabilityDate(c.LastStatementIssueDate),
			DueAt:     parseLiabilityDate(c.NextPaymentDueDate),
		}
		if c.LastStatementBalance != nil {
			statement.Known = true
			statement.Balance = banking.Money{Amount: *c.LastStatementBalance, Currency: "USD"}
		}
		out = append(out, statement)
	}
	return out
}

// parseLiabilityDate reads Plaid's plain YYYY-MM-DD day. An absent or
// unparseable day stays nil: a guessed date would be indistinguishable from a
// reported one.
func parseLiabilityDate(raw *string) *time.Time {
	if raw == nil || *raw == "" {
		return nil
	}
	day, err := time.Parse(dateLayout, *raw)
	if err != nil {
		return nil
	}
	return &day
}
