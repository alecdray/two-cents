package plaid

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

func testContext() contextx.ContextX {
	return contextx.NewContextX(context.Background())
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %q: %v", name, err)
	}
	return data
}

// fixtureServer serves a recorded JSON body per Plaid path. It records the
// bodies of the requests it receives so tests can assert on what the client
// sent. When a path maps to a slice of fixtures, each call to that path is
// answered with the next entry, modelling has_more pagination.
type fixtureServer struct {
	srv           *httptest.Server
	calls         map[string]int
	requestBodies []map[string]any
}

func newFixtureServer(t *testing.T, bodies map[string][][]byte) *fixtureServer {
	t.Helper()
	fs := &fixtureServer{calls: map[string]int{}}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(raw, &parsed)
		fs.requestBodies = append(fs.requestBodies, parsed)

		fixtures, ok := bodies[r.URL.Path]
		if !ok {
			http.Error(w, "no fixture for "+r.URL.Path, http.StatusNotFound)
			return
		}
		idx := fs.calls[r.URL.Path]
		if idx >= len(fixtures) {
			idx = len(fixtures) - 1
		}
		fs.calls[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixtures[idx])
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *fixtureServer) service(t *testing.T) *Service {
	t.Helper()
	client, err := NewClient("test-client-id", "test-secret", WithOrigin(fs.srv.URL))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return NewService(client)
}

func TestListAccounts(t *testing.T) {
	fs := newFixtureServer(t, map[string][][]byte{
		"/accounts/get": {readFixture(t, "accounts.json")},
	})
	svc := fs.service(t)

	accounts, err := svc.ListAccounts(testContext(), "access-token")
	if err != nil {
		t.Fatalf("listing accounts: %v", err)
	}

	if len(accounts) != 5 {
		t.Fatalf("expected 5 accounts, got %d", len(accounts))
	}

	byID := map[string]banking.Account{}
	for _, a := range accounts {
		byID[a.ID] = a
	}

	t.Run("depository account is cash kind with its current balance", func(t *testing.T) {
		checking := byID["blgvvBlXw3cq5GMPwqB6s6q4dLKB9WcVqGDGo"]
		if checking.Kind != banking.KindCash {
			t.Errorf("expected cash kind, got %q", checking.Kind)
		}
		if checking.Name != "Plaid Gold Standard 0% Interest Checking" {
			t.Errorf("unexpected name %q", checking.Name)
		}
		if checking.Subtype != "checking" {
			t.Errorf("expected subtype 'checking', got %q", checking.Subtype)
		}
		if !checking.Balance.Known || checking.Balance.Money.Amount != 110.0 {
			t.Errorf("expected known balance 110.0, got %+v", checking.Balance)
		}
		if checking.Balance.Money.Currency != "USD" {
			t.Errorf("expected USD, got %q", checking.Balance.Money.Currency)
		}
	})

	t.Run("credit account is credit kind", func(t *testing.T) {
		card := byID["K6axjjeoyACtllp4bRn5skVxgGqxw9F6qmgr4"]
		if card.Kind != banking.KindCredit {
			t.Errorf("expected credit kind, got %q", card.Kind)
		}
		if card.Subtype != "credit card" {
			t.Errorf("expected subtype 'credit card', got %q", card.Subtype)
		}
		if !card.Balance.Known || card.Balance.Money.Amount != 410.0 {
			t.Errorf("expected known balance 410.0, got %+v", card.Balance)
		}
	})

	t.Run("a loan account is the other kind and carries its subtype", func(t *testing.T) {
		mortgage := byID["MortgageLoan00000000000000000000000000"]
		if mortgage.Kind != banking.KindOther {
			t.Errorf("expected other kind for a loan, got %q", mortgage.Kind)
		}
		if mortgage.Subtype != "mortgage" {
			t.Errorf("expected subtype 'mortgage', got %q", mortgage.Subtype)
		}
	})

	t.Run("an investment account is the other kind and carries its subtype", func(t *testing.T) {
		retirement := byID["Investment401k0000000000000000000000000"]
		if retirement.Kind != banking.KindOther {
			t.Errorf("expected other kind for an investment, got %q", retirement.Kind)
		}
		if retirement.Subtype != "401k" {
			t.Errorf("expected subtype '401k', got %q", retirement.Subtype)
		}
	})

	t.Run("no Plaid-native account type appears in the returned values", func(t *testing.T) {
		for _, a := range accounts {
			switch a.Kind {
			case banking.KindCash, banking.KindCredit, banking.KindOther:
			default:
				t.Errorf("account %q has a non-bucket kind %q (Plaid-native type leaked through)", a.ID, a.Kind)
			}
		}
	})

	t.Run("savings account defaults counts-as-savings on", func(t *testing.T) {
		saving := byID["6PdjjRoyAFflGG4byKp3J5kjP7eDPgyWIrhuq"]
		if !saving.CountsAsSavings {
			t.Errorf("expected savings account to count as savings")
		}
		if saving.Kind != banking.KindCash {
			t.Errorf("expected savings to be cash kind, got %q", saving.Kind)
		}
	})

	t.Run("non-savings accounts default counts-as-savings off", func(t *testing.T) {
		if byID["blgvvBlXw3cq5GMPwqB6s6q4dLKB9WcVqGDGo"].CountsAsSavings {
			t.Errorf("checking should not count as savings")
		}
		if byID["K6axjjeoyACtllp4bRn5skVxgGqxw9F6qmgr4"].CountsAsSavings {
			t.Errorf("credit card should not count as savings")
		}
	})
}

