package sweep

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/banking"
)

// Fixtures sit far from the staleness boundary on both sides, so a slow test
// cannot drift one across it.
const (
	freshAgo = 2 * time.Hour
	staleAgo = 72 * time.Hour
)

func cashAccount(name string, countsAsSavings bool, balance float64, known bool, syncedAgo time.Duration, now time.Time) accounts.Account {
	syncedAt := now.Add(-syncedAgo)
	return accounts.Account{
		ID:              name,
		Name:            name,
		Kind:            banking.KindCash,
		State:           accounts.AccountActive,
		CountsAsSavings: countsAsSavings,
		Balance: banking.Balance{
			Known: known,
			Money: banking.Money{Amount: balance},
		},
		LastSyncedAt: &syncedAt,
	}
}

func creditAccount(name string, balance float64, known bool, syncedAgo time.Duration, now time.Time) accounts.Account {
	a := cashAccount(name, false, balance, known, syncedAgo, now)
	a.Kind = banking.KindCredit
	return a
}

func hasReason(reasons []NeedsAttentionReason, want NeedsAttentionReason) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

// computeFrom runs the whole pure path — derivation then computation — the way
// Compute does, so these assert the result the user actually sees rather than an
// intermediate struct.
func computeFrom(cash, credit []accounts.Account, now time.Time) Recommendation {
	in := deriveAccounts(cash, credit, now)
	in.fixedSafetyMargin = 500
	return compute(in, now)
}

func TestDeriveChecking(t *testing.T) {
	loc := appZone(t)
	now := runInstant(loc)

	t.Run("exactly one active non-savings cash account is checking", func(t *testing.T) {
		rec := computeFrom([]accounts.Account{
			cashAccount("Everyday", false, 3000, true, freshAgo, now),
			cashAccount("Rainy Day", true, 8000, true, freshAgo, now),
		}, nil, now)

		if rec.Kind != KindNumeric {
			t.Fatalf("kind = %s with reasons %v, want numeric", rec.Kind, rec.Reasons)
		}
		if rec.CurrentChecking != 3000 {
			t.Errorf("checking = %v, want 3000", rec.CurrentChecking)
		}
		if rec.CurrentSavings != 8000 || rec.SavingsUnknown {
			t.Errorf("savings = %v (unknown=%v), want 8000 known", rec.CurrentSavings, rec.SavingsUnknown)
		}
	})

	t.Run("no checking candidate cannot be derived", func(t *testing.T) {
		rec := computeFrom([]accounts.Account{
			cashAccount("Rainy Day", true, 8000, true, freshAgo, now),
		}, nil, now)

		if !hasReason(rec.Reasons, ReasonCheckingUndetermined) {
			t.Errorf("reasons = %v, want checking undetermined", rec.Reasons)
		}
	})

	t.Run("two checking candidates are ambiguous", func(t *testing.T) {
		rec := computeFrom([]accounts.Account{
			cashAccount("Everyday", false, 3000, true, freshAgo, now),
			cashAccount("Spare", false, 400, true, freshAgo, now),
		}, nil, now)

		if !hasReason(rec.Reasons, ReasonCheckingUndetermined) {
			t.Errorf("reasons = %v, want checking undetermined", rec.Reasons)
		}
	})

	t.Run("an unreported checking balance is its own reason, not ambiguity", func(t *testing.T) {
		// The account is perfectly well identified; getting the bank to report a
		// balance is a different fix from designating an account.
		rec := computeFrom([]accounts.Account{
			cashAccount("Everyday", false, 0, false, freshAgo, now),
		}, nil, now)

		if !hasReason(rec.Reasons, ReasonCheckingBalanceUnknown) {
			t.Errorf("reasons = %v, want the balance-unknown reason", rec.Reasons)
		}
		if hasReason(rec.Reasons, ReasonCheckingUndetermined) {
			t.Error("an identified account must not also read as undetermined")
		}
	})

	t.Run("a stale checking balance blocks", func(t *testing.T) {
		rec := computeFrom([]accounts.Account{
			cashAccount("Everyday", false, 3000, true, staleAgo, now),
		}, nil, now)

		if rec.Kind != KindNeedsAttention || !hasReason(rec.Reasons, ReasonCheckingStale) {
			t.Errorf("kind = %s reasons = %v, want the stale reason", rec.Kind, rec.Reasons)
		}
	})
}

