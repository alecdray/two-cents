package plaid

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
)

// When Plaid reports ITEM_LOGIN_REQUIRED on a non-200 response, the client maps
// it onto the provider-agnostic banking.ErrReauthRequired so consumers never
// see Plaid's native error vocabulary.
func TestItemLoginRequiredMapsToReauthSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error_type": "ITEM_ERROR",
			"error_code": "ITEM_LOGIN_REQUIRED",
			"error_message": "the login details of this item have changed",
			"display_message": "please reconnect your account"
		}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("client-id", "secret", WithOrigin(srv.URL))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	svc := NewService(client)

	t.Run("ListAccounts surfaces the sentinel", func(t *testing.T) {
		_, err := svc.ListAccounts(testContext(), "access-token")
		if !errors.Is(err, banking.ErrReauthRequired) {
			t.Fatalf("ListAccounts err = %v, want banking.ErrReauthRequired", err)
		}
	})

	t.Run("GetBalances surfaces the sentinel", func(t *testing.T) {
		_, err := svc.GetBalances(testContext(), "access-token")
		if !errors.Is(err, banking.ErrReauthRequired) {
			t.Fatalf("GetBalances err = %v, want banking.ErrReauthRequired", err)
		}
	})
}

// NO_ACCOUNTS is an Item-level state the user must resolve at their institution
// (all accounts closed, or the Item's account selection no longer matches any
// account). It never clears on retry, so it maps onto banking.ErrReauthRequired
// like a login-required Item — the connection is flagged and skipped rather than
// failing the sync pass.
func TestNoAccountsMapsToReauthSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error_type": "ITEM_ERROR",
			"error_code": "NO_ACCOUNTS",
			"error_message": "no valid accounts were found for this item",
			"display_message": "No valid accounts were found at the financial institution."
		}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("client-id", "secret", WithOrigin(srv.URL))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	svc := NewService(client)

	t.Run("ListAccounts surfaces the sentinel", func(t *testing.T) {
		_, err := svc.ListAccounts(testContext(), "access-token")
		if !errors.Is(err, banking.ErrReauthRequired) {
			t.Fatalf("ListAccounts err = %v, want banking.ErrReauthRequired", err)
		}
	})

	t.Run("GetBalances surfaces the sentinel", func(t *testing.T) {
		_, err := svc.GetBalances(testContext(), "access-token")
		if !errors.Is(err, banking.ErrReauthRequired) {
			t.Fatalf("GetBalances err = %v, want banking.ErrReauthRequired", err)
		}
	})
}

// A transient provider-side failure must NOT be classified as needing re-auth:
// flagging the connection would tell the user to reconnect a login that is
// perfectly valid and will recover on the next tick by itself.
func TestTransientProviderErrorIsNotReauthSentinel(t *testing.T) {
	for _, code := range []string{"INSTITUTION_DOWN", "INSTITUTION_NOT_RESPONDING", "RATE_LIMIT_EXCEEDED"} {
		t.Run(code, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error_type":"INSTITUTION_ERROR","error_code":"` + code + `"}`))
			}))
			t.Cleanup(srv.Close)

			client, err := NewClient("client-id", "secret", WithOrigin(srv.URL))
			if err != nil {
				t.Fatalf("building client: %v", err)
			}
			svc := NewService(client)

			if _, err := svc.ListAccounts(testContext(), "access-token"); err == nil {
				t.Fatal("expected an error")
			} else if errors.Is(err, banking.ErrReauthRequired) {
				t.Fatalf("transient error %s must not map to the reauth sentinel; got %v", code, err)
			}
		})
	}
}

// A non-200 that is not a login-required error keeps the generic status error
// and does not masquerade as the reauth sentinel.
func TestOtherErrorIsNotReauthSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error_code":"INTERNAL_SERVER_ERROR"}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("client-id", "secret", WithOrigin(srv.URL))
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	svc := NewService(client)

	if _, err := svc.ListAccounts(testContext(), "access-token"); err == nil {
		t.Fatal("expected an error")
	} else if errors.Is(err, banking.ErrReauthRequired) {
		t.Fatalf("non-login error must not map to the reauth sentinel; got %v", err)
	}
}