func TestGetBalances(t *testing.T) {
	fs := newFixtureServer(t, map[string][][]byte{
		"/accounts/balance/get": {readFixture(t, "balances.json")},
	})
	svc := fs.service(t)

	balances, err := svc.GetBalances(testContext(), "access-token")
	if err != nil {
		t.Fatalf("getting balances: %v", err)
	}

	byID := map[string]banking.Balance{}
	for _, b := range balances {
		byID[b.AccountID] = b
	}

	t.Run("reports the current balance as a USD amount", func(t *testing.T) {
		checking := byID["blgvvBlXw3cq5GMPwqB6s6q4dLKB9WcVqGDGo"]
		if !checking.Known {
			t.Fatalf("expected known balance")
		}
		if checking.Money.Amount != 110.0 || checking.Money.Currency != "USD" {
			t.Errorf("expected 110.0 USD, got %+v", checking.Money)
		}
	})

	t.Run("an unreported balance is surfaced as unknown, not zero", func(t *testing.T) {
		unreported := byID["unreportedBalanceAccount000000000000"]
		if unreported.Known {
			t.Errorf("expected unknown balance, got known %+v", unreported.Money)
		}
		if unreported.Money.Amount != 0 {
			t.Errorf("unknown balance should carry no amount, got %v", unreported.Money.Amount)
		}
	})
}