func TestDeriveSavings(t *testing.T) {
	loc := appZone(t)
	now := runInstant(loc)

	// Savings is not a term in the formula, so every way of not knowing it reads
	// as unknown and none of them blocks.
	tests := []struct {
		name string
		cash []accounts.Account
	}{
		{
			name: "no savings account at all",
			cash: nil,
		},
		{
			name: "two savings candidates",
			cash: []accounts.Account{
				cashAccount("Rainy Day", true, 8000, true, freshAgo, now),
				cashAccount("Vacation", true, 2000, true, freshAgo, now),
			},
		},
		{
			name: "an unreported savings balance",
			cash: []accounts.Account{cashAccount("Rainy Day", true, 0, false, freshAgo, now)},
		},
		{
			name: "a stale savings balance",
			cash: []accounts.Account{cashAccount("Rainy Day", true, 8000, true, staleAgo, now)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+" reads as unknown without blocking", func(t *testing.T) {
			cash := append([]accounts.Account{cashAccount("Everyday", false, 3000, true, freshAgo, now)}, tc.cash...)

			rec := computeFrom(cash, nil, now)

			if rec.Kind != KindNumeric {
				t.Fatalf("kind = %s with reasons %v, want numeric — savings never blocks", rec.Kind, rec.Reasons)
			}
			if !rec.SavingsUnknown {
				t.Errorf("SavingsUnknown = false, want true")
			}
		})
	}
}

func TestDeriveCards(t *testing.T) {
	loc := appZone(t)
	now := runInstant(loc)
	checking := []accounts.Account{cashAccount("Everyday", false, 10000, true, freshAgo, now)}

	t.Run("every active card lands its own obligation on the timeline", func(t *testing.T) {
		rec := computeFrom(checking, []accounts.Account{
			creditAccount("Sapphire", 840, true, freshAgo, now),
			creditAccount("Freedom", 260, true, freshAgo, now),
		}, now)

		if rec.Kind != KindNumeric {
			t.Fatalf("kind = %s with reasons %v, want numeric", rec.Kind, rec.Reasons)
		}
		if len(rec.Timeline) != 2 {
			t.Fatalf("timeline = %v, want one event per card", summaries(rec.Timeline))
		}
		if rec.RequiredChecking != 1100 {
			t.Errorf("required = %v, want the 1100 both cards owe", rec.RequiredChecking)
		}
	})

	t.Run("no cards at all is a perfectly good result", func(t *testing.T) {
		rec := computeFrom(checking, nil, now)

		if rec.Kind != KindNumeric {
			t.Fatalf("kind = %s with reasons %v, want numeric", rec.Kind, rec.Reasons)
		}
		if rec.RequiredChecking != 0 {
			t.Errorf("required = %v, want 0", rec.RequiredChecking)
		}
	})

	t.Run("an unreported card balance blocks", func(t *testing.T) {
		rec := computeFrom(checking, []accounts.Account{
			creditAccount("Sapphire", 0, false, freshAgo, now),
		}, now)

		if !hasReason(rec.Reasons, ReasonCardBalanceUnknown) {
			t.Errorf("reasons = %v, want the card-unknown reason", rec.Reasons)
		}
	})

	t.Run("a stale card balance blocks", func(t *testing.T) {
		rec := computeFrom(checking, []accounts.Account{
			creditAccount("Sapphire", 840, true, staleAgo, now),
		}, now)

		if !hasReason(rec.Reasons, ReasonCardBalanceStale) {
			t.Errorf("reasons = %v, want the card-stale reason", rec.Reasons)
		}
	})

	t.Run("a card with no statement detail still produces a result", func(t *testing.T) {
		// No statement detail is held for any card yet, so this is the ordinary
		// path: a known balance with no due date falls due immediately. It is the
		// most conservative reading, and it is not a failure.
		rec := computeFrom(checking, []accounts.Account{
			creditAccount("Sapphire", 840, true, freshAgo, now),
		}, now)

		if rec.Kind != KindNumeric {
			t.Fatalf("kind = %s, want numeric", rec.Kind)
		}
		if !rec.Timeline[0].Date.Equal(now) {
			t.Errorf("card event dated %s, want the run instant", rec.Timeline[0].Date)
		}
	})
}

func TestComputeNeedsAttention(t *testing.T) {
	loc := appZone(t)
	now := runInstant(loc)

	t.Run("every applicable reason is listed, not just the first", func(t *testing.T) {
		rec := computeFrom(
			[]accounts.Account{cashAccount("Everyday", false, 3000, true, staleAgo, now)},
			[]accounts.Account{creditAccount("Sapphire", 0, false, freshAgo, now)},
			now,
		)

		if !hasReason(rec.Reasons, ReasonCheckingStale) || !hasReason(rec.Reasons, ReasonCardBalanceUnknown) {
			t.Errorf("reasons = %v, want both the stale checking and the unknown card", rec.Reasons)
		}
	})

	t.Run("a needs-attention result carries no timeline to explain", func(t *testing.T) {
		rec := computeFrom(nil, nil, now)

		if len(rec.Timeline) != 0 {
			t.Errorf("timeline = %v, want none — there is no number to derive", summaries(rec.Timeline))
		}
	})

	t.Run("a needs-attention result is still stamped with the run instant", func(t *testing.T) {
		rec := computeFrom(nil, nil, now)

		if !rec.ComputedAt.Equal(now) {
			t.Errorf("ComputedAt = %s, want the run instant %s", rec.ComputedAt, now)
		}
	})
}

func TestComputeSuggestedSweep(t *testing.T) {
	loc := appZone(t)
	now := runInstant(loc)

	t.Run("the sweep is what is left after the requirement and the margin", func(t *testing.T) {
		rec := computeFrom(
			[]accounts.Account{cashAccount("Everyday", false, 5000, true, freshAgo, now)},
			[]accounts.Account{creditAccount("Sapphire", 1000, true, freshAgo, now)},
			now,
		)

		if rec.SuggestedSweep != 3500 {
			t.Errorf("sweep = %v, want 5000 − 1000 − 500", rec.SuggestedSweep)
		}
		if rec.Direction != DirectionCheckingToSavings {
			t.Errorf("direction = %s, want checking->savings", rec.Direction)
		}
	})

	t.Run("a shortfall is a negative sweep, not a floor at zero", func(t *testing.T) {
		rec := computeFrom(
			[]accounts.Account{cashAccount("Everyday", false, 400, true, freshAgo, now)},
			[]accounts.Account{creditAccount("Sapphire", 1000, true, freshAgo, now)},
			now,
		)

		if rec.SuggestedSweep != -1100 {
			t.Errorf("sweep = %v, want 400 − 1000 − 500", rec.SuggestedSweep)
		}
		if rec.Direction != DirectionSavingsToChecking {
			t.Errorf("direction = %s, want savings->checking — a pull back is meaningful", rec.Direction)
		}
	})

	t.Run("an exactly-covered checking balance needs no transfer", func(t *testing.T) {
		rec := computeFrom(
			[]accounts.Account{cashAccount("Everyday", false, 1500, true, freshAgo, now)},
			[]accounts.Account{creditAccount("Sapphire", 1000, true, freshAgo, now)},
			now,
		)

		if rec.SuggestedSweep != 0 || rec.Direction != DirectionNone {
			t.Errorf("sweep = %v direction = %s, want 0 / none", rec.SuggestedSweep, rec.Direction)
		}
	})
}
