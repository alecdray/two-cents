package views

// trackerRegionID is the DOM id of the Tracker page's root region — the <main>
// the page renders its content into. The page owns the id here so any future
// in-place swap names the region without hard-coding the string at the call site.
const trackerRegionID = "tracker"

// TrackerRegionID returns the Tracker region's DOM id.
func TrackerRegionID() string { return trackerRegionID }

// spendDrillRegionID is the DOM id of the spend drill-down's swap region — the
// net-total header plus the list. A re-categorize swaps this region's inner HTML,
// so the total stays reconciled to the rows when an edit moves a row out of the
// bucket.
const spendDrillRegionID = "spend-drill"

// SpendDrillRegionID returns the spend drill-down region's DOM id.
func SpendDrillRegionID() string { return spendDrillRegionID }
