package templates

// NavTab identifies the application surface a page belongs to, so AppNavbar can
// mark the matching bottom-bar slot active. It is the navbar's own enum of
// destinations — not a domain type — which keeps the navbar a domain-free
// primitive (ADR-0014).
type NavTab int

// The iota values carry no display meaning; AppNavbar decides slot order and
// which tabs sit in the overflow.
const (
	NavHome NavTab = iota
	NavTransactions
	NavBudget
	NavAccounts
	NavWraps
	NavCategories
	NavRules
)

// isMore reports whether the tab lives in the overflow sheet rather than on the
// bar, so AppNavbar highlights the More control when one of them is current.
func (t NavTab) isMore() bool {
	return t == NavWraps || t == NavCategories || t == NavRules
}
