package views

// budgetRegionID is the DOM id of the budget page's shared swap region — the
// <main> the page renders its content into and the save form targets for an
// innerHTML swap. The page owns the id here so the form and the page agree on
// the region name without hard-coding the string at the call site.
const budgetRegionID = "budget"

// BudgetRegionID returns the budget swap region's DOM id for callers that need
// to name the region (e.g. the save form's hx-target).
func BudgetRegionID() string { return budgetRegionID }
