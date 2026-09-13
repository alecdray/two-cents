package views

// The DOM ids of the two regions the /sweep page is built from. They swap
// independently: a run replaces the snapshot, a schedule edit replaces the
// schedule, and neither disturbs the other.
//
// A schedule edit deliberately does *not* refresh the snapshot. A snapshot is an
// immutable record of what was advised at an instant ([ADR-0022]); changing the
// schedule afterwards does not change what that run said, and re-running is how
// the user sees the effect.
const (
	sweepRegionID    = "sweep-region"
	scheduleRegionID = "sweep-schedule"
)

// SweepRegionID returns the snapshot region's DOM id for callers (the Run now
// control's hx-target) that need to name the region.
func SweepRegionID() string { return sweepRegionID }

// ScheduleRegionID returns the schedule region's DOM id — the hx-target every
// schedule mutation swaps its re-render into.
func ScheduleRegionID() string { return scheduleRegionID }
