package adapters

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/core/templates"
	"github.com/alecdray/two-cents/src/internal/schedule"
	"github.com/alecdray/two-cents/src/internal/sweep"
	"github.com/alecdray/two-cents/src/internal/sweep/adapters/views"
)

// HttpHandler serves the sweep snapshot history and the schedule it is computed
// from. Snapshot reads render a stored result and never trigger a compute; the
// one path that computes is the explicit Run now action.
//
// The schedule surface lives here rather than in a schedule adapter because the
// sweep is its only consumer, and a page may not render another module's views.
type HttpHandler struct {
	sweep    *sweep.Service
	schedule *schedule.Service
}

// NewHttpHandler builds the handler over the sweep Service and the schedule
// Service whose items the /sweep page manages.
func NewHttpHandler(sweepSvc *sweep.Service, scheduleSvc *schedule.Service) *HttpHandler {
	return &HttpHandler{sweep: sweepSvc, schedule: scheduleSvc}
}

// GetPage renders the newest snapshot at /sweep. Before any run has been stored
// it shows the first-run empty state, which is distinct from a stored
// needs-attention snapshot.
func (h *HttpHandler) GetPage(w http.ResponseWriter, r *http.Request) {
	h.renderSnapshot(w, r, "")
}

// GetSnapshot renders one snapshot by id at /sweep/{id} — the deep link, and
// where the older/newer steps point. An id that is not in the history is a 404
// rather than a silent fall back to the newest, which would show the reader a
// different snapshot than the one they asked for.
func (h *HttpHandler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	h.renderSnapshot(w, r, r.PathValue("id"))
}

// PostRun computes a fresh recommendation, appends it, and swaps the snapshot
// region to the result — the only path on the page that computes. The snapshot's
// own address is pushed so the fresh result is bookmarkable and the back button
// returns to the snapshot that was on screen before.
//
// A failed run re-renders the same region with a recoverable inline error rather
// than an error page: the run is a read over live data and retrying is often all
// it takes, so the snapshot already on screen stays in view.
func (h *HttpHandler) PostRun(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	rec, err := h.sweep.Run(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to run sweep", "error", err)
		// The snapshot the control was rendered with, not the newest: a reader
		// stepping back through the history must not be swapped onto a different
		// snapshot than the one the address bar still names.
		h.renderRegion(ctx, w, r.FormValue("snapshot"), "We couldn't run the sweep. Please try again.")
		return
	}

	w.Header().Set("HX-Push-Url", "/sweep/"+rec.ID)
	h.renderRegion(ctx, w, rec.ID, "")
}

// PostScheduleItem declares a new scheduled item and swaps the schedule region
// back. A rejected declaration re-renders the region with the message beside the
// add form and nothing stored.
func (h *HttpHandler) PostScheduleItem(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	item, err := parseScheduleItem(r)
	if err == nil {
		// A freshly declared item is on the timeline. The add form carries no
		// active toggle because declaring something in order to leave it switched
		// off is not a thing anyone means to do — the toggle exists on a row, to
		// retire a commitment later without losing what was declared.
		item.Active = true
		_, err = h.schedule.Create(ctx, item)
	}
	if err != nil {
		h.renderScheduleFailure(ctx, w, "", err)
		return
	}
	h.renderSchedule(ctx, w, views.ScheduleProps{})
}

// GetEditScheduleItemModal opens the per-item editor in edit mode: the shared
// modal shell ([ADR-0011]) loaded with that item's complete declaration form,
// its active toggle, its delete control, and its existing occurrence-matching
// history and controls — everything the compact row moved out of inline view
// ([ADR-0031]) — carrying any validated same-origin return handle the opener
// passed ([ADR-0016]). The opener hx-gets this with hx-swap="none"; the Modal's
// OOB container mounts it. An id that names no declared item is a 404 — the
// modal has nothing left to edit.
func (h *HttpHandler) GetEditScheduleItemModal(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")

	item, window, found, err := h.scheduleItemEditorData(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	if !found {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusNotFound})
		return
	}

	views.ScheduleItemEditorModalFrag(views.ScheduleItemEditorProps{
		Item:     item,
		Window:   window,
		ReturnTo: validReturnHandle(r.FormValue("return_to")),
	}).Render(ctx, w)
}

