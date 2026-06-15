package views

// trackerRegionID is the DOM id of the Tracker page's root region — the <main>
// the page renders its content into. The page owns the id here so any future
// in-place swap names the region without hard-coding the string at the call site.
const trackerRegionID = "tracker"

// TrackerRegionID returns the Tracker region's DOM id.
func TrackerRegionID() string { return trackerRegionID }
