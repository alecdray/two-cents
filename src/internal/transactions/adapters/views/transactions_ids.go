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

// transactionEditorModalID names the transaction-editing modal's <dialog> (so the
// shell's close form can dismiss it); transactionEditorRegionID is its body region —
// the editor's own controls swap their re-render into it so the modal stays open
// across edits. Both ids are owned here once, shared by the GET /edit render and the
// re-render the categorize / transfer-destination saves return.
const (
	transactionEditorModalID  = "transaction-editor-modal"
	transactionEditorRegionID = "transaction-editor"
)

// TransactionEditorRegionID returns the editor body region's DOM id — the hx-target
// the editor's controls swap their re-render into.
func TransactionEditorRegionID() string { return transactionEditorRegionID }

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

// ViewValue is the hidden `view` field value the search box carries so a re-render
// (a toggle, a search, or a transaction-changed self-refresh) stays in the current
// view (empty = the default All view).
func (c ListControls) ViewValue() string {
	if c.NeedsAttentionView {
		return ViewNeedsAttentionParam
	}
	return ""
}
