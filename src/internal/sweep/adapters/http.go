package adapters

import (
	"log/slog"
	"net/http"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/sweep"
	"github.com/alecdray/two-cents/src/internal/sweep/adapters/views"
)

// HttpHandler serves the sweep snapshot history. Reads render a stored snapshot
// and never trigger a compute; the one write is the explicit Run now action.
type HttpHandler struct {
	sweep *sweep.Service
}

// NewHttpHandler builds the handler over the sweep Service.
func NewHttpHandler(sweepSvc *sweep.Service) *HttpHandler {
	return &HttpHandler{sweep: sweepSvc}
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
		h.renderRegion(ctx, w, "", "We couldn't run the sweep. Please try again.")
		return
	}

	w.Header().Set("HX-Push-Url", "/sweep/"+rec.ID)
	h.renderRegion(ctx, w, rec.ID, "")
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

	views.SweepPage(snap, found).Render(ctx, w)
}
