package accounts

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// A connection that fails to sync without ever reaching needs-reconnect is
// exactly the case the reconnect badge misses, so the two flags must be able to
// disagree — the row carries both facts and lets the view choose.
func TestDashboardMarksStaleIndependentlyOfNeedsReconnect(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 500)),
	}}
	svc := NewService(database, provider, testKey)

	conn, err := svc.RegisterConnection(ctx, "tok", "item-123")
	if err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	t.Run("a freshly synced account is not stale", func(t *testing.T) {
		dash, err := svc.Dashboard(ctx)
		if err != nil {
			t.Fatalf("Dashboard: %v", err)
		}
		if len(dash.Cash) != 1 {
			t.Fatalf("cash rows = %d, want 1", len(dash.Cash))
		}
		if dash.Cash[0].Stale {
			t.Error("an account synced moments ago must not be marked stale")
		}
	})

	t.Run("an account whose sync stopped goes stale while the connection stays active", func(t *testing.T) {
		// Back-date the last sync past the threshold, leaving the connection
		// active — the shape of a provider error we cannot classify.
		stale := time.Now().Add(-(staleAfter + time.Hour))
		if _, err := database.Sql().Exec(
			"UPDATE accounts SET last_synced_at = ? WHERE connection_id = ?", stale, conn.ID,
		); err != nil {
			t.Fatalf("back-date last_synced_at: %v", err)
		}

		dash, err := svc.Dashboard(ctx)
		if err != nil {
			t.Fatalf("Dashboard: %v", err)
		}
		if len(dash.Cash) != 1 {
			t.Fatalf("cash rows = %d, want 1", len(dash.Cash))
		}
		row := dash.Cash[0]
		if !row.Stale {
			t.Error("an account that stopped syncing must be marked stale")
		}
		if row.NeedsReconnect {
			t.Error("staleness must not imply needs-reconnect; the connection is still active")
		}
		if row.LastSyncedAt == nil {
			t.Error("the row must carry when it was last refreshed so the view can say how old it is")
		}
	})
}
