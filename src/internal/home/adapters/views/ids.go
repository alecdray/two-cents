package views

// trackerRegionID is the DOM id of the Tracker page's root region — the <main>
// the page renders its content into. The page owns the id here so any future
// in-place swap names the region without hard-coding the string at the call site.
const trackerRegionID = "tracker"

// TrackerRegionID returns the Tracker region's DOM id.
func TrackerRegionID() string { return trackerRegionID }

// trackerFigureRegionID is the DOM id of the Tracker's self-refreshing inner
// region — the figure tiers (or the no-budget actuals) plus the inline
// All-transactions list. Editing a row in the list can shift any figure, so the
// whole region self-refreshes on the transaction-changed event, mirroring the
// wrap's figure region. The month rail + label sit outside it (an edit cannot
// change them).
const trackerFigureRegionID = "tracker-figures"

// TrackerFigureRegionID returns the Tracker figure region's DOM id.
func TrackerFigureRegionID() string { return trackerFigureRegionID }

// spendDrillRegionID is the DOM id of the spend drill-down's swap region — the
// net-total header plus the list. A re-categorize swaps this region's inner HTML,
// so the total stays reconciled to the rows when an edit moves a row out of the
// bucket.
const spendDrillRegionID = "spend-drill"

// SpendDrillRegionID returns the spend drill-down region's DOM id.
func SpendDrillRegionID() string { return spendDrillRegionID }

// wrapFigureRegionID is the DOM id of the wrap's figure region — every figure
// (net/gross income, savings, spend-by-Category) plus the inline full-month list.
// Editing a row in the list can shift any figure, so the whole region self-refreshes
// on the transaction-changed event.
const wrapFigureRegionID = "wrap-figures"

// WrapFigureRegionID returns the wrap figure region's DOM id.
func WrapFigureRegionID() string { return wrapFigureRegionID }
