package adapters

import (
	"errors"
	"net/http"
	"time"

	"github.com/alecdray/two-cents/src/internal/categorization"
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
	views.SpendDrillPage(view).Render(ctx, w)
}

// PostDrillCategorize records a re-categorization of one drilled row and swaps the
// whole drill region back in, so a row the edit moves out of the bucket drops from
// the list and the net total updates. A coupling validation error renders inline
// in the same region beside the unchanged list.
func (h *HttpHandler) PostDrillCategorize(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	year, month, err := parseMonth(r.PathValue("ym"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusNotFound, Err: err})
		return
	}

	view, validationMsg, err := h.home.ReCategorizeInDrill(ctx, year, month, r.PathValue("bucket"), r.PathValue("id"), classificationFromForm(r), categoryIDFromForm(r))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, drillErrorProps(err))
		return
	}
	views.SpendDrillRegionFrag(view, validationMsg).Render(ctx, w)
}

// drillErrorProps maps a drill service error to a response: the residual-bucket
// constraint is a 404 (no such page), anything else an unexpected 500.
func drillErrorProps(err error) httpx.HandleErrorResponseProps {
	if errors.Is(err, home.ErrResidualBucketUnavailable) {
		return httpx.HandleErrorResponseProps{Status: http.StatusNotFound, Err: err}
	}
	return httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err}
}

// classificationFromForm reads the outcome the drill's re-categorize picker posted.
func classificationFromForm(r *http.Request) categorization.Classification {
	return categorization.Classification(r.FormValue("classification"))
}

// categoryIDFromForm reads the chosen Category id, returning nil when none was
// selected (the empty option) so an income/transfer/needs-review choice carries
// no Category.
func categoryIDFromForm(r *http.Request) *string {
	id := r.FormValue("category_id")
	if id == "" {
		return nil
	}
	return &id
}

// parseMonth parses a YYYY-MM month slug into its calendar year and month.
func parseMonth(ym string) (int, time.Month, error) {
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		return 0, 0, err
	}
	return t.Year(), t.Month(), nil
}
