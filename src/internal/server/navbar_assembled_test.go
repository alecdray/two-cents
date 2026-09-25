package server_test

// Assembled test for the shared navbar. The claim is cross-module — the same
// core/templates navbar renders on surfaces owned by different modules — so it
// can only be asserted where more than one module's views are in scope. A domain
// module's adapters may not import a peer's adapters
// (docs/architecture/archetypes/domain-module.md); the composition root may.

import (
	"context"
	"strings"
	"testing"

	"github.com/alecdray/two-cents/src/internal/accounts"
	accountsViews "github.com/alecdray/two-cents/src/internal/accounts/adapters/views"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	txnViews "github.com/alecdray/two-cents/src/internal/transactions/adapters/views"
)

func navbarTestCtx() contextx.ContextX {
	return contextx.NewContextX(context.Background())
}

// TestNavbarOnTransactionsPage asserts the transactions page renders the navbar
// with both links.
func TestNavbarOnTransactionsPage(t *testing.T) {
	var sb strings.Builder
	if err := txnViews.TransactionsPage(false, nil, txnViews.ListControls{}).Render(navbarTestCtx(), &sb); err != nil {
		t.Fatalf("render transactions page: %v", err)
	}
	assertNavbar(t, "transactions page", sb.String())
}

// TestNavbarOnOverviewPage asserts the accounts overview page renders the same
// navbar with both links, so the navbar appears on both surfaces.
func TestNavbarOnOverviewPage(t *testing.T) {
	var sb strings.Builder
	if err := accountsViews.AccountsOverviewPage(accounts.Dashboard{}, accountsViews.BankModeFake).Render(navbarTestCtx(), &sb); err != nil {
		t.Fatalf("render overview page: %v", err)
	}
	assertNavbar(t, "overview page", sb.String())
}

func assertNavbar(t *testing.T, page, body string) {
	t.Helper()
	checks := map[string]string{
		"spending link testid":     `data-testid="nav-spending"`,
		"accounts link testid":     `data-testid="nav-accounts"`,
		"transactions link testid": `data-testid="nav-transactions"`,
		"home href":                `href="/"`,
		"accounts href":            `href="/accounts"`,
		"transactions href":        `href="/transactions"`,
	}
	for label, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("%s missing %s (%q)", page, label, want)
		}
	}
}
