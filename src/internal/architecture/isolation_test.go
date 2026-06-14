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
	modulePath  = "github.com/alecdray/two-cents"
	internalPkg = modulePath + "/src/internal"
	plaidPkg    = internalPkg + "/plaid"
	fakebankPkg = internalPkg + "/fakebank"
	bankingPkg  = internalPkg + "/banking"
	serverPkg   = internalPkg + "/server"
	accountsPkg = internalPkg + "/accounts"
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
	var sawPlaid, sawFakebank, sawBanking, sawAccounts bool
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
