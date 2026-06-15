// Package architecture holds cross-cutting tests that assert structural
// properties of the codebase as a whole, rather than the behaviour of any one
// module. These tests read the import graph and fail when a boundary the
// design depends on is crossed.
package architecture

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePath        = "github.com/alecdray/two-cents"
	internalPkg       = modulePath + "/src/internal"
	plaidPkg          = internalPkg + "/plaid"
	fakebankPkg       = internalPkg + "/fakebank"
	bankingPkg        = internalPkg + "/banking"
	serverPkg         = internalPkg + "/server"
	accountsPkg       = internalPkg + "/accounts"
	transactionsPkg   = internalPkg + "/transactions"
	categorizationPkg = internalPkg + "/categorization"
)

// pkg is the slice of `go list -json` output this test cares about: a package's
// import path plus the import paths it (and its test files) pull in.
type pkg struct {
	ImportPath  string
	Imports     []string
	TestImports []string
}

// listInternalPackages shells out to `go list -json` for every package under
// src/internal/... and decodes the stream. Shelling to the toolchain is the
// robust, idiomatic way to read the real import graph the compiler sees,
// including build-tag and platform resolution, without taking on the
// golang.org/x/tools dependency.
func listInternalPackages(t *testing.T) []pkg {
	t.Helper()

	// Run from the repo root so the module-relative pattern resolves. The test
	// binary runs in this package's dir (src/internal/architecture), so climb
	// three levels back to the root.
	cmd := exec.Command("go", "list", "-json", "./src/internal/...")
	cmd.Dir = "../../.."

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list failed: %v", err)
	}

	var pkgs []pkg
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("decoding go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		t.Fatal("go list returned no packages under src/internal/...")
	}
	return pkgs
}

// allImports merges a package's production and test imports — the seam must
// hold for test code too, since a test that reaches into plaid would couple a
// consumer to the provider just as surely as production code would.
func allImports(p pkg) []string {
	merged := make([]string, 0, len(p.Imports)+len(p.TestImports))
	merged = append(merged, p.Imports...)
	merged = append(merged, p.TestImports...)
	return merged
}

