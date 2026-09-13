package plaid

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// The seam documents a login with no supported credit account as an ordinary
// empty result, not a failure — and the app must honour that, or a login whose
// accounts we type as credit but which the product does not cover returns a
// hard error from every sync pass, forever, with the connection deliberately
// left unchanged.
func TestGetCardStatementsTreatsNoLiabilityAccountsAsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error_type": "INVALID_REQUEST",
			"error_code": "NO_LIABILITY_ACCOUNTS",
		})
	}))
	defer server.Close()

	client, err := NewClient("client-id", "secret", WithOrigin(server.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := NewService(client).GetCardStatements(contextx.NewContextX(t.Context()), "access-token")
	if err != nil {
		t.Fatalf("GetCardStatements = %v, want an ordinary empty result", err)
	}
	if len(got) != 0 {
		t.Errorf("statements = %v, want none", got)
	}
}
