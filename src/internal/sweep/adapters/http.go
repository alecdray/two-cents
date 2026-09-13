package adapters

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
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

// PutScheduleItem overwrites one declared item. The active toggle rides the same
// form, so turning an item off is an ordinary edit rather than its own verb.
func (h *HttpHandler) PutScheduleItem(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")

	item, err := parseScheduleItem(r)
	if err == nil {
		item.ID = id
		err = h.schedule.Update(ctx, item)
	}
	if err != nil {
		h.renderScheduleFailure(ctx, w, id, err)
		return
	}
	h.renderSchedule(ctx, w, views.ScheduleProps{})
}

// DeleteScheduleItem removes one declared item from the schedule.
func (h *HttpHandler) DeleteScheduleItem(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	if err := h.schedule.Delete(ctx, r.PathValue("id")); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	h.renderSchedule(ctx, w, views.ScheduleProps{})
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
	items, err := h.schedule.List(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}
	props.Items = items
	views.ScheduleFrag(props).Render(ctx, w)
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

	items, err := h.schedule.List(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.SweepPage(snap, found, views.ScheduleProps{Items: items}).Render(ctx, w)
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

// parseAmount reads a dollar figure from a form field, treating blank as zero so
// an empty amount reaches the schedule's own "greater than zero" rule rather
// than failing here as a malformed number.
func parseAmount(raw string) (float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	return strconv.ParseFloat(trimmed, 64)
}
