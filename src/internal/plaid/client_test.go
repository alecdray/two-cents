package plaid

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	t.Run("rejects a blank client id", func(t *testing.T) {
		if _, err := NewClient("", "secret"); err == nil {
			t.Fatal("expected error for blank client id")
		}
	})

	t.Run("rejects a blank secret", func(t *testing.T) {
		if _, err := NewClient("client-id", ""); err == nil {
			t.Fatal("expected error for blank secret")
		}
	})

	t.Run("builds with both credentials present", func(t *testing.T) {
		if _, err := NewClient("client-id", "secret"); err != nil {
			t.Fatalf("expected success, got %v", err)
		}
	})
}

func TestRequestCredentials(t *testing.T) {
	fs := newFixtureServer(t, map[string][][]byte{
		"/accounts/get":         {readFixture(t, "accounts.json")},
		"/accounts/balance/get": {readFixture(t, "balances.json")},
		"/transactions/sync":    {readFixture(t, "sync_nochange.json")},
	})

	client, err := NewClient("the-client-id", "the-secret", WithOrigin(fs.srv.URL))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	svc := NewService(client)

	if _, err := svc.ListAccounts(testContext(), "the-access-token"); err != nil {
		t.Fatalf("listing accounts: %v", err)
	}
	if _, err := svc.GetBalances(testContext(), "the-access-token"); err != nil {
		t.Fatalf("getting balances: %v", err)
	}
	if _, err := svc.SyncTransactions(testContext(), "the-access-token", ""); err != nil {
		t.Fatalf("syncing: %v", err)
	}

	if len(fs.requestBodies) != 3 {
		t.Fatalf("expected 3 captured requests, got %d", len(fs.requestBodies))
	}

	for i, body := range fs.requestBodies {
		t.Run("request carries the configured credentials and access token", func(t *testing.T) {
			if body["client_id"] != "the-client-id" {
				t.Errorf("request %d: expected client_id 'the-client-id', got %v", i, body["client_id"])
			}
			if body["secret"] != "the-secret" {
				t.Errorf("request %d: expected secret 'the-secret', got %v", i, body["secret"])
			}
			if body["access_token"] != "the-access-token" {
				t.Errorf("request %d: expected access_token 'the-access-token', got %v", i, body["access_token"])
			}
		})
	}
}
