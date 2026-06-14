package plaid

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// linkConfig is the app-level configuration a real deployment threads into the
// client; the link-token tests assert it reaches Plaid's link/token/create body.
var linkConfig = LinkConfig{
	ClientName:   "Two Cents",
	Language:     "en",
	CountryCodes: []string{"US"},
	Products:     []string{"transactions"},
	ClientUserID: "two-cents",
}

func TestCreateLinkToken(t *testing.T) {
	t.Run("a new connection returns a real-mode token and requests the configured products", func(t *testing.T) {
		fs := newFixtureServer(t, map[string][][]byte{
			"/link/token/create": {[]byte(`{"link_token":"link-sandbox-new","expiration":"2026-01-01T00:00:00Z","request_id":"req-1"}`)},
		})
		client, err := NewClient("client-id", "secret", WithOrigin(fs.srv.URL), WithLinkConfig(linkConfig))
		if err != nil {
			t.Fatalf("building client: %v", err)
		}
		svc := NewService(client)

		token, err := svc.CreateLinkToken(testContext(), banking.LinkOptions{})
		if err != nil {
			t.Fatalf("creating link token: %v", err)
		}

		if token.Token != "link-sandbox-new" {
			t.Errorf("expected token 'link-sandbox-new', got %q", token.Token)
		}
		if token.Mode != "real" {
			t.Errorf("expected mode 'real', got %q", token.Mode)
		}

		body := fs.requestBodies[0]
		if _, ok := body["products"]; !ok {
			t.Errorf("a new-connection request must carry products, got body %v", body)
		}
		if _, ok := body["access_token"]; ok {
			t.Errorf("a new-connection request must not carry an access_token, got body %v", body)
		}
		if body["client_name"] != "Two Cents" {
			t.Errorf("expected client_name 'Two Cents', got %v", body["client_name"])
		}
		user, ok := body["user"].(map[string]any)
		if !ok || user["client_user_id"] != "two-cents" {
			t.Errorf("expected user.client_user_id 'two-cents', got %v", body["user"])
		}
	})

	t.Run("update mode returns a real-mode token, carries the access token, and omits products", func(t *testing.T) {
		fs := newFixtureServer(t, map[string][][]byte{
			"/link/token/create": {[]byte(`{"link_token":"link-sandbox-update","request_id":"req-2"}`)},
		})
		client, err := NewClient("client-id", "secret", WithOrigin(fs.srv.URL), WithLinkConfig(linkConfig))
		if err != nil {
			t.Fatalf("building client: %v", err)
		}
		svc := NewService(client)

		token, err := svc.CreateLinkToken(testContext(), banking.LinkOptions{AccessToken: "access-token-existing"})
		if err != nil {
			t.Fatalf("creating update-mode link token: %v", err)
		}

		if token.Token != "link-sandbox-update" {
			t.Errorf("expected token 'link-sandbox-update', got %q", token.Token)
		}
		if token.Mode != "real" {
			t.Errorf("expected mode 'real', got %q", token.Mode)
		}

		body := fs.requestBodies[0]
		if body["access_token"] != "access-token-existing" {
			t.Errorf("update mode must carry the existing access_token, got %v", body["access_token"])
		}
		if _, ok := body["products"]; ok {
			t.Errorf("update mode must omit products, got body %v", body)
		}
	})
}

func TestExchangePublicToken(t *testing.T) {
	fs := newFixtureServer(t, map[string][][]byte{
		"/item/public_token/exchange": {[]byte(`{"access_token":"access-prod-123","item_id":"item-prod-456","request_id":"req-3"}`)},
	})
	svc := fs.service(t)

	item, err := svc.ExchangePublicToken(testContext(), "public-sandbox-token")
	if err != nil {
		t.Fatalf("exchanging public token: %v", err)
	}

	if item.AccessToken != "access-prod-123" {
		t.Errorf("expected access token 'access-prod-123', got %q", item.AccessToken)
	}
	if item.ProviderItemID != "item-prod-456" {
		t.Errorf("expected provider item id 'item-prod-456', got %q", item.ProviderItemID)
	}

	body := fs.requestBodies[0]
	if body["public_token"] != "public-sandbox-token" {
		t.Errorf("expected the public token in the request, got %v", body["public_token"])
	}
}

func TestRemoveItem(t *testing.T) {
	fs := newFixtureServer(t, map[string][][]byte{
		"/item/remove": {[]byte(`{"request_id":"req-4"}`)},
	})
	svc := fs.service(t)

	if err := svc.RemoveItem(testContext(), "access-token-to-remove"); err != nil {
		t.Fatalf("removing item: %v", err)
	}

	if fs.calls["/item/remove"] != 1 {
		t.Errorf("expected the remove endpoint to be called once, got %d", fs.calls["/item/remove"])
	}
	body := fs.requestBodies[0]
	if body["access_token"] != "access-token-to-remove" {
		t.Errorf("expected the access token in the remove request, got %v", body["access_token"])
	}
}