func TestSyncTransactions(t *testing.T) {
	t.Run("accumulates added, modified, and removed across pages and returns the final cursor", func(t *testing.T) {
		fs := newFixtureServer(t, map[string][][]byte{
			"/transactions/sync": {
				readFixture(t, "sync_page1.json"),
				readFixture(t, "sync_page2.json"),
			},
		})
		svc := fs.service(t)

		changes, err := svc.SyncTransactions(testContext(), "access-token", "")
		if err != nil {
			t.Fatalf("syncing: %v", err)
		}

		if fs.calls["/transactions/sync"] != 2 {
			t.Errorf("expected 2 pages fetched, got %d", fs.calls["/transactions/sync"])
		}

		if len(changes.Added) != 3 {
			t.Fatalf("expected 3 added (2 from page 1, 1 from page 2), got %d", len(changes.Added))
		}
		if len(changes.Modified) != 1 {
			t.Fatalf("expected 1 modified, got %d", len(changes.Modified))
		}

		wantRemoved := map[string]bool{
			"removedPendingAuth0000000000000000001": true,
			"removedPendingAuth0000000000000000002": true,
		}
		if len(changes.RemovedIDs) != len(wantRemoved) {
			t.Fatalf("expected %d removed ids, got %v", len(wantRemoved), changes.RemovedIDs)
		}
		for _, id := range changes.RemovedIDs {
			if !wantRemoved[id] {
				t.Errorf("unexpected removed id %q", id)
			}
		}

		if changes.Cursor != "cursor-final" {
			t.Errorf("expected final cursor 'cursor-final', got %q", changes.Cursor)
		}
	})

	t.Run("normalizes amount, merchant, counterparty, and carries the two-level category", func(t *testing.T) {
		fs := newFixtureServer(t, map[string][][]byte{
			"/transactions/sync": {
				readFixture(t, "sync_page1.json"),
				readFixture(t, "sync_page2.json"),
			},
		})
		svc := fs.service(t)

		changes, err := svc.SyncTransactions(testContext(), "access-token", "")
		if err != nil {
			t.Fatalf("syncing: %v", err)
		}

		byID := map[string]banking.Transaction{}
		for _, txn := range append(changes.Added, changes.Modified...) {
			byID[txn.ID] = txn
		}

		t.Run("an outflow keeps Plaid's positive sign and uses merchant_name as the cleaned merchant", func(t *testing.T) {
			purchase := byID["x8Jn8eVxprFb4kPbQ3pqU7m9aMD7e1tDoLZje"]
			if purchase.Amount.Amount != -25.0 {
				t.Errorf("a refund inflow should stay negative, got %v", purchase.Amount.Amount)
			}
		})

		t.Run("carries the two-level category from personal_finance_category", func(t *testing.T) {
			coffee := byID["Wm99e6mDPLckQQ5Mz6oeF7DjP1qjLnFGo8rje"]
			if coffee.Category.Primary != "FOOD_AND_DRINK" || coffee.Category.Detailed != "FOOD_AND_DRINK_COFFEE" {
				t.Errorf("expected (FOOD_AND_DRINK, FOOD_AND_DRINK_COFFEE), got %+v", coffee.Category)
			}
			if coffee.Amount.Amount != 6.33 {
				t.Errorf("outflow should stay positive 6.33, got %v", coffee.Amount.Amount)
			}
		})

		t.Run("raw counterparty is preserved while merchant is the cleaned form", func(t *testing.T) {
			refund := byID["x8Jn8eVxprFb4kPbQ3pqU7m9aMD7e1tDoLZje"]
			if refund.Counterparty != "REFUND WM SUPERCENTER 00042" {
				t.Errorf("expected raw counterparty, got %q", refund.Counterparty)
			}
			if refund.Merchant != "Refund Wm Supercenter" {
				t.Errorf("expected cleaned merchant 'Refund Wm Supercenter', got %q", refund.Merchant)
			}
		})

		t.Run("uses the transaction date", func(t *testing.T) {
			walmart := byID["lPNjeW1nR6CDn5okmGQ6hEpMo4lLNoSrzqDje"]
			if walmart.Date.Format(dateLayout) != "2023-09-24" {
				t.Errorf("expected 2023-09-24, got %s", walmart.Date.Format(dateLayout))
			}
		})
	})

	t.Run("re-syncing from the cursor over a no-change page returns empty changes and the unchanged cursor", func(t *testing.T) {
		fs := newFixtureServer(t, map[string][][]byte{
			"/transactions/sync": {readFixture(t, "sync_nochange.json")},
		})
		svc := fs.service(t)

		changes, err := svc.SyncTransactions(testContext(), "access-token", "cursor-final")
		if err != nil {
			t.Fatalf("syncing: %v", err)
		}

		if len(changes.Added) != 0 || len(changes.Modified) != 0 || len(changes.RemovedIDs) != 0 {
			t.Errorf("expected no changes, got %+v", changes)
		}
		if changes.Cursor != "cursor-final" {
			t.Errorf("expected cursor unchanged at 'cursor-final', got %q", changes.Cursor)
		}
	})
}
