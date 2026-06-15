package adapters

import (
	"net/http"
	"time"

	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/home"
	"github.com/alecdray/two-cents/src/internal/home/adapters/views"
)

// HttpHandler serves the dashboard pages. It reads everything through the home
// composing Service, which fans out to the domain services and the pure
// projections; the handler maps service errors to responses and renders.
type HttpHandler struct {
	home *home.Service
}

// NewHttpHandler builds the handler over the home composing Service.
func NewHttpHandler(homeSvc *home.Service) *HttpHandler {
	return &HttpHandler{home: homeSvc}
}

// GetTrackerPage renders the current-month Tracker dashboard at the application
// root: budgeted-Category standings, the everything-else line, totals and pace,
// income and savings progress — or, with no budget set, the actuals plus a
// prompt to create one.
func (h *HttpHandler) GetTrackerPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	view, err := h.home.CurrentMonthTracker(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.TrackerPage(view).Render(ctx, w)
}

// GetWrapsPage renders the wraps list: every month from the earliest
// transaction's month through the current month, most-recent first, each linking
// to its wrap.
func (h *HttpHandler) GetWrapsPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	summaries, err := h.home.WrapList(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.WrapsPage(summaries).Render(ctx, w)
}

// GetWrapPage renders a single month's wrap. The {ym} path value is a YYYY-MM
// month; a malformed slug is a 404 (no such month page), not a 500.
func (h *HttpHandler) GetWrapPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	year, month, err := parseMonth(r.PathValue("ym"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusNotFound, Err: err})
		return
	}

	view, err := h.home.MonthWrap(ctx, year, month)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.WrapPage(view).Render(ctx, w)
}

// parseMonth parses a YYYY-MM month slug into its calendar year and month.
func parseMonth(ym string) (int, time.Month, error) {
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		return 0, 0, err
	}
	return t.Year(), t.Month(), nil
}
