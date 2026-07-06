package adapters

import (
	"errors"
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

// GetWrapPage renders a single month's wrap. The {ym} path value is a YYYY-MM
// month; a malformed slug is a 404 (no such month page), not a 500. The current
// month's wrap address redirects to the root Tracker — the current month's one
// canonical face — so a current-month drill's back-link returns there.
func (h *HttpHandler) GetWrapPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	year, month, err := parseMonth(r.PathValue("ym"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusNotFound, Err: err})
		return
	}

	if h.home.IsCurrentMonth(year, month) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	view, err := h.home.MonthWrap(ctx, year, month)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}

	// A transaction-changed self-refresh (the figure region's hidden listener) targets
	// just the region; a boosted navigation gets the full page.
	if isRegionSwap(r) {
		views.WrapFigureRegionFrag(view).Render(ctx, w)
		return
	}
	views.WrapPage(view).Render(ctx, w)
}

// GetSpendDrillPage renders the spend drill-down for one bucket of a month: the
// Spending transactions making up that bucket's net figure. A malformed {ym} is a
// 404; an everything-else bucket for a non-current month is also a 404 (there is
// no budget residual to drill outside the current month).
func (h *HttpHandler) GetSpendDrillPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	year, month, err := parseMonth(r.PathValue("ym"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusNotFound, Err: err})
		return
	}

	view, err := h.home.SpendDrill(ctx, year, month, r.PathValue("bucket"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, drillErrorProps(err))
		return
	}

	// A transaction-changed self-refresh (the hidden listener's hx-get) targets just
	// the region with an innerHTML swap, so return the region fragment; a boosted
	// navigation carries HX-Request too but swaps the whole body and must get the
	// full page, distinguished by the HX-Boosted header the listener never sends.
	if isRegionSwap(r) {
		views.SpendDrillRegionFrag(view).Render(ctx, w)
		return
	}
	views.SpendDrillPage(view).Render(ctx, w)
}

// isRegionSwap reports whether a GET is the drill region's own transaction-changed
// self-refresh (HX-Request, not a boosted navigation), so the handler returns just
// the region fragment rather than the full page.
func isRegionSwap(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true"
}

// drillErrorProps maps a drill service error to a response: the residual-bucket
// constraint is a 404 (no such page), anything else an unexpected 500.
func drillErrorProps(err error) httpx.HandleErrorResponseProps {
	if errors.Is(err, home.ErrResidualBucketUnavailable) {
		return httpx.HandleErrorResponseProps{Status: http.StatusNotFound, Err: err}
	}
	return httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err}
}

// parseMonth parses a YYYY-MM month slug into its calendar year and month.
func parseMonth(ym string) (int, time.Month, error) {
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		return 0, 0, err
	}
	return t.Year(), t.Month(), nil
}
