package architecture

import (
	"os"
	"strings"
	"testing"
)

// TestAllSurfacesDelegateToSharedRow anchors the claim that every transaction
// surface reaches the one shared row templ (directly, or via AllTransactionsFrag),
// so editing that single component covers the row — avatar included — everywhere.
// It lives here rather than in either module's tests because it constrains the
// relationship between transactions/adapters/views and home/adapters/views, which
// neither package can assert about itself without importing its peer's adapters.
func TestAllSurfacesDelegateToSharedRow(t *testing.T) {
	cases := []struct {
		file  string
		token string
	}{
		{"../transactions/adapters/views/transactions_page.templ", "TransactionRowFrag"},
		{"../home/adapters/views/drill_page.templ", "TransactionRowFrag"},
		{"../home/adapters/views/all_transactions_frag.templ", "TransactionRowFrag"},
		{"../home/adapters/views/tracker_page.templ", "AllTransactionsFrag"},
		{"../home/adapters/views/wrap_page.templ", "AllTransactionsFrag"},
	}
	// Anchor guard: a sweep that reads nothing must not pass silently. Renaming or
	// moving a surface has to fail here, not shrink the set being checked.
	if len(cases) != 5 {
		t.Fatalf("expected 5 transaction surfaces under check, got %d", len(cases))
	}
	for _, tc := range cases {
		src, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatalf("reading %s: %v", tc.file, err)
		}
		if !strings.Contains(string(src), tc.token) {
			t.Errorf("%s should delegate via %s", tc.file, tc.token)
		}
	}
}
