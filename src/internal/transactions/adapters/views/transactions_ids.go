package views

// transactionsRegionID is the DOM id of the transactions page's shared swap
// region — the <main> element TransactionsPage renders its content into and the
// "Sync now" control targets. The page owns the id here so the control and the
// page agree on the region's name without hard-coding the string at the call
// site.
const transactionsRegionID = "transactions"

// TransactionsRegionID returns the transactions swap region's DOM id for callers
// (e.g. the sync control's hx-target) that need to name the region.
func TransactionsRegionID() string { return transactionsRegionID }
