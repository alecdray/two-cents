package adapters_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/core/app"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/sweep/adapters"
)

// safetyMarginRoutesServe wires the sweep routes onto a real mux behind
// session middleware — the composition root's own shape — and returns a
// dispatcher that drives a request through the actual registered route table,
// the same real-route discipline schedule_from_transaction_routes_test.go
// established: calling PostSafetyMargin directly would never catch an
// unrouted or mismatched-method form action.
func safetyMarginRoutesServe(t *testing.T, h *adapters.HttpHandler) func(*http.Request) *httptest.ResponseRecorder {
	t.Helper()
	application := app.NewApp(app.Config{Env: app.EnvLocal, JwtSecret: "test-secret"})
	appMux := httpx.NewMux(application, httpx.JwtMiddleware)
	adapters.RegisterRoutes(appMux, h)

	issueRec := httptest.NewRecorder()
	if err := application.IssueSession(issueRec, "user-1"); err != nil {
		t.Fatalf("IssueSession: %v", err)
	}
	cookies := issueRec.Result().Cookies()

	return func(req *http.Request) *httptest.ResponseRecorder {
		for _, c := range cookies {
			req.AddCookie(c)
		}
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		appMux.ServeHTTP(rec, req)
		return rec
	}
}

func postSafetyMargin(t *testing.T, serve func(*http.Request) *httptest.ResponseRecorder, raw string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/sweep/safety-margin", strings.NewReader("margin="+raw))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return serve(req)
}

// TestSafetyMarginDefaultsAndRoundTripsThroughRoutes drives the real
// GET /sweep and POST /sweep/safety-margin routes: a fresh database starts at
// the migration-seeded $500, a save takes effect immediately on the next page
// read, and 0 is accepted as a deliberate no-cushion choice ([ADR-0032]).
func TestSafetyMarginDefaultsAndRoundTripsThroughRoutes(t *testing.T) {
	h, _, _, _ := newHandler(t)
	serve := safetyMarginRoutesServe(t, h)

	_, body := getSweepPage(t, h)
	if !strings.Contains(body, `data-testid="sweep-safety-margin-value"`) {
		t.Fatalf("page does not render the safety-margin control:\n%s", body)
	}
	if !strings.Contains(body, "$500.00") {
		t.Fatalf("page does not show the migration-seeded $500.00 default:\n%s", body)
	}

	rec := postSafetyMargin(t, serve, "750.5")
	if rec.Code != http.StatusOK {
		t.Fatalf("save: status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "$750.50") {
		t.Fatalf("save response does not show the new value:\n%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `data-testid="sweep-schedule"`) || strings.Contains(rec.Body.String(), `data-testid="sweep-numeric"`) {
		t.Fatalf("save response is not scoped to the margin region alone:\n%s", rec.Body.String())
	}

	_, body = getSweepPage(t, h)
	if !strings.Contains(body, "$750.50") {
		t.Fatalf("page does not reflect the saved margin after a plain read:\n%s", body)
	}

	rec = postSafetyMargin(t, serve, "0")
	if rec.Code != http.StatusOK {
		t.Fatalf("save 0: status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "$0.00") {
		t.Fatalf("0 was not accepted as a deliberate no-cushion value:\n%s", rec.Body.String())
	}
}

// TestSafetyMarginRejectsInvalidInput covers the three rejected shapes: blank,
// unparseable, and negative. Each must leave the stored value untouched and
// reopen the form with its message, rather than silently keeping whatever was
// last valid with no explanation.
func TestSafetyMarginRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantMsg string
	}{
		{"blank", "", "Enter a safety margin."},
		{"not a number", "abc", "Enter the safety margin as a number."},
		{"negative", "-5", "Enter a safety margin of $0 or more."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, svc, _, ctx := newHandler(t)
			serve := safetyMarginRoutesServe(t, h)

			before, err := svc.SafetyMargin(ctx)
			if err != nil {
				t.Fatalf("SafetyMargin: %v", err)
			}

			rec := postSafetyMargin(t, serve, tc.raw)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200, body:\n%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.wantMsg) {
				t.Errorf("body = %q, want the message %q", body, tc.wantMsg)
			}
			if !strings.Contains(body, `x-data="{ editing: true }"`) {
				t.Errorf("rejected submit did not reopen the form in place:\n%s", body)
			}

			after, err := svc.SafetyMargin(ctx)
			if err != nil {
				t.Fatalf("SafetyMargin: %v", err)
			}
			if after != before {
				t.Errorf("stored margin = %v after a rejected submit, want it unchanged from %v", after, before)
			}
		})
	}
}
