package accounts

import (
	"testing"
	"time"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/db"
)

// TestEarliestConnectedAt registers several connections, stamps each with a
// distinct created_at, and asserts EarliestConnectedAt returns the earliest of
// them. It also asserts the no-connection case reports (zero, false) rather than
// an error, so the wrap's partial flag treats "no connections" as not-partial.
func TestEarliestConnectedAt(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{accounts: []banking.Account{
		providerAccount("p-check", "Checking", banking.KindCash, false, knownBalance("p-check", 100)),
	}}
	svc := NewService(database, provider, testKey)

	t.Run("false when there are no connections", func(t *testing.T) {
		_, ok, err := svc.EarliestConnectedAt(ctx)
		if err != nil {
			t.Fatalf("EarliestConnectedAt: %v", err)
		}
		if ok {
			t.Errorf("ok = true with no connections, want false")
		}
	})

	// Register three connections, then stamp distinct created_at values so the
	// "earliest" is unambiguous (CURRENT_TIMESTAMP alone is second-granular and
	// would tie when registered back to back).
	first := registerConn(t, svc, "tok-1", "item-1")
	second := registerConn(t, svc, "tok-2", "item-2")
	third := registerConn(t, svc, "tok-3", "item-3")

	want := time.Date(2026, time.January, 5, 8, 30, 0, 0, time.UTC)
	stampCreatedAt(t, database, second, want) // the earliest
	stampCreatedAt(t, database, first, time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC))
	stampCreatedAt(t, database, third, time.Date(2026, time.June, 14, 0, 0, 0, 0, time.UTC))

	t.Run("returns the earliest connection's created time", func(t *testing.T) {
		got, ok, err := svc.EarliestConnectedAt(ctx)
		if err != nil {
			t.Fatalf("EarliestConnectedAt: %v", err)
		}
		if !ok {
			t.Fatalf("ok = false with connections present, want true")
		}
		if !got.Equal(want) {
			t.Errorf("earliest = %s, want %s", got, want)
		}
	})
}

func registerConn(t *testing.T, svc *Service, token, itemID string) string {
	t.Helper()
	conn, err := svc.RegisterConnection(testCtx(), token, itemID)
	if err != nil {
		t.Fatalf("RegisterConnection(%s): %v", itemID, err)
	}
	return conn.ID
}

// stampCreatedAt overwrites a connection's created_at directly so a test can set
// distinct, ordered registration times (CURRENT_TIMESTAMP alone is second-granular
// and ties when connections are registered back to back).
func stampCreatedAt(t *testing.T, database *db.DB, id string, at time.Time) {
	t.Helper()
	if _, err := database.Sql().Exec("UPDATE connections SET created_at = ? WHERE id = ?", at, id); err != nil {
		t.Fatalf("stamp created_at: %v", err)
	}
}
