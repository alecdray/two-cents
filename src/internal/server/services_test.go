package server

import (
	"database/sql"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/db"
	"github.com/alecdray/two-cents/src/internal/fakebank"

	_ "github.com/mattn/go-sqlite3"
)

// With BankProvider set to "fake", the composition root selects the
// deterministic stand-in — and does so with no Plaid configuration at all,
// proving the fake branch builds no Plaid client.
func TestSelectBankProviderFake(t *testing.T) {
	provider, err := selectBankProvider(app.Config{BankProvider: "fake"})
	if err != nil {
		t.Fatalf("selectBankProvider: %v", err)
	}
	if _, ok := provider.(*fakebank.Service); !ok {
		t.Errorf("selected provider is %T, want *fakebank.Service", provider)
	}
}

// The unset/default provider selects the live Plaid client, not the stand-in.
func TestSelectBankProviderDefaultsToPlaid(t *testing.T) {
	provider, err := selectBankProvider(app.Config{
		Plaid: app.PlaidConfig{ClientID: "test-client-id", Secret: "test-secret", Env: "sandbox"},
	})
	if err != nil {
		t.Fatalf("selectBankProvider: %v", err)
	}
	if _, ok := provider.(*fakebank.Service); ok {
		t.Error("default selected the fake stand-in; want the Plaid provider")
	}
}

// The composition root constructs the accounts service backed by the plaid
// provider through the banking seam when given valid Plaid credentials and a
// hex encryption key. This is a construction smoke — it exercises the wiring
// without binding a port.
func TestNewServicesWiresAccountsOverPlaid(t *testing.T) {
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	database := db.WrapSqlDB(sqlDB)

	application := app.NewApp(app.Config{
		EncryptionKey: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		Plaid: app.PlaidConfig{
			ClientID: "test-client-id",
			Secret:   "test-secret",
			Env:      "sandbox",
		},
	})

	services, err := NewServices(application, database)
	if err != nil {
		t.Fatalf("NewServices: %v", err)
	}
	if services.accountsService == nil {
		t.Fatal("accounts service was not constructed")
	}
	if services.taskManager == nil {
		t.Fatal("task manager was not constructed")
	}
}
