package architecture

// Product criteria PC3 cross-goal tests: provider & architecture boundaries.
//
//   PC3 — Provider & architecture boundaries hold:
//     (a) The sweep module and its adapters import no provider client.
//         Domain modules reach the bank through the banking seam; sweep is no
//         exception — it reads balances through the accounts domain service,
//         not the Plaid client.
//     (b) The Plaid provider client moves no money and reads no loan detail.
//         Transfer and payment endpoints must never appear. The liabilities
//         endpoint is permitted — [ADR-0024] narrowed that non-goal to loan APR
//         and interest detail so billing-cycle facts could date a card's
//         obligation — but decoding the APR or interest detail itself would
//         reopen what the narrowing kept closed.
//     (c) The sweep module is a domain-module-archetype component: it depends
//         on other domain modules (accounts, schedule) and must not be imported
//         by any module other than the composition root (server).
//
// The edges sweep must NOT have — the budget and the ledger — are guarded in
// isolation_test.go, beside the other dependency-direction tests.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sweepPkg is the import path for the sweep domain module.
const sweepPkg = internalPkg + "/sweep"

// TestPC3_SweepModuleImportsNoProviderClient asserts that the sweep module
// (and its adapters) never imports the plaid or fakebank provider packages.
// Sweep reads account balances through the accounts domain service, not by
// reaching the bank directly. Breaking this boundary would couple the
// recommendation logic to a specific provider.
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

// TestPC3_PlaidProviderSurfaceMovesNoMoneyAndReadsNoLoanDetail reads all
// production source files in the plaid provider package and asserts the
// provider surface stays read-only and narrow.
//
// Money movement is the permanent constraint: the app never initiates a
// transfer or payment, so those endpoints must never appear.
//
// /liabilities is deliberately NOT forbidden. [ADR-0024] narrowed the
// liabilities non-goal to loan APR and interest detail so that billing-cycle
// facts could date a card's obligation, and [ADR-0026] makes that detail an
// enhancement the sweep degrades without. What remains forbidden is the detail
// itself — an APR or interest field decoded off that response would reopen the
// non-goal the narrowing kept closed.
func TestPC3_PlaidProviderSurfaceMovesNoMoneyAndReadsNoLoanDetail(t *testing.T) {
	// Relative to the architecture package directory (src/internal/architecture),
	// the plaid package is one level up.
	plaidDir := "../plaid"

	entries, err := os.ReadDir(plaidDir)
	if err != nil {
		t.Fatalf("PC3: could not read plaid directory %q: %v", plaidDir, err)
	}

	// Money movement is checked against the raw source: an endpoint path is a
	// string literal, and no legitimate prose needs it.
	forbiddenEndpoints := []string{"/transfer", "/payment"}

	// Loan and interest detail is checked against the *declared json tags*, not
	// raw text — the category is what matters (the liabilities response carries
	// far more than any single field), and naming a category in a substring
	// scan would trip on a doc-comment explaining what is deliberately not
	// decoded. A tag is what actually pulls a field into the app.
	forbiddenTagParts := []string{
		"apr",
		"interest",
		"student",
		"mortgage",
		"origination",
		"minimum_payment",
	}
	jsonTag := regexp.MustCompile(`json:"([^",]*)`)

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

		for _, pattern := range forbiddenEndpoints {
			if strings.Contains(content, pattern) {
				t.Errorf("PC3: plaid/%s reaches forbidden endpoint %q — the app "+
					"initiates no money movement", name, pattern)
			}
		}

		for _, m := range jsonTag.FindAllStringSubmatch(content, -1) {
			tag := strings.ToLower(m[1])
			for _, part := range forbiddenTagParts {
				if strings.Contains(tag, part) {
					t.Errorf("PC3: plaid/%s decodes json field %q — loan and interest "+
						"detail remain a non-goal, which [ADR-0024]'s narrowing kept "+
						"closed when it opened billing-cycle facts", name, m[1])
				}
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
		t.Fatalf("PC3 anchor: /transactions/sync not found in plaid/ — " +
			"the endpoint check is targeting the wrong directory or the provider surface changed unexpectedly")
	}
}