// PutScheduleItem saves an edit from the item's modal. The active toggle rides
// the same form, so turning an item off is an ordinary edit rather than its own
// verb. A validation failure re-renders the editor body in place (the modal
// stays open, the stored item unchanged since nothing was written); a success
// dispatches to scheduleItemMutated — return to origin, or close the modal and
// refresh the list — the same shape the Rule editor's own save already
// established ([ADR-0016]).
func (h *HttpHandler) PutScheduleItem(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")
	returnTo := validReturnHandle(r.FormValue("return_to"))

	item, err := parseScheduleItem(r)
	if err == nil {
		item.ID = id
		err = h.schedule.Update(ctx, item)
	}
	if err != nil {
		ve, ok := schedule.IsValidationError(err)
		if !ok {
			httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
				Status: http.StatusInternalServerError,
				Err:    err,
			})
			return
		}
		h.renderScheduleItemEditor(ctx, w, id, returnTo, ve.Message)
		return
	}
	h.scheduleItemMutated(ctx, w, returnTo)
}

// DeleteScheduleItem removes one declared item from the schedule and dispatches
// to scheduleItemMutated — return to origin, or close the modal and refresh the
// list.
func (h *HttpHandler) DeleteScheduleItem(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")
	returnTo := validReturnHandle(r.FormValue("return_to"))

	if err := h.schedule.Delete(ctx, id); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	h.scheduleItemMutated(ctx, w, returnTo)
}

// PostOccurrenceMatch records that a checking transaction satisfied one
// occurrence — the guaranteed path, which automatic resolution may never
// overwrite. Its response re-renders the item's editor body in place: a
// decision made mid-edit is reflected there, not treated as a save or dismiss,
// so the modal stays open.
func (h *HttpHandler) PostOccurrenceMatch(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id, occurrence, ok := occurrenceTarget(ctx, w, r)
	if !ok {
		return
	}
	returnTo := validReturnHandle(r.FormValue("return_to"))

	transactionID := strings.TrimSpace(r.FormValue("transaction_id"))
	if transactionID == "" {
		h.renderScheduleItemEditor(ctx, w, id, returnTo, "Choose the transaction that paid it.")
		return
	}

	if err := h.schedule.MatchOccurrence(ctx, id, occurrence, transactionID); err != nil {
		// The picker never offers a row that already settles something, so this is
		// a race with another tab rather than a mistake the user can see. It is
		// still theirs to recover from, not a server error page.
		slog.ErrorContext(ctx, "failed to match occurrence", "error", err, "item", id)
		h.renderScheduleItemEditor(ctx, w, id, returnTo, "That transaction already settles another occurrence. Pick a different one.")
		return
	}
	h.renderScheduleItemEditor(ctx, w, id, returnTo, "")
}

// PostOccurrenceConfirm promotes an automatic match to the user's own decision,
// which is what takes it out of resolution's reach. Its response re-renders the
// item's editor body in place, keeping the modal open.
func (h *HttpHandler) PostOccurrenceConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id, occurrence, ok := occurrenceTarget(ctx, w, r)
	if !ok {
		return
	}
	returnTo := validReturnHandle(r.FormValue("return_to"))

	if err := h.schedule.ConfirmOccurrence(ctx, id, occurrence); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	h.renderScheduleItemEditor(ctx, w, id, returnTo, "")
}

// PostOccurrenceClear records the user's assertion that nothing satisfied this
// occurrence. It stays on the timeline, and resolution leaves it alone. Its
// response re-renders the item's editor body in place, keeping the modal open.
func (h *HttpHandler) PostOccurrenceClear(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id, occurrence, ok := occurrenceTarget(ctx, w, r)
	if !ok {
		return
	}
	returnTo := validReturnHandle(r.FormValue("return_to"))

	if err := h.schedule.ClearOccurrence(ctx, id, occurrence); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	h.renderScheduleItemEditor(ctx, w, id, returnTo, "")
}

// occurrenceTarget reads the item and occurrence a decision is about off the
// path. A date that will not parse is a broken link rather than something the
// user can fix in the form, so it is a 400 and not an inline message.
func occurrenceTarget(ctx contextx.ContextX, w http.ResponseWriter, r *http.Request) (string, time.Time, bool) {
	occurrence, err := time.Parse("2006-01-02", r.PathValue("date"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusBadRequest,
			Err:    err,
		})
		return "", time.Time{}, false
	}
	return r.PathValue("id"), occurrence, true
}

