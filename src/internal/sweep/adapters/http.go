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

// PostScheduleItem declares a new scheduled item, two ways. Posted with no
// transaction_id (the ordinary add form on /sweep) it validates and stores a
// plain declaration, swapping the schedule region back — a rejected
// declaration re-renders the region with the message beside the add form and
// nothing stored. Posted with a transaction_id (the create-from-transaction
// modal opened from a checking transaction's editor, [ADR-0031]) it instead
// calls CreateItemFromTransaction — which seeds direction and date from that
// transaction and records it as the item's first match in the same step — and
// dispatches through scheduleItemMutated (return to origin, or close the
// modal and refresh the list), the same shape every other modal save uses
// ([ADR-0016]); a rejected declaration re-renders the create editor in place.
func (h *HttpHandler) PostScheduleItem(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	transactionID := strings.TrimSpace(r.FormValue("transaction_id"))
	returnTo := validReturnHandle(r.FormValue("return_to"))

	item, err := parseScheduleItem(r)
	if err == nil {
		// A freshly declared item is on the timeline. Neither form carries an
		// active toggle: declaring something in order to leave it switched off is
		// not a thing anyone means to do — the toggle exists on a row, to retire a
		// commitment later without losing what was declared.
		item.Active = true
		if transactionID != "" {
			_, err = h.schedule.CreateItemFromTransaction(ctx, transactionID, item)
		} else {
			_, err = h.schedule.Create(ctx, item)
		}
	}
	if err != nil {
		if transactionID != "" {
			h.renderNewScheduleItemEditor(ctx, w, item, transactionID, returnTo, err)
			return
		}
		h.renderScheduleFailure(ctx, w, "", err)
		return
	}
	if transactionID != "" {
		h.scheduleItemMutated(ctx, w, returnTo)
		return
	}
	h.renderSchedule(ctx, w, views.ScheduleProps{})
}

// GetNewScheduleItemModal opens the per-item editor in create-from-transaction
// mode ([ADR-0031]): the shared modal shell loaded with an editor body
// prefilled from the named transaction — its direction and an initial
// cadence-date guess, read off the query the transaction editor built this URL
// from (mirroring ruleCreateURL's shape exactly) — a starting point, never a
// locked value: the name and cadence stay the user's to declare. The opener
// hx-gets this with hx-swap="none"; the Modal's OOB container mounts it. No
// schedule read backs this: everything it renders comes off the request.
func (h *HttpHandler) GetNewScheduleItemModal(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	views.ScheduleItemEditorModalFrag(views.ScheduleItemEditorProps{
		Item: schedule.Item{
			Name:       r.FormValue("merchant"),
			Direction:  scheduleDirectionFromParam(r.FormValue("direction")),
			Cadence:    schedule.CadenceMonthly,
			DayOfMonth: dayOfMonthFromParam(r.FormValue("day")),
			Active:     true,
		},
		TransactionID: strings.TrimSpace(r.FormValue("transaction_id")),
		ReturnTo:      validReturnHandle(r.FormValue("return_to")),
	}).Render(ctx, w)
}

