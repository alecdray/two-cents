package views

// sweepRegionID is the DOM id of the swappable snapshot region — everything the
// page re-renders when a run produces a new snapshot.
const sweepRegionID = "sweep-region"

// SweepRegionID returns the snapshot region's DOM id for callers (the Run now
// control's hx-target) that need to name the region.
func SweepRegionID() string { return sweepRegionID }