// renderScheduleFailure re-renders the schedule region carrying a recoverable
// validation message, scoped to the row it belongs to (empty id = the add form).
// Anything that is not a validation error is a real server failure.
func (h *HttpHandler) renderScheduleFailure(ctx contextx.ContextX, w http.ResponseWriter, id string, err error) {
	ve, ok := schedule.IsValidationError(err)
	if !ok {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	props := views.ScheduleProps{}
	if id == "" {
		props.AddError = ve.Message
	} else {
		props.RowErrID, props.RowErrMsg = id, ve.Message
	}
	h.renderSchedule(ctx, w, props)
}

// renderSchedule renders the schedule region over the currently stored items,
// carrying whatever inline message props already holds.
func (h *HttpHandler) renderSchedule(ctx contextx.ContextX, w http.ResponseWriter, props views.ScheduleProps) {
	scheduled, err := h.scheduleViewData(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	props.Items = scheduled.Items
	props.Windows = scheduled.Windows
	props.AmountStates = scheduled.AmountStates
	props.Next = scheduled.Next

	views.ScheduleFrag(props).Render(ctx, w)
}

// scheduleViewData is everything the schedule region reads to render, beyond
// whatever inline message a particular request already carries — read once so
// the page's initial load and every schedule mutation build the same set of
// props the same way.
type scheduleViewData struct {
	Items        []schedule.Item
	Windows      []schedule.ItemOccurrences
	AmountStates map[string]schedule.AmountState
	Next         map[string]time.Time
}

// scheduleItemEditorData reads one declared item and its occurrences inside
// the matching window — everything the per-item modal renders beyond whatever
// inline message a particular request already carries. found is false when the
// id names no declared item, which the caller reports as a 404: the modal has
// nothing left to edit.
func (h *HttpHandler) scheduleItemEditorData(ctx contextx.ContextX, id string) (schedule.Item, schedule.ItemOccurrences, bool, error) {
	items, err := h.schedule.List(ctx)
	if err != nil {
		return schedule.Item{}, schedule.ItemOccurrences{}, false, err
	}
	var item schedule.Item
	found := false
	for _, it := range items {
		if it.ID == id {
			item, found = it, true
			break
		}
	}
	if !found {
		return schedule.Item{}, schedule.ItemOccurrences{}, false, nil
	}

	windows, err := h.schedule.MatchingWindow(ctx)
	if err != nil {
		return schedule.Item{}, schedule.ItemOccurrences{}, false, err
	}
	var window schedule.ItemOccurrences
	for _, w := range windows {
		if w.Item.ID == id {
			window = w
			break
		}
	}
	return item, window, true, nil
}

// renderScheduleItemEditor re-renders one item's editor body in place: the
// stored item, its occurrence window, and the return handle it was opened
// with, carrying an optional recoverable validation message (empty for a plain
// refresh, e.g. after an occurrence decision). A missing item id is a 404.
func (h *HttpHandler) renderScheduleItemEditor(ctx contextx.ContextX, w http.ResponseWriter, id, returnTo, errMsg string) {
	item, window, found, err := h.scheduleItemEditorData(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	if !found {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusNotFound})
		return
	}
	views.ScheduleItemEditorContentFrag(views.ScheduleItemEditorProps{
		Item:     item,
		Window:   window,
		ReturnTo: returnTo,
		Error:    errMsg,
	}).Render(ctx, w)
}

// scheduleItemMutated writes the response shared by a modal save and delete.
// With a valid return handle it returns the self-firing return loader, which
// re-mounts the origin on load. With no handle (opened from /sweep itself) it
// closes the modal and refreshes the schedule list with the current state in
// one response: it retargets the swap to the schedule region, emits an
// OOB-empty modal container to clear the modal, then the refreshed schedule —
// the same two-case shape the Rule editor's own save/delete already
// established ([ADR-0016]).
func (h *HttpHandler) scheduleItemMutated(ctx contextx.ContextX, w http.ResponseWriter, returnTo string) {
	if returnTo != "" {
		views.ScheduleItemEditorReturnLoader(returnTo).Render(ctx, w)
		return
	}

	scheduled, err := h.scheduleViewData(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	w.Header().Set("HX-Retarget", "#"+views.ScheduleRegionID())
	w.Header().Set("HX-Reswap", "outerHTML")
	templates.ModalContainer(true).Render(ctx, w)
	views.ScheduleFrag(views.ScheduleProps{
		Items:        scheduled.Items,
		Windows:      scheduled.Windows,
		AmountStates: scheduled.AmountStates,
		Next:         scheduled.Next,
	}).Render(ctx, w)
}

// validReturnHandle echoes a return handle only when it is a same-origin
// relative path — a single leading slash (rejecting protocol-relative
// "//host") and no scheme ("://"). Anything else (absent, absolute,
// scheme-bearing) yields the empty string, so a handle carrying a host or
// scheme is never followed. This is a hard security property: the editor
// round-trips the handle without inspecting where it points, so the
// validation is the only guard — the same rule the Rule editor's own
// validReturnHandle enforces ([ADR-0016]).
func validReturnHandle(raw string) string {
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "://") {
		return ""
	}
	return raw
}

// scheduleViewData reads the declared schedule and everything the compact
// list's row needs to summarize it: each item's reconcilable occurrences (for
// the full item history, and a dateless item's settled-or-not month), its
// amount state (override vs. computed, [ADR-0031]), and its next projected
// date.
func (h *HttpHandler) scheduleViewData(ctx contextx.ContextX) (scheduleViewData, error) {
	items, err := h.schedule.List(ctx)
	if err != nil {
		return scheduleViewData{}, err
	}

	window, err := h.schedule.MatchingWindow(ctx)
	if err != nil {
		return scheduleViewData{}, err
	}

	states, err := h.schedule.AmountStates(ctx)
	if err != nil {
		return scheduleViewData{}, err
	}

	next := make(map[string]time.Time, len(items))
	for _, item := range items {
		if date, ok := h.schedule.NextOccurrence(item); ok {
			next[item.ID] = date
		}
	}

	return scheduleViewData{Items: items, Windows: window, AmountStates: states, Next: next}, nil
}

// renderRegion renders the snapshot region for one snapshot (empty id = newest),
// used by the run action for both its outcomes. A read failure here is a real
// error page: there is no region left to render the inline error into.
func (h *HttpHandler) renderRegion(ctx contextx.ContextX, w http.ResponseWriter, id, runError string) {
	snap, found, err := h.sweep.Snapshot(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	views.SweepSnapshotFrag(snap, found, runError).Render(ctx, w)
}

// renderSnapshot is the shared read path. id is empty for the newest snapshot.
func (h *HttpHandler) renderSnapshot(w http.ResponseWriter, r *http.Request, id string) {
	ctx := contextx.NewContextX(r.Context())

	snap, found, err := h.sweep.Snapshot(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	// An empty history is the first-run empty state; a named snapshot that is not
	// there is a broken link.
	if !found && id != "" {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusNotFound,
		})
		return
	}

	scheduled, err := h.scheduleViewData(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.SweepPage(snap, found, views.ScheduleProps{
		Items:        scheduled.Items,
		Windows:      scheduled.Windows,
		AmountStates: scheduled.AmountStates,
		Next:         scheduled.Next,
	}).Render(ctx, w)
}

// parseScheduleItem reads a declared item off the submitted form. Only shape
// errors are reported here (an amount that is not a number, a date that is not a
// date); what counts as a valid *declaration* is the schedule module's rule, so
// this leaves those to it rather than keeping a second copy that could drift.
func parseScheduleItem(r *http.Request) (schedule.Item, error) {
	item := schedule.Item{
		Name:      r.FormValue("name"),
		Direction: schedule.Direction(r.FormValue("direction")),
		Cadence:   schedule.Cadence(r.FormValue("cadence")),
		// An unchecked toggle submits nothing, which is how it reads as off.
		Active: r.FormValue("active") != "",
	}

	amount, err := parseAmount(r.FormValue("amount"))
	if err != nil {
		return schedule.Item{}, schedule.ValidationError{Message: "Enter an amount as a number."}
	}
	item.Amount = amount

	if raw := strings.TrimSpace(r.FormValue("day_of_month")); raw != "" {
		day, err := strconv.Atoi(raw)
		if err != nil {
			return schedule.Item{}, schedule.ValidationError{Message: "Enter the day of the month as a number."}
		}
		item.DayOfMonth = day
	}

	if raw := strings.TrimSpace(r.FormValue("anchor_date")); raw != "" {
		anchor, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return schedule.Item{}, schedule.ValidationError{Message: "Enter the anchor as a date."}
		}
		item.AnchorDate = anchor
	}

	return item, nil
}

// parseAmount reads a dollar figure from a form field, treating blank as no
// declared amount at all — left for the schedule to compute — rather than a
// malformed number ([ADR-0031]).
func parseAmount(raw string) (*float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	amount, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil, err
	}
	return &amount, nil
}
