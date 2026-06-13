package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/core/db/sqlc"

	"github.com/pressly/goose/v3"

	_ "github.com/mattn/go-sqlite3"
)

// newTestDB opens a fresh on-disk sqlite DB in a temp dir, applies every
// migration, and wraps it with WrapSqlDB (which skips its own migration run).
func newTestDB(t *testing.T) *db.DB {
	t.Helper()

	migrationsDir, err := filepath.Abs("../../../../db/migrations")
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	return db.WrapSqlDB(sqlDB)
}

// A connection written inside WithTx commits as one unit and reads back with
// every field intact, including the stored (encrypted-string) token.
func TestConnectionSurvivesTransactionRoundTrip(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	const (
		id        = "conn-1"
		itemID    = "item-abc"
		token     = "gAAAAA.stored-cipher-text"
		stateWant = "active"
	)

	err := database.WithTx(func(tx *db.DB) error {
		_, err := tx.Queries().CreateConnection(ctx, sqlc.CreateConnectionParams{
			ID:          id,
			ItemID:      itemID,
			AccessToken: token,
			State:       stateWant,
		})
		return err
	})
	if err != nil {
		t.Fatalf("WithTx create connection: %v", err)
	}

	got, err := database.Queries().GetConnection(ctx, id)
	if err != nil {
		t.Fatalf("read back connection: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.ItemID != itemID {
		t.Errorf("item id = %q, want %q", got.ItemID, itemID)
	}
	if got.State != stateWant {
		t.Errorf("state = %q, want %q", got.State, stateWant)
	}
	if got.AccessToken != token {
		t.Errorf("access token = %q, want %q", got.AccessToken, token)
	}
}

// A failing transaction leaves nothing behind: the write is rolled back as a
// unit when fn returns an error.
func TestFailedTransactionRollsBack(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	sentinel := errEarlyExit
	err := database.WithTx(func(tx *db.DB) error {
		if _, err := tx.Queries().CreateConnection(ctx, sqlc.CreateConnectionParams{
			ID:          "conn-rollback",
			ItemID:      "item-rollback",
			AccessToken: "tok",
			State:       "active",
		}); err != nil {
			return err
		}
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("WithTx error = %v, want sentinel", err)
	}

	if _, err := database.Queries().GetConnection(ctx, "conn-rollback"); err != sql.ErrNoRows {
		t.Fatalf("after rollback, GetConnection err = %v, want sql.ErrNoRows", err)
	}
}

var errEarlyExit = sentinelError("intentional rollback")

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

// An account round-trips with its connection link, descriptive fields, kind,
// counts-as-savings flag, balance, and state intact.
func TestAccountRoundTripPreservesAllFields(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	mustConnection(t, database, "conn-acct", "item-acct")

	want := sqlc.CreateAccountParams{
		ID:                "acct-1",
		ConnectionID:      "conn-acct",
		ProviderAccountID: "plaid-acct-1",
		Name:              "Everyday Checking",
		BankType:          "depository",
		Kind:              "cash",
		KindOverridden:    0,
		CountsAsSavings:   1,
		SavingsOverridden: 1,
		BalanceAmount:     1234.56,
		BalanceCurrency:   "USD",
		BalanceKnown:      1,
		State:             "active",
	}

	if _, err := database.Queries().CreateAccount(ctx, want); err != nil {
		t.Fatalf("create account: %v", err)
	}

	got, err := database.Queries().GetAccount(ctx, want.ID)
	if err != nil {
		t.Fatalf("read back account: %v", err)
	}

	if got.ConnectionID != want.ConnectionID {
		t.Errorf("connection id = %q, want %q", got.ConnectionID, want.ConnectionID)
	}
	if got.Name != want.Name {
		t.Errorf("name = %q, want %q", got.Name, want.Name)
	}
	if got.BankType != want.BankType {
		t.Errorf("bank type = %q, want %q", got.BankType, want.BankType)
	}
	if got.Kind != want.Kind {
		t.Errorf("kind = %q, want %q", got.Kind, want.Kind)
	}
	if got.CountsAsSavings != want.CountsAsSavings {
		t.Errorf("counts as savings = %d, want %d", got.CountsAsSavings, want.CountsAsSavings)
	}
	if got.BalanceAmount != want.BalanceAmount {
		t.Errorf("balance amount = %v, want %v", got.BalanceAmount, want.BalanceAmount)
	}
	if got.BalanceCurrency != want.BalanceCurrency {
		t.Errorf("balance currency = %q, want %q", got.BalanceCurrency, want.BalanceCurrency)
	}
	if got.BalanceKnown != 1 {
		t.Errorf("balance known = %d, want 1", got.BalanceKnown)
	}
	if got.State != want.State {
		t.Errorf("state = %q, want %q", got.State, want.State)
	}
}

// An account stored with an unknown balance reads back as unknown — the
// balance_known flag is 0, distinguishing it from a genuine zero balance.
func TestAccountWithUnknownBalanceReadsBackAsUnknown(t *testing.T) {
	database := newTestDB(t)
	ctx := context.Background()

	mustConnection(t, database, "conn-unknown", "item-unknown")

	if _, err := database.Queries().CreateAccount(ctx, sqlc.CreateAccountParams{
		ID:                "acct-unknown",
		ConnectionID:      "conn-unknown",
		ProviderAccountID: "plaid-acct-unknown",
		Name:              "Mystery Card",
		BankType:          "credit",
		Kind:              "credit",
		BalanceAmount:     0,
		BalanceCurrency:   "",
		BalanceKnown:      0,
		State:             "active",
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}

	got, err := database.Queries().GetAccount(ctx, "acct-unknown")
	if err != nil {
		t.Fatalf("read back account: %v", err)
	}

	if got.BalanceKnown != 0 {
		t.Fatalf("balance known = %d, want 0 (unknown)", got.BalanceKnown)
	}
}

func mustConnection(t *testing.T, database *db.DB, id, itemID string) {
	t.Helper()
	if _, err := database.Queries().CreateConnection(context.Background(), sqlc.CreateConnectionParams{
		ID:          id,
		ItemID:      itemID,
		AccessToken: "tok-" + id,
		State:       "active",
	}); err != nil {
		t.Fatalf("seed connection %q: %v", id, err)
	}
}
