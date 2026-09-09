package adapters

import (
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

// PostRun computes a fresh recommendation, appends it, and lands the user on the
// snapshot it produced. This is the only path on the page that computes.
func (h *HttpHandler) PostRun(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	rec, err := h.sweep.Run(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	http.Redirect(w, r, "/sweep/"+rec.ID, http.StatusSeeOther)
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
