package accounts

import (
	"testing"

	"github.com/alecdray/two-cents/src/internal/banking"
)

func cash(id string, countsAsSavings bool) Account {
	return Account{ID: id, Kind: banking.KindCash, CountsAsSavings: countsAsSavings}
}

// counts-as-savings is this module's flag, so "which active cash Account is
// checking" is this module's question to answer. Both the sweep and occurrence
// matching ask rather than re-deriving it ([ADR-0027]).
func TestDeriveChecking(t *testing.T) {
	t.Run("the single cash account not marked counts-as-savings is checking", func(t *testing.T) {
		got, ok := DeriveChecking([]Account{cash("a", false), cash("b", true)})
		if !ok || got.ID != "a" {
			t.Errorf("DeriveChecking = %+v/%v, want account a", got, ok)
		}
	})

	t.Run("two candidates are ambiguous, not a winner", func(t *testing.T) {
		// Designating one is a user action, and guessing would silently compute
		// against an account they did not mean.
		_, ok := DeriveChecking([]Account{cash("a", false), cash("b", false)})
		if ok {
			t.Error("DeriveChecking resolved two candidates; want undetermined")
		}
	})

	t.Run("no candidate at all is undetermined", func(t *testing.T) {
		_, ok := DeriveChecking([]Account{cash("b", true)})
		if ok {
			t.Error("DeriveChecking resolved with no candidate; want undetermined")
		}
	})
}