// renderNewScheduleItemEditor re-renders the create-from-transaction editor in
// place after a rejected declaration, carrying whatever the user had typed
// (item), the transaction id, and the return handle, so a corrected resubmit
// still creates from the same transaction and still returns to the same
// origin. Anything that is not a validation error is a real server failure.
func (h *HttpHandler) renderNewScheduleItemEditor(ctx contextx.ContextX, w http.ResponseWriter, item schedule.Item, transactionID, returnTo string, err error) {
	ve, ok := schedule.IsValidationError(err)
	if !ok {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	views.ScheduleItemEditorContentFrag(views.ScheduleItemEditorProps{
		Item:          item,
		TransactionID: transactionID,
		ReturnTo:      returnTo,
		Error:         ve.Message,
	}).Render(ctx, w)
}

// scheduleDirectionFromParam reads the direction the create-from-transaction
// opener guessed, defaulting to out (the common case, a bill) for anything
// else — this only ever affects the disabled, informational direction field;
// CreateItemFromTransaction derives the real direction from the transaction
// itself and ignores whatever this form carries.
func scheduleDirectionFromParam(raw string) schedule.Direction {
	if raw == "in" {
		return schedule.DirectionIn
	}
	return schedule.DirectionOut
}

// dayOfMonthFromParam reads the initial cadence-date guess off the query,
// zero (dateless) for anything that will not parse as a day.
func dayOfMonthFromParam(raw string) int {
	day, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return day
}

// GetAttachScheduleItemModal opens the attach picker for an unmatched checking
// transaction ([ADR-0031]): the shared modal shell loaded with every active
// item sharing the transaction's direction that currently holds an
// outstanding occurrence, each paired with the occurrence nearest the
// transaction's own date (or its containing calendar month for a dateless
// item). direction and date come off the query the transaction editor built
// this URL from (mirroring ruleCreateURL's shape). The opener hx-gets this
// with hx-swap="none"; the Modal's OOB container mounts it.
func (h *HttpHandler) GetAttachScheduleItemModal(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	direction := scheduleDirectionFromParam(r.FormValue("direction"))
	date, err := time.Parse("2006-01-02", r.FormValue("date"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusBadRequest, Err: err})
		return
	}

	options, err := h.attachableItems(ctx, direction, date)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}

	views.ScheduleAttachModalFrag(views.ScheduleAttachProps{
		TransactionID: strings.TrimSpace(r.FormValue("transaction_id")),
		ReturnTo:      validReturnHandle(r.FormValue("return_to")),
		Items:         options,
	}).Render(ctx, w)
}

// attachableItems reads the active items the attach picker offers for one
// direction: only an item currently holding at least one outstanding
// (unsettled) occurrence in its matching window is offered, each paired with
// the outstanding occurrence nearest date — the same nearest-occurrence
// convention CreateItemFromTransaction's own bootstrap already uses
// ([ADR-0031]). Composed entirely from ActiveItems and MatchingWindow, the
// existing reads: no new schedule method, since these two already answer
// exactly which items qualify and which of their occurrences are unsettled.
func (h *HttpHandler) attachableItems(ctx contextx.ContextX, direction schedule.Direction, date time.Time) ([]views.ScheduleAttachOption, error) {
	windows, err := h.schedule.MatchingWindow(ctx)
	if err != nil {
		return nil, err
	}

	var options []views.ScheduleAttachOption
	for _, window := range windows {
		if window.Item.Direction != direction {
			continue
		}
		nearest, ok := nearestOutstandingOccurrence(window, date)
		if !ok {
			continue
		}
		options = append(options, views.ScheduleAttachOption{Item: window.Item, Occurrence: nearest})
	}
	return options, nil
}

// nearestOutstandingOccurrence picks the outstanding occurrence of one item's
// window nearest a given date. Every occurrence MatchingWindow projects for a
// dateless item stands in for the calendar month it falls in, so "nearest"
// already reads as "this transaction's own month" for one, with no special
// case needed here.
func nearestOutstandingOccurrence(window schedule.ItemOccurrences, date time.Time) (time.Time, bool) {
	var best time.Time
	found := false
	for _, occurrence := range window.Occurrences {
		if !occurrence.Outstanding() {
			continue
		}
		if !found || absDuration(occurrence.Occurrence, date) < absDuration(best, date) {
			best, found = occurrence.Occurrence, true
		}
	}
	return best, found
}

// absDuration is the unsigned gap between two instants, in whichever
// direction it runs.
func absDuration(a, b time.Time) time.Duration {
	if a.After(b) {
		return a.Sub(b)
	}
	return b.Sub(a)
}

// PostAttachScheduleItem records the manual match an attach picker's chosen
// option carries — the same MatchOccurrence operation the ordinary occurrence
// picker already uses ([ADR-0031]) — and dispatches through scheduleItemMutated:
// return to origin, or close the modal and refresh the list, the same shape
// every other modal save uses ([ADR-0016]). A race with another tab (the
// transaction already settles a different occurrence by the time this posts)
// re-renders the picker in place with a recoverable inline error instead.
func (h *HttpHandler) PostAttachScheduleItem(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	itemID := strings.TrimSpace(r.FormValue("item_id"))
	transactionID := strings.TrimSpace(r.FormValue("transaction_id"))
	returnTo := validReturnHandle(r.FormValue("return_to"))

	occurrence, err := time.Parse("2006-01-02", r.FormValue("occurrence"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusBadRequest, Err: err})
		return
	}

	if err := h.schedule.MatchOccurrence(ctx, itemID, occurrence, transactionID); err != nil {
		// The picker never offers an item without an outstanding occurrence, so
		// this is a race with another tab rather than a mistake the user can see.
		slog.ErrorContext(ctx, "failed to attach transaction to scheduled item", "error", err, "item", itemID)
		h.renderScheduleAttachFailure(ctx, w, itemID, occurrence, transactionID, returnTo, "That transaction already settles another occurrence. Choose a different item.")
		return
	}
	h.scheduleItemMutated(ctx, w, returnTo)
}

