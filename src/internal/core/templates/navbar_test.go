package templates

import "testing"

func TestNavTabIsMore(t *testing.T) {
	overflow := []NavTab{NavWraps, NavCategories, NavRules}
	for _, tab := range overflow {
		if !tab.isMore() {
			t.Errorf("tab %d should be an overflow (More) destination", tab)
		}
	}
	primary := []NavTab{NavHome, NavTransactions, NavBudget, NavAccounts}
	for _, tab := range primary {
		if tab.isMore() {
			t.Errorf("tab %d should be a primary bar destination, not overflow", tab)
		}
	}
}
