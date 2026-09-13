package adapters_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/accounts/adapters"
	"github.com/alecdray/two-cents/src/internal/banking"
)

// The re-authorize remedy drives an Alpine component defined by a script the
// page must actually emit. That script used to live inside the reconnect
// control, which renders only on a needs-reconnect row — and a
// statements-unavailable card sits on an **active** connection by design
// ([ADR-0026]), so the script was absent exactly where the remedy needed it and
// the button did nothing in real bank mode. The e2e suite runs in fake mode and
// cannot see this, so it is pinned here.
func TestStatementsUnavailableRemedyIsWiredInRealBankMode(t *testing.T) {
	database := newTestDB(t)
	ctx := testCtx()

	provider := &fakeProvider{
		refuseStatements: true,
		accounts: []banking.Account{
			providerAccount("p-card", "Sapphire", banking.KindCredit, "credit card", knownBalance("p-card", 840)),
		},
	}
	svc := accounts.NewService(database, provider, testKey)
	if _, err := svc.RegisterConnection(ctx, "token", "item"); err != nil {
		t.Fatalf("register: %v", err)
	}
	// The sync records the gap per card and leaves the connection active.
	if err := svc.SyncAccounts(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	handler := adapters.NewHttpHandler(svc, adapters.BankModeReal, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	rec := httptest.NewRecorder()
	handler.GetOverviewPage(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "accounts-overview-card-reauthorize") {
		t.Fatalf("remedy control absent from a statements-unavailable card in real bank mode")
	}
	if !strings.Contains(body, "function bankReconnect") {
		t.Errorf("remedy renders but the Alpine component it calls is never defined — the control is inert")
	}
	// The connection is healthy, so the reconnect control must not be what is
	// carrying the script: that is the coupling this guards against.
	if strings.Contains(body, "accounts-overview-account-reconnect") {
		t.Errorf("a needs-reconnect control rendered on an active connection")
	}
}