// TestProviderIsolation asserts the provider seam holds across the whole
// internal tree: no package other than plaid itself may import plaid, and the
// banking seam must import neither plaid nor any third-party Plaid dependency.
func TestProviderIsolation(t *testing.T) {
	pkgs := listInternalPackages(t)

	// Guard against the test silently passing because the assertions never ran
	// against the packages they name. Confirm the anchor packages are present:
	// plaid (the provider), banking (the seam), and accounts — the headline
	// consumer that must reach the bank only through the seam. Naming accounts
	// explicitly means dropping it from the graph fails loudly here rather than
	// quietly removing it from the "no package imports plaid" sweep below.
	var sawPlaid, sawFakebank, sawBanking, sawAccounts, sawTransactions bool
	for _, p := range pkgs {
		switch p.ImportPath {
		case plaidPkg:
			sawPlaid = true
		case fakebankPkg:
			sawFakebank = true
		case bankingPkg:
			sawBanking = true
		case accountsPkg:
			sawAccounts = true
		case transactionsPkg:
			sawTransactions = true
		}
	}
	if !sawPlaid {
		t.Fatalf("plaid package %q not found in the import graph; the test is not exercising what it claims", plaidPkg)
	}
	if !sawFakebank {
		t.Fatalf("fakebank package %q not found in the import graph; the test is not exercising what it claims", fakebankPkg)
	}
	if !sawBanking {
		t.Fatalf("banking package %q not found in the import graph; the test is not exercising what it claims", bankingPkg)
	}
	if !sawAccounts {
		t.Fatalf("accounts package %q not found in the import graph; the test is not exercising what it claims", accountsPkg)
	}
	if !sawTransactions {
		t.Fatalf("transactions package %q not found in the import graph; the test is not exercising what it claims", transactionsPkg)
	}

	t.Run("the accounts consumer imports no provider client", func(t *testing.T) {
		var accounts *pkg
		for i := range pkgs {
			if pkgs[i].ImportPath == accountsPkg {
				accounts = &pkgs[i]
				break
			}
		}
		if accounts == nil {
			t.Fatalf("accounts package %q not found", accountsPkg)
		}
		for _, imp := range allImports(*accounts) {
			if imp == plaidPkg || imp == fakebankPkg {
				t.Errorf("accounts imports the %q provider package; it must reach the bank only through the banking seam", imp)
			}
			if strings.Contains(strings.ToLower(imp), "plaid") && imp != plaidPkg {
				t.Errorf("accounts imports a Plaid-named dependency %q; it must reach the bank only through the banking seam", imp)
			}
		}
	})

	t.Run("the transactions consumer imports no provider client", func(t *testing.T) {
		var txns *pkg
		for i := range pkgs {
			if pkgs[i].ImportPath == transactionsPkg {
				txns = &pkgs[i]
				break
			}
		}
		if txns == nil {
			t.Fatalf("transactions package %q not found", transactionsPkg)
		}
		for _, imp := range allImports(*txns) {
			if imp == plaidPkg || imp == fakebankPkg {
				t.Errorf("transactions imports the %q provider package; it must reach the bank only through the banking seam", imp)
			}
			if strings.Contains(strings.ToLower(imp), "plaid") && imp != plaidPkg {
				t.Errorf("transactions imports a Plaid-named dependency %q; it must reach the bank only through the banking seam", imp)
			}
		}
	})

	// Each provider client is fenced identically: only the client itself and the
	// composition root (manual DI at the root) may import it; no domain or
	// adapter package may reach a concrete provider — they depend on the banking
	// seam alone.
	for _, providerPkg := range []string{plaidPkg, fakebankPkg} {
		providerPkg := providerPkg
		t.Run("no internal package outside "+providerPkg+" and the composition root imports it", func(t *testing.T) {
			for _, p := range pkgs {
				if p.ImportPath == providerPkg {
					continue // a provider may, of course, refer to itself.
				}
				if p.ImportPath == serverPkg {
					continue // the composition root constructs the concrete provider and injects it through the seam.
				}
				for _, imp := range allImports(p) {
					if imp == providerPkg {
						t.Errorf("%s imports the %q provider package; consumers must depend only on the banking seam, never on a provider directly", p.ImportPath, providerPkg)
					}
				}
			}
		})
	}

	t.Run("banking imports neither plaid nor any Plaid dependency", func(t *testing.T) {
		var banking *pkg
		for i := range pkgs {
			if pkgs[i].ImportPath == bankingPkg {
				banking = &pkgs[i]
				break
			}
		}
		if banking == nil {
			t.Fatalf("banking package %q not found", bankingPkg)
		}

		for _, imp := range allImports(*banking) {
			if imp == plaidPkg || imp == fakebankPkg {
				t.Errorf("banking imports the %q provider package; the seam must depend on no provider client", imp)
			}
			// Catch any third-party Plaid client SDK sneaking in (e.g. a
			// github.com/plaid/... module). The seam is provider-agnostic, so a
			// Plaid-named dependency anywhere in its imports is a leak.
			if strings.Contains(strings.ToLower(imp), "plaid") && imp != plaidPkg {
				t.Errorf("banking imports a Plaid-named dependency %q; the seam must stay provider-agnostic", imp)
			}
		}
	})
}

// TestSyncDependencyDirection asserts the one-way module dependency the sync
// orchestration rests on: transactions imports accounts, and accounts (with
// every package beneath it, including its adapters) imports transactions never.
// Accounts is a leaf with respect to transactions; the connect/reconnect
// backfill runs through a server-wired seam, so the connect handlers trigger a
// sync without accounts ever reaching into transactions. Guarding the forbidden
// direction tree-wide keeps the module graph an acyclic DAG.
func TestSyncDependencyDirection(t *testing.T) {
	pkgs := listInternalPackages(t)

	// Anchor presence guard: confirm both ends of the relationship are in the
	// graph before asserting anything about them, so dropping (or renaming)
	// either package fails loudly here rather than shrinking the sweep to nothing.
	var sawAccounts, sawTransactions bool
	for _, p := range pkgs {
		switch p.ImportPath {
		case accountsPkg:
			sawAccounts = true
		case transactionsPkg:
			sawTransactions = true
		}
	}
	if !sawAccounts {
		t.Fatalf("accounts package %q not found in the import graph; the test is not exercising what it claims", accountsPkg)
	}
	if !sawTransactions {
		t.Fatalf("transactions package %q not found in the import graph; the test is not exercising what it claims", transactionsPkg)
	}

	t.Run("accounts and everything under it imports transactions never", func(t *testing.T) {
		// The boundary constrains production code: accounts must never compile in a
		// dependency on transactions. Reading the real Imports the compiler sees
		// (not just a grep) means a transitive or aliased import is caught too.
		var checked int
		for _, p := range pkgs {
			if p.ImportPath != accountsPkg && !strings.HasPrefix(p.ImportPath, accountsPkg+"/") {
				continue
			}
			checked++
			for _, imp := range p.Imports {
				if imp == transactionsPkg || strings.HasPrefix(imp, transactionsPkg+"/") {
					t.Errorf("%s imports the transactions package %q; accounts must never import transactions — the sync runs transactions→accounts only, with connect/reconnect backfill injected through a server-wired seam", p.ImportPath, imp)
				}
			}
		}
		if checked == 0 {
			t.Fatalf("no packages under %q were checked; the import-graph sweep matched nothing", accountsPkg)
		}
	})

	t.Run("transactions imports accounts", func(t *testing.T) {
		// The allowed direction must actually hold: if transactions stopped
		// importing accounts the orchestration above would be vacuous, so assert
		// the edge exists rather than only forbidding its reverse.
		var txns *pkg
		for i := range pkgs {
			if pkgs[i].ImportPath == transactionsPkg {
				txns = &pkgs[i]
				break
			}
		}
		if txns == nil {
			t.Fatalf("transactions package %q not found", transactionsPkg)
		}
		var importsAccounts bool
		for _, imp := range txns.Imports {
			if imp == accountsPkg {
				importsAccounts = true
				break
			}
		}
		if !importsAccounts {
			t.Errorf("transactions does not import accounts; the sync's transactions→accounts dependency edge is missing")
		}
	})
}

