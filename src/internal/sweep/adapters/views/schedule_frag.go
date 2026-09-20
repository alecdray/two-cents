package views

// scheduleRegionID is the DOM id of the region [ScheduleFrag] renders. It swaps
// independently of the snapshot region ([SweepRegionID]): a schedule edit
// replaces the schedule and leaves the snapshot alone.
const scheduleRegionID = "sweep-schedule"

// ScheduleRegionID returns the schedule region's DOM id — the hx-target every
// schedule mutation swaps its re-render into.
func ScheduleRegionID() string { return scheduleRegionID }
