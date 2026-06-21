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

// TransactionRowID returns the DOM id of one transaction's row — the
// re-categorize form's hx-target, so a categorize swap replaces just that row in
// place. The row owns the id here so the form and the row agree on it.
func TransactionRowID(txnID string) string { return "txn-row-" + txnID }

// TransferDestinationOptionID returns the testid of one connected-account option
// in the transfer-destination picker, so a test can target a specific destination
// account by id rather than by display text.
func TransferDestinationOptionID(accountID string) string {
	return "txn-destination-option-" + accountID
}

// ViewNeedsAttentionParam is the `?view=` value (and hidden-field value) that
// selects the needs-attention worklist on /transactions. Empty / any other value
// is the default "All" view. The handler and the view share this one constant so
// the toggle link, the search box, and the resolve forms all agree on the wire
// value the handler parses.
const ViewNeedsAttentionParam = "needs-attention"

// ListControls is the /transactions view state the page and its fragments render
// against: the current merchant search text and whether the needs-attention
// worklist is active. It drives the search box value, the toggle's active tab, the
// filtered empty state, and — via NeedsAttentionView — whether a per-row resolve
// swaps the row in place (All view) or re-renders the whole region so a resolved
// row drops out of the worklist (needs-attention view).
type ListControls struct {
	Query              string
	NeedsAttentionView bool
}

// FilterActive reports whether either facet narrows the list — the signal to show
// the controls bar + a filtered empty state rather than the nothing-synced state.
func (c ListControls) FilterActive() bool {
	return c.Query != "" || c.NeedsAttentionView
}

// ViewValue is the hidden `view` field value the search box and resolve forms
// carry so a re-render stays in the current view (empty = the default All view).
func (c ListControls) ViewValue() string {
	if c.NeedsAttentionView {
		return ViewNeedsAttentionParam
	}
	return ""
}

// RowResolveTarget / RowResolveSwap pick where a per-row resolve (re-categorize or
// mark-destination) swaps its response. In the All view the row is replaced in
// place; in the needs-attention worklist the whole region is re-rendered so a
// resolved row drops out and the month headers + empty state recompute (the
// view-aware-handlers invariant in the module CLAUDE.md).
func (c ListControls) RowResolveTarget(rowID string) string {
	if c.NeedsAttentionView {
		return "#" + transactionsRegionID
	}
	return "#" + TransactionRowID(rowID)
}

// RowResolveSwap is the htmx swap that pairs with RowResolveTarget.
func (c ListControls) RowResolveSwap() string {
	if c.NeedsAttentionView {
		return "innerHTML"
	}
	return "outerHTML"
}
