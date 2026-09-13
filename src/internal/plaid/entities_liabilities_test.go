package plaid

import (
	"encoding/json"
	"testing"
)

// The payload shape mirrors /liabilities/get: a credit array beside the student
// and mortgage arrays we deliberately never decode.
const liabilitiesPayload = `{
  "liabilities": {
    "credit": [
      {
        "account_id": "acc-card",
        "last_statement_balance": 500.25,
        "last_statement_issue_date": "2026-09-03",
        "next_payment_due_date": "2026-09-28",
        "aprs": [{"apr_percentage": 19.99, "apr_type": "purchase_apr"}]
      },
      {
        "account_id": "acc-card-2",
        "last_statement_balance": null,
        "last_statement_issue_date": null,
        "next_payment_due_date": null
      }
    ],
    "student": [{"account_id": "acc-loan", "interest_rate_percentage": 4.5}],
    "mortgage": [{"account_id": "acc-mortgage"}]
  }
}`

func TestLiabilitiesToCardStatements(t *testing.T) {
	var resp liabilitiesResponse
	if err := json.Unmarshal([]byte(liabilitiesPayload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := resp.toCardStatements()

	if len(got) != 2 {
		t.Fatalf("statements = %d, want 2 — only the credit array is read", len(got))
	}

	t.Run("a reported statement carries its billed amount and both dates", func(t *testing.T) {
		s := got[0]
		if s.AccountID != "acc-card" || !s.Known {
			t.Fatalf("statement = %+v, want a known statement for acc-card", s)
		}
		if s.Balance.Amount != 500.25 {
			t.Errorf("billed = %v, want 500.25", s.Balance.Amount)
		}
		if s.IssuedAt == nil || s.IssuedAt.Format("2006-01-02") != "2026-09-03" {
			t.Errorf("issued = %v, want 2026-09-03", s.IssuedAt)
		}
		if s.DueAt == nil || s.DueAt.Format("2006-01-02") != "2026-09-28" {
			t.Errorf("due = %v, want 2026-09-28", s.DueAt)
		}
	})

	t.Run("a card reporting no statement figures is unknown, not zero", func(t *testing.T) {
		s := got[1]
		if s.Known {
			t.Errorf("statement = %+v, want Known false — the bank reported no figures", s)
		}
		if s.IssuedAt != nil || s.DueAt != nil {
			t.Errorf("dates = %v/%v, want both nil", s.IssuedAt, s.DueAt)
		}
	})
}