// TestCategorizationDependencyDirection asserts categorization's place in the
// module graph: it is the decider, not a writer, so it must never import the
// transactions module (its one cross-domain write goes through a server-wired
// seam) nor any provider client. Guarding the forbidden directions tree-wide
// across categorization and everything under it keeps the graph acyclic and the
// module provider-agnostic. The allowed transactions→categorization edge is
// asserted too: transactions consults the decider on every sync and to populate
// the re-categorize picker, so that edge must hold or the wiring is vacuous.
func TestCategorizationDependencyDirection(t *testing.T) {
	pkgs := listInternalPackages(t)

	// Anchor presence guard: confirm both ends of the relationship are in the
	// graph before asserting anything about them, so dropping (or renaming) either
	// package fails loudly here rather than shrinking the sweep to nothing.
	var sawCategorization, sawTransactions bool
	for _, p := range pkgs {
		switch p.ImportPath {
		case categorizationPkg:
			sawCategorization = true
		case transactionsPkg:
			sawTransactions = true
		}
	}
	if !sawCategorization {
		t.Fatalf("categorization package %q not found in the import graph; the test is not exercising what it claims", categorizationPkg)
	}
	if !sawTransactions {
		t.Fatalf("transactions package %q not found in the import graph; the test is not exercising what it claims", transactionsPkg)
	}

	t.Run("categorization and everything under it imports neither transactions nor a provider", func(t *testing.T) {
		var checked int
		for _, p := range pkgs {
			if p.ImportPath != categorizationPkg && !strings.HasPrefix(p.ImportPath, categorizationPkg+"/") {
				continue
			}
			checked++
			for _, imp := range allImports(p) {
				if imp == transactionsPkg || strings.HasPrefix(imp, transactionsPkg+"/") {
					t.Errorf("%s imports the transactions package %q; categorization decides but never writes transactions — its re-categorize write goes through a server-wired seam", p.ImportPath, imp)
				}
				if imp == plaidPkg || imp == fakebankPkg {
					t.Errorf("%s imports the %q provider package; categorization is provider-agnostic", p.ImportPath, imp)
				}
				if strings.Contains(strings.ToLower(imp), "plaid") && imp != plaidPkg {
					t.Errorf("%s imports a Plaid-named dependency %q; categorization must stay provider-agnostic", p.ImportPath, imp)
				}
			}
		}
		if checked == 0 {
			t.Fatalf("no packages under %q were checked; the import-graph sweep matched nothing", categorizationPkg)
		}
	})

	t.Run("transactions imports categorization", func(t *testing.T) {
		// The allowed direction must actually hold: transactions is the writer that
		// asks the categorization decider to classify each synced row and reads its
		// taxonomy for the picker. If that edge vanished the auto-categorize wiring
		// would be gone, so assert the edge exists rather than only forbidding its
		// reverse.
		var txns *pkg
		for i := range pkgs {
			if pkgs[i].ImportPath == transactionsPkg {
				txns = &pkgs[i]
				break
			}
		}
		if txns == nil {
			t.Fatalf("transactions package %q not found", transactionsPkg)
		}
		var importsCategorization bool
		for _, imp := range txns.Imports {
			if imp == categorizationPkg {
				importsCategorization = true
				break
			}
		}
		if !importsCategorization {
			t.Errorf("transactions does not import categorization; the auto-categorize-on-sync dependency edge is missing")
		}
	})
}
