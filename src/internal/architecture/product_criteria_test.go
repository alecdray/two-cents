package architecture

// Product criteria PC3 cross-goal tests: provider & architecture boundaries.
//
//   PC3 — Provider & architecture boundaries hold:
//     (a) The sweep module and its adapters import no provider client.
//         Domain modules reach the bank through the banking seam; sweep is no
//         exception — it reads balances and MTD data through the accounts and
//         transactions domain services, not the Plaid client.
//     (b) The Plaid provider client exposes no payment, transfer, or liabilities
//         endpoint. The feature must never add money-movement or
//         credit-position reads to the provider surface.
//     (c) The sweep module is a domain-module-archetype component: it depends
//         on other domain modules (accounts, budget, transactions) and must
//         not be imported by any module other than the composition root (server).
//
// PC1 and PC2 tests live in src/internal/sweep/product_criteria_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sweepPkg is the import path for the sweep domain module.
const sweepPkg = internalPkg + "/sweep"

// TestPC3_SweepModuleImportsNoProviderClient asserts that the sweep module
// (and its adapters) never imports the plaid or fakebank provider packages.
// Sweep reads account balances and month-to-date activity through the accounts
// and transactions domain services, not by reaching the bank directly. Breaking
// this boundary would couple the recommendation logic to a specific provider.
func TestPC3_SweepModuleImportsNoProviderClient(t *testing.T) {
	pkgs := listInternalPackages(t)

	// Anchor guard: confirm sweep is present in the import graph before asserting
	// about it — a missing anchor means the test swept nothing.
	var sawSweep bool
	for _, p := range pkgs {
		if p.ImportPath == sweepPkg || strings.HasPrefix(p.ImportPath, sweepPkg+"/") {
			sawSweep = true
			break
		}
	}
	if !sawSweep {
		t.Fatalf("sweep package %q not found in the import graph; the test is not exercising what it claims", sweepPkg)
	}

	var checked int
	for _, p := range pkgs {
		if p.ImportPath != sweepPkg && !strings.HasPrefix(p.ImportPath, sweepPkg+"/") {
			continue
		}
		checked++
		for _, imp := range allImports(p) {
			if imp == plaidPkg || imp == fakebankPkg {
				t.Errorf("PC3: %s imports the %q provider package; sweep must reach "+
					"balances and MTD data through the accounts and transactions "+
					"domain services, never a provider client directly",
					p.ImportPath, imp)
			}
			if strings.Contains(strings.ToLower(imp), "plaid") && imp != plaidPkg {
				t.Errorf("PC3: %s imports a Plaid-named dependency %q; the sweep module must stay provider-agnostic", p.ImportPath, imp)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("no packages under %q were checked; the import-graph sweep matched nothing", sweepPkg)
	}
}

// TestPC3_SweepIsImportedOnlyByServer asserts that nothing imports the sweep
// module except the composition root (server) and sweep's own sub-packages.
// Sweep is a domain-module; like home it composes multiple services and must
// not be imported by any other domain module.
func TestPC3_SweepIsImportedOnlyByServer(t *testing.T) {
	pkgs := listInternalPackages(t)

	// Anchor guard.
	var sawSweep, sawServer bool
	for _, p := range pkgs {
		switch {
		case p.ImportPath == sweepPkg || strings.HasPrefix(p.ImportPath, sweepPkg+"/"):
			sawSweep = true
		case p.ImportPath == serverPkg:
			sawServer = true
		}
	}
	if !sawSweep {
		t.Fatalf("sweep package %q not found in the import graph", sweepPkg)
	}
	if !sawServer {
		t.Fatalf("server package %q not found in the import graph", serverPkg)
	}

	var checked int
	for _, p := range pkgs {
		// sweep may import itself (sub-packages); server wires it in.
		if p.ImportPath == sweepPkg || strings.HasPrefix(p.ImportPath, sweepPkg+"/") {
			continue
		}
		if p.ImportPath == serverPkg {
			continue
		}
		checked++
		for _, imp := range allImports(p) {
			if imp == sweepPkg || strings.HasPrefix(imp, sweepPkg+"/") {
				t.Errorf("PC3: %s imports sweep package %q; only the composition root (server) may import sweep", p.ImportPath, imp)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("no packages were checked; the sweep matched nothing")
	}
}

// TestPC3_PlaidProviderSurfaceHasNoPaymentTransferOrLiabilitiesEndpoint reads
// all production source files in the plaid provider package and asserts that
// none contain the forbidden Plaid endpoint path strings. The feature constraint
// is read-only data access (accounts, balances, transactions): money-movement
// endpoints (transfer, payment) and credit-position endpoints (liabilities, auth)
// must never be added to the provider surface.
func TestPC3_PlaidProviderSurfaceHasNoPaymentTransferOrLiabilitiesEndpoint(t *testing.T) {
	// Relative to the architecture package directory (src/internal/architecture),
	// the plaid package is one level up.
	plaidDir := "../plaid"

	entries, err := os.ReadDir(plaidDir)
	if err != nil {
		t.Fatalf("PC3: could not read plaid directory %q: %v", plaidDir, err)
	}

	// Forbidden Plaid API path prefixes: money-movement and liability endpoints.
	forbidden := []string{
		"/transfer",
		"/payment",
		"/liabilities",
	}

	var filesChecked int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Production source files only: skip test files.
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		filesChecked++

		src, err := os.ReadFile(filepath.Join(plaidDir, name))
		if err != nil {
			t.Fatalf("PC3: reading %s: %v", name, err)
		}
		content := string(src)

		for _, pattern := range forbidden {
			if strings.Contains(content, pattern) {
				t.Errorf("PC3: plaid/%s contains forbidden endpoint string %q — "+
					"the sweep feature must not introduce payment, transfer, or "+
					"liabilities calls to the provider client",
					name, pattern)
			}
		}
	}

	if filesChecked == 0 {
		t.Fatalf("PC3: no production .go files found in plaid/ — the endpoint check covered nothing")
	}

	// Anchor guard: confirm the read-data endpoint we DO use is present, so the
	// test doesn't silently pass because it targeted the wrong directory.
	var sawTransactionsSync bool
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, _ := os.ReadFile(filepath.Join(plaidDir, e.Name()))
		if strings.Contains(string(src), "/transactions/sync") {
			sawTransactionsSync = true
			break
		}
	}
	if !sawTransactionsSync {
		t.Fatalf("PC3 anchor: /transactions/sync not found in plaid/ — "+
			"the endpoint check is targeting the wrong directory or the provider surface changed unexpectedly")
	}
}
