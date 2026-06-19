package adapters_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/accounts/adapters"
	"github.com/alecdray/two-cents/src/internal/banking"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
)

// repairSpy records how many times the injected re-pair seam fired, so a test can
// assert that an override re-pairs transfers exactly when counts-as-savings
// effectively changed — and never otherwise.
type repairSpy struct{ calls int }

func (s *repairSpy) repair(_ contextx.ContextX) error {
	s.calls++
	return nil
}

// savingsAccount builds a provider account carrying the counts-as-savings flag,
// which the package's providerAccount helper does not set.
func savingsAccount(id, name string, kind banking.AccountKind, savings bool, balance banking.Balance) banking.Account {
	a := providerAccount(id, name, kind, "", balance)
	a.CountsAsSavings = savings
	return a
}

// overrideFixture registers the canonical trio and returns the service, a fresh
// re-pair spy, and a handler wired to that spy.
func overrideFixture(t *testing.T) (*accounts.Service, contextx.ContextX, *repairSpy, *adapters.HttpHandler) {
	t.Helper()
	database := newTestDB(t)
	ctx := testCtx()
	provider := &fakeProvider{accounts: []banking.Account{
		savingsAccount("p-check", "Everyday Checking", banking.KindCash, false, knownBalance("p-check", 1200)),
		savingsAccount("p-save", "High-Yield Savings", banking.KindCash, true, knownBalance("p-save", 3400)),
		savingsAccount("p-card", "Travel Rewards Card", banking.KindCredit, false, knownBalance("p-card", 450)),
	}}
	svc := accounts.NewService(database, provider, testKey)
	if _, err := svc.RegisterConnection(ctx, "tok", "item-1"); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}
	spy := &repairSpy{}
	handler := adapters.NewHttpHandler(svc, adapters.BankModeFake, nil, spy.repair)
	return svc, ctx, spy, handler
}

// rowByName finds a rendered account row across all buckets, returning its id and
// the bucket it currently sits in.
func rowByName(t *testing.T, svc *accounts.Service, ctx contextx.ContextX, name string) (id, bucket string) {
	t.Helper()
	dash, err := svc.Dashboard(ctx)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	for _, g := range []struct {
		bucket string
		rows   []accounts.AccountRow
	}{{"cash", dash.Cash}, {"credit", dash.Credit}, {"other", dash.Other}} {
		for _, r := range g.rows {
			if r.Name == name {
				return r.ID, g.bucket
			}
		}
	}
	t.Fatalf("no row named %q", name)
	return "", ""
}

func postKind(t *testing.T, h *adapters.HttpHandler, id, kind string) (int, string) {
	t.Helper()
	body := url.Values{"kind": {kind}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/accounts/accounts/"+id+"/kind", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.PostAccountKind(rec, req)
	return rec.Code, rec.Body.String()
}

func postSavings(t *testing.T, h *adapters.HttpHandler, id string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/accounts/accounts/"+id+"/counts-as-savings", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.PostCountsAsSavings(rec, req)
	return rec.Code, rec.Body.String()
}

func TestPostAccountKind(t *testing.T) {
	t.Run("cash→other re-buckets the row and does not re-pair", func(t *testing.T) {
		svc, ctx, spy, handler := overrideFixture(t)
		id, _ := rowByName(t, svc, ctx, "Everyday Checking")

		code, _ := postKind(t, handler, id, "other")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if _, bucket := rowByName(t, svc, ctx, "Everyday Checking"); bucket != "other" {
			t.Errorf("bucket = %q, want other after the override", bucket)
		}
		if spy.calls != 0 {
			t.Errorf("re-pair fired %d times, want 0 for a cash→other change", spy.calls)
		}
	})

	t.Run("savings→credit re-pairs, re-buckets, and drops the savings toggle", func(t *testing.T) {
		svc, ctx, spy, handler := overrideFixture(t)
		id, _ := rowByName(t, svc, ctx, "High-Yield Savings")

		code, body := postKind(t, handler, id, "credit")
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if _, bucket := rowByName(t, svc, ctx, "High-Yield Savings"); bucket != "credit" {
			t.Errorf("bucket = %q, want credit after the override", bucket)
		}
		if spy.calls != 1 {
			t.Errorf("re-pair fired %d times, want 1 (credit cleared a set savings flag)", spy.calls)
		}
		if strings.Contains(body, "/accounts/accounts/"+id+"/counts-as-savings") {
			t.Errorf("the credit row still renders a counts-as-savings toggle")
		}
	})

	t.Run("an unknown kind is a 400", func(t *testing.T) {
		svc, ctx, spy, handler := overrideFixture(t)
		id, _ := rowByName(t, svc, ctx, "Everyday Checking")

		code, _ := postKind(t, handler, id, "bogus")
		if code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", code)
		}
		if spy.calls != 0 {
			t.Errorf("re-pair fired on a rejected override")
		}
	})
}

func TestPostCountsAsSavings(t *testing.T) {
	t.Run("toggling a cash account re-pairs", func(t *testing.T) {
		svc, ctx, spy, handler := overrideFixture(t)
		id, _ := rowByName(t, svc, ctx, "Everyday Checking")

		code, _ := postSavings(t, handler, id)
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if spy.calls != 1 {
			t.Errorf("re-pair fired %d times, want 1 after a savings toggle", spy.calls)
		}
	})

	t.Run("toggling a credit account is a 400 and does not re-pair", func(t *testing.T) {
		svc, ctx, spy, handler := overrideFixture(t)
		id, _ := rowByName(t, svc, ctx, "Travel Rewards Card")

		code, _ := postSavings(t, handler, id)
		if code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", code)
		}
		if spy.calls != 0 {
			t.Errorf("re-pair fired on a rejected toggle")
		}
	})
}
