package adapters

import (
	"net/http"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/sweep"
	"github.com/alecdray/two-cents/src/internal/sweep/adapters/views"
)

// HttpHandler serves the sweep recommendation page. It reads only the
// persisted latest result through the sweep Service — it never triggers a
// compute on request.
type HttpHandler struct {
	sweep *sweep.Service
}

// NewHttpHandler builds the handler over the sweep Service.
func NewHttpHandler(sweepSvc *sweep.Service) *HttpHandler {
	return &HttpHandler{sweep: sweepSvc}
}

// GetPage renders the latest sweep recommendation at /sweep. When no
// recommendation has been stored yet, the page shows a first-run empty state.
// When a recommendation is stored it renders either the numeric breakdown
// (leading with the action line) or the needs-attention reasons, depending on
// the stored kind.
func (h *HttpHandler) GetPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	rec, found, err := h.sweep.LoadLatest(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.SweepPage(rec, found).Render(ctx, w)
}