// renderScheduleAttachFailure re-reads the attach picker's options and
// re-renders the picker in place with the recoverable message, keeping the
// modal open. It reads the failed item's own direction (rather than trusting
// a client-supplied one) and reckons nearest-occurrence from the occurrence
// just attempted, which is close enough to reproduce the same picker the user
// was already looking at for their retry.
func (h *HttpHandler) renderScheduleAttachFailure(ctx contextx.ContextX, w http.ResponseWriter, itemID string, occurrence time.Time, transactionID, returnTo, message string) {
	items, err := h.schedule.ActiveItems(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	var direction schedule.Direction
	for _, item := range items {
		if item.ID == itemID {
			direction = item.Direction
			break
		}
	}
	options, err := h.attachableItems(ctx, direction, occurrence)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.ScheduleAttachContentFrag(views.ScheduleAttachProps{
		TransactionID: transactionID,
		ReturnTo:      returnTo,
		Error:         message,
		Items:         options,
	}).Render(ctx, w)
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
	// This decision keeps the modal open (renderScheduleItemEditor below), unlike
	// a save or delete, so it never reaches scheduleItemMutated — announce here
	// instead, so the list behind the modal still hears a match changed.
	if !h.announceScheduleChange(ctx, w) {
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
	// Same reasoning as PostOccurrenceMatch: this decision stays in the modal
	// rather than going through scheduleItemMutated, so it announces here.
	if !h.announceScheduleChange(ctx, w) {
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
	// Same reasoning as PostOccurrenceMatch: this decision stays in the modal
	// rather than going through scheduleItemMutated, so it announces here.
	if !h.announceScheduleChange(ctx, w) {
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
//
// Either way it announces schedule-changed first, so every other region that
// depends on the declared schedule — the transactions list's per-row match
// badge, and this same list if it is sitting behind a modal that a
// transaction's editor opened — re-fetches itself regardless of which of the
// two response shapes this call ends up writing ([ADR-0010]).
func (h *HttpHandler) scheduleItemMutated(ctx contextx.ContextX, w http.ResponseWriter, returnTo string) {
	if !h.announceScheduleChange(ctx, w) {
		return
	}
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

// announceScheduleChange sets the schedule-changed event header so every
// region that depends on the declared schedule re-fetches itself ([ADR-0010]):
// the transactions list's per-row match badge (schedule.MatchedItemNames) and
// this page's own compact list, from whichever handler renders the response.
//
// This is a distinct event from transactions' own transaction-changed, not a
// reuse of it. transaction-changed announces an actual write to a Transaction
// row (a re-categorization, e.g.) — a save, delete, or match decision here
// writes only schedule's own tables and never touches a Transaction row, so
// firing transaction-changed would misdescribe what happened to a future
// reader grepping for what it means. schedule-changed says exactly what did.
//
// It reports whether the header was set so the caller can stop before writing
// a body on the rare marshal failure (a 500).
func (h *HttpHandler) announceScheduleChange(ctx contextx.ContextX, w http.ResponseWriter) bool {
	if err := httpx.SetHXTrigger(w, "schedule-changed", nil); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return false
	}
	return true
}

// GetScheduleRegion re-fetches the schedule region on its own: the endpoint
// the region's own hidden listener hx-gets on schedule-changed ([ADR-0010]), so
// a currently-loaded /sweep page picks up a save, delete, or match change made
// through the per-item modal even when that modal returned to a transaction's
// editor instead of closing back into this page. A plain read, like the page
// load it mirrors — it triggers no run and stores nothing.
func (h *HttpHandler) GetScheduleRegion(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	h.renderSchedule(ctx, w, views.ScheduleProps{})
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
