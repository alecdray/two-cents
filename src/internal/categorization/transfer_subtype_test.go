package categorization

import (
	"testing"
	"time"
)

// baseDate is the outflow leg's date the pairing window is measured from.
var baseDate = time.Date(2026, time.June, 4, 0, 0, 0, 0, time.UTC)

// inflowLeg builds a candidate inflow leg on the given account.
func inflowLeg(txnID, accountID string, cents int64, date time.Time, countsAsSavings bool) TransferLeg {
	return TransferLeg{
		TransactionID:   txnID,
		AccountID:       accountID,
		AmountCents:     cents,
		Date:            date,
		CountsAsSavings: countsAsSavings,
	}
}

// TestResolveTransferSubtype walks the full pairing matrix: a single match into a
// savings vs. a non-savings destination, the conservative unknown outcomes (zero
// and ambiguous matches), the exact-amount and inclusive ±window boundaries, the
// same-account exclusion, and order-independence.
func TestResolveTransferSubtype(t *testing.T) {
	const source = "acct-checking"

	t.Run("single matching inflow into a savings account is a savings contribution", func(t *testing.T) {
		got := ResolveTransferSubtype(TransferSubtypeInput{
			SourceAccountID: source,
			AmountCents:     50000,
			Date:            baseDate,
			Candidates:      []TransferLeg{inflowLeg("in-1", "acct-savings", 50000, baseDate, true)},
			WindowDays:      3,
		})
		if got.DestinationAccountID == nil || *got.DestinationAccountID != "acct-savings" {
			t.Fatalf("destination: got %v, want acct-savings", got.DestinationAccountID)
		}
		if got.Subtype != SubtypeSavingsContribution {
			t.Fatalf("subtype: got %q, want %q", got.Subtype, SubtypeSavingsContribution)
		}
	})

	t.Run("single matching inflow into a non-savings account is a plain transfer", func(t *testing.T) {
		got := ResolveTransferSubtype(TransferSubtypeInput{
			SourceAccountID: source,
			AmountCents:     50000,
			Date:            baseDate,
			Candidates:      []TransferLeg{inflowLeg("in-1", "acct-other", 50000, baseDate, false)},
			WindowDays:      3,
		})
		if got.DestinationAccountID == nil || *got.DestinationAccountID != "acct-other" {
			t.Fatalf("destination: got %v, want acct-other", got.DestinationAccountID)
		}
		if got.Subtype != SubtypePlain {
			t.Fatalf("subtype: got %q, want %q", got.Subtype, SubtypePlain)
		}
	})

	t.Run("single matching inflow into a credit destination is a plain transfer", func(t *testing.T) {
		// A credit-card-payment destination is modelled simply as CountsAsSavings
		// false (the account kind is not an engine input), so it resolves to plain.
		got := ResolveTransferSubtype(TransferSubtypeInput{
			SourceAccountID: source,
			AmountCents:     50000,
			Date:            baseDate,
			Candidates:      []TransferLeg{inflowLeg("in-1", "acct-credit", 50000, baseDate, false)},
			WindowDays:      3,
		})
		if got.DestinationAccountID == nil || *got.DestinationAccountID != "acct-credit" {
			t.Fatalf("destination: got %v, want acct-credit", got.DestinationAccountID)
		}
		if got.Subtype != SubtypePlain {
			t.Fatalf("subtype: got %q, want %q", got.Subtype, SubtypePlain)
		}
	})

	t.Run("no matching inflow leaves the destination unknown and plain", func(t *testing.T) {
		got := ResolveTransferSubtype(TransferSubtypeInput{
			SourceAccountID: source,
			AmountCents:     50000,
			Date:            baseDate,
			Candidates:      []TransferLeg{inflowLeg("in-1", "acct-savings", 12345, baseDate, true)},
			WindowDays:      3,
		})
		if got.DestinationAccountID != nil {
			t.Fatalf("destination: got %v, want nil (unknown)", *got.DestinationAccountID)
		}
		if got.Subtype != SubtypePlain {
			t.Fatalf("subtype: got %q, want %q", got.Subtype, SubtypePlain)
		}
	})

	t.Run("more than one matching inflow is never guessed and stays unknown", func(t *testing.T) {
		got := ResolveTransferSubtype(TransferSubtypeInput{
			SourceAccountID: source,
			AmountCents:     50000,
			Date:            baseDate,
			Candidates: []TransferLeg{
				inflowLeg("in-1", "acct-savings", 50000, baseDate, true),
				inflowLeg("in-2", "acct-other", 50000, baseDate, false),
			},
			WindowDays: 3,
		})
		if got.DestinationAccountID != nil {
			t.Fatalf("destination: got %v, want nil (ambiguous)", *got.DestinationAccountID)
		}
		if got.Subtype != SubtypePlain {
			t.Fatalf("subtype: got %q, want %q", got.Subtype, SubtypePlain)
		}
	})

	t.Run("an amount mismatch by one cent does not match", func(t *testing.T) {
		got := ResolveTransferSubtype(TransferSubtypeInput{
			SourceAccountID: source,
			AmountCents:     50000,
			Date:            baseDate,
			Candidates:      []TransferLeg{inflowLeg("in-1", "acct-savings", 49999, baseDate, true)},
			WindowDays:      3,
		})
		if got.DestinationAccountID != nil {
			t.Fatalf("destination: got %v, want nil (amount mismatch)", *got.DestinationAccountID)
		}
		if got.Subtype != SubtypePlain {
			t.Fatalf("subtype: got %q, want %q", got.Subtype, SubtypePlain)
		}
	})

	t.Run("a date exactly three days apart matches at the inclusive window edge", func(t *testing.T) {
		for _, delta := range []int{-3, 3} {
			got := ResolveTransferSubtype(TransferSubtypeInput{
				SourceAccountID: source,
				AmountCents:     50000,
				Date:            baseDate,
				Candidates:      []TransferLeg{inflowLeg("in-1", "acct-savings", 50000, baseDate.AddDate(0, 0, delta), true)},
				WindowDays:      3,
			})
			if got.DestinationAccountID == nil || *got.DestinationAccountID != "acct-savings" {
				t.Fatalf("delta %d: destination got %v, want acct-savings", delta, got.DestinationAccountID)
			}
			if got.Subtype != SubtypeSavingsContribution {
				t.Fatalf("delta %d: subtype got %q, want %q", delta, got.Subtype, SubtypeSavingsContribution)
			}
		}
	})

	t.Run("a date four days apart falls outside the window and does not match", func(t *testing.T) {
		for _, delta := range []int{-4, 4} {
			got := ResolveTransferSubtype(TransferSubtypeInput{
				SourceAccountID: source,
				AmountCents:     50000,
				Date:            baseDate,
				Candidates:      []TransferLeg{inflowLeg("in-1", "acct-savings", 50000, baseDate.AddDate(0, 0, delta), true)},
				WindowDays:      3,
			})
			if got.DestinationAccountID != nil {
				t.Fatalf("delta %d: destination got %v, want nil (outside window)", delta, *got.DestinationAccountID)
			}
			if got.Subtype != SubtypePlain {
				t.Fatalf("delta %d: subtype got %q, want %q", delta, got.Subtype, SubtypePlain)
			}
		}
	})

	t.Run("a within-window match holds even when the inflow carries a time component", func(t *testing.T) {
		// Three calendar days apart but with a late time-of-day, so a raw 24h
		// subtraction would round past the window; the calendar-day diff must not.
		inflowDate := time.Date(2026, time.June, 7, 23, 30, 0, 0, time.UTC)
		got := ResolveTransferSubtype(TransferSubtypeInput{
			SourceAccountID: source,
			AmountCents:     50000,
			Date:            baseDate,
			Candidates:      []TransferLeg{inflowLeg("in-1", "acct-savings", 50000, inflowDate, true)},
			WindowDays:      3,
		})
		if got.DestinationAccountID == nil || *got.DestinationAccountID != "acct-savings" {
			t.Fatalf("destination: got %v, want acct-savings", got.DestinationAccountID)
		}
	})

	t.Run("a candidate on the source account is never matched", func(t *testing.T) {
		got := ResolveTransferSubtype(TransferSubtypeInput{
			SourceAccountID: source,
			AmountCents:     50000,
			Date:            baseDate,
			Candidates:      []TransferLeg{inflowLeg("in-1", source, 50000, baseDate, true)},
			WindowDays:      3,
		})
		if got.DestinationAccountID != nil {
			t.Fatalf("destination: got %v, want nil (same account excluded)", *got.DestinationAccountID)
		}
		if got.Subtype != SubtypePlain {
			t.Fatalf("subtype: got %q, want %q", got.Subtype, SubtypePlain)
		}
	})

	t.Run("the decision is independent of candidate order", func(t *testing.T) {
		// One real match buried among non-matching legs; reversing the slice must
		// not change which leg wins.
		candidates := []TransferLeg{
			inflowLeg("noise-1", "acct-other", 49999, baseDate, false),
			inflowLeg("in-1", "acct-savings", 50000, baseDate, true),
			inflowLeg("noise-2", source, 50000, baseDate, true),
		}
		reversed := []TransferLeg{candidates[2], candidates[1], candidates[0]}

		forward := ResolveTransferSubtype(TransferSubtypeInput{SourceAccountID: source, AmountCents: 50000, Date: baseDate, Candidates: candidates, WindowDays: 3})
		backward := ResolveTransferSubtype(TransferSubtypeInput{SourceAccountID: source, AmountCents: 50000, Date: baseDate, Candidates: reversed, WindowDays: 3})

		if forward.DestinationAccountID == nil || backward.DestinationAccountID == nil {
			t.Fatalf("expected a destination in both orders: forward=%v backward=%v", forward.DestinationAccountID, backward.DestinationAccountID)
		}
		if *forward.DestinationAccountID != *backward.DestinationAccountID || forward.Subtype != backward.Subtype {
			t.Fatalf("order-dependent decision: forward=%+v backward=%+v", forward, backward)
		}
		if *forward.DestinationAccountID != "acct-savings" || forward.Subtype != SubtypeSavingsContribution {
			t.Fatalf("wrong leg won: got %+v", forward)
		}
	})
}

// TestTransferSubtypeAmountCents covers the cents helper callers use to feed the
// engine: magnitude, rounding, and sign-folding of the banking.Money convention.
func TestTransferSubtypeAmountCents(t *testing.T) {
	cases := []struct {
		name   string
		amount float64
		want   int64
	}{
		{"a positive outflow", 500.00, 50000},
		{"a negative inflow folds to its magnitude", -500.00, 50000},
		{"a fractional amount rounds to the nearest cent", 12.345, 1235},
		{"zero is zero", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AmountCents(tc.amount); got != tc.want {
				t.Fatalf("AmountCents(%v): got %d, want %d", tc.amount, got, tc.want)
			}
		})
	}
}
