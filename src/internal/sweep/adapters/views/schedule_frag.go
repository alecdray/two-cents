package views

// scheduleRegionID is the DOM id of the region [ScheduleFrag] renders. It swaps
// independently of the snapshot region ([SweepRegionID]): a schedule edit
// replaces the schedule and leaves the snapshot alone.
const scheduleRegionID = "sweep-schedule"

// ScheduleRegionID returns the schedule region's DOM id — the hx-target every
// schedule mutation swaps its re-render into.
func ScheduleRegionID() string { return scheduleRegionID }

// The DOM ids of the per-item editor modal: the <dialog> the shared shell names
// (so its close form can dismiss it) and the editor body region a save/delete
// re-renders in place, so a validation error swaps itself back in and the modal
// stays open — the same two-id shape the Rule editor's own modal already uses
// ([ADR-0016]).
const (
	scheduleItemEditorModalID  = "schedule-item-editor-modal"
	scheduleItemEditorRegionID = "schedule-item-editor"
)

// ScheduleItemEditorRegionID returns the editor body region's DOM id — the
// hx-target every control inside the modal (the save form, the delete form,
// and every occurrence decision) swaps its re-render into.
func ScheduleItemEditorRegionID() string { return scheduleItemEditorRegionID }

// The DOM ids of the attach-to-existing-item modal ([ADR-0031]): reached from
// an unmatched checking transaction's editor, alongside the create-from-
// transaction mode of the item editor above.
const (
	scheduleAttachModalID  = "schedule-attach-modal"
	scheduleAttachRegionID = "schedule-attach"
)

// ScheduleAttachRegionID returns the attach picker's DOM id — the hx-target
// each option's form swaps its re-render into.
func ScheduleAttachRegionID() string { return scheduleAttachRegionID }
