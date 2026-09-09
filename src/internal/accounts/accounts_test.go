package accounts

import (
	"testing"
	"time"
)

// BalanceStale is the one definition of "this balance is too old to trust",
// exported so callers outside this package (the sweep) ask the same question
// the overview does instead of carrying a second threshold.
func TestAccountBalanceStale(t *testing.T) {
	now := time.Date(2026, time.September, 8, 18, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time {
		t := now.Add(-d)
		return &t
	}

	cases := []struct {
		name         string
		lastSyncedAt *time.Time
		want         bool
	}{
		{"a balance refreshed on the last pass is current", ago(1 * time.Hour), false},
		{"a few missed passes are tolerated", ago(20 * time.Hour), false},
		{"exactly at the threshold is still current", ago(staleAfter), false},
		{"past the threshold is stale", ago(staleAfter + time.Minute), true},
		{"a long-failing connection is stale", ago(9 * 24 * time.Hour), true},
		{"a never-synced account is stale", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Account{LastSyncedAt: tc.lastSyncedAt}
			if got := a.BalanceStale(now); got != tc.want {
				t.Errorf("BalanceStale() = %v, want %v", got, tc.want)
			}
		})
	}
}
