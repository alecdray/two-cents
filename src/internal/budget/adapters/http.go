package adapters

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/alecdray/two-cents/src/internal/budget"
	"github.com/alecdray/two-cents/src/internal/budget/adapters/views"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// HttpHandler serves the budget creator/editor page. It reads and writes the
// budget through the budget Service and reads the active Category list through
// the categorization Service (to render a limit row per Category); it never
// reaches storage or the bank directly.
type HttpHandler struct {
	budget         *budget.Service
	categorization *categorization.Service
}

// NewHttpHandler builds the handler over the budget Service and the
// categorization Service it consults for the active Category list.
func NewHttpHandler(budgetSvc *budget.Service, categorizationSvc *categorization.Service) *HttpHandler {
	return &HttpHandler{budget: budgetSvc, categorization: categorizationSvc}
}

// GetBudgetPage renders the budget editor: the income and savings targets, a
// spending-limit row per active Category, the computed residual, and the
// balance banner.
func (h *HttpHandler) GetBudgetPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	props, err := h.budgetProps(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.BudgetPage(props).Render(ctx, w)
}

// PostBudget saves the submitted budget and swaps the refreshed region back in,
// showing the saved values, the new residual, and the balance banner. A
// malformed amount renders inline without saving; an over-allocated plan still
// saves and surfaces the over-allocated banner.
func (h *HttpHandler) PostBudget(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	active, err := h.categorization.ListCategories(ctx, false)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}

	income, err := parseAmount(r.FormValue("income"))
	if err != nil {
		h.renderBudget(ctx, w, func(p *views.BudgetProps) { p.SaveError = "Enter a valid amount." })
		return
	}
	savings, err := parseAmount(r.FormValue("savings"))
	if err != nil {
		h.renderBudget(ctx, w, func(p *views.BudgetProps) { p.SaveError = "Enter a valid amount." })
		return
	}

	limits := make([]budget.CategoryLimit, 0, len(active))
	for _, c := range active {
		amount, err := parseAmount(r.FormValue("limit_" + c.ID))
		if err != nil {
			h.renderBudget(ctx, w, func(p *views.BudgetProps) { p.SaveError = "Enter a valid amount." })
			return
		}
		// Only a positive cap is a limit; a blank or zero row leaves the
		// Category unbudgeted, so the no-budget predicate stays meaningful.
		if amount > 0 {
			limits = append(limits, budget.CategoryLimit{CategoryID: c.ID, Limit: amount})
		}
	}

	if _, err := h.budget.SetBudget(ctx, income, savings, limits); err != nil {
		if ve, ok := budget.IsValidationError(err); ok {
			h.renderBudget(ctx, w, func(p *views.BudgetProps) { p.SaveError = ve.Message })
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}

	h.renderBudget(ctx, w, nil)
}

// renderBudget re-reads the saved budget and renders the budget content
// fragment, applying an optional decorator to surface an inline error.
func (h *HttpHandler) renderBudget(ctx contextx.ContextX, w http.ResponseWriter, decorate func(*views.BudgetProps)) {
	props, err := h.budgetProps(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	if decorate != nil {
		decorate(&props)
	}
	views.BudgetContentFrag(props).Render(ctx, w)
}

// budgetProps loads the stored budget and the active Categories, building one
// limit row per active Category (filling in any stored limit) and computing the
// residual and balance verdict over the active limits.
func (h *HttpHandler) budgetProps(ctx contextx.ContextX) (views.BudgetProps, error) {
	b, limits, err := h.budget.GetBudget(ctx)
	if err != nil {
		return views.BudgetProps{}, err
	}
	active, err := h.categorization.ListCategories(ctx, false)
	if err != nil {
		return views.BudgetProps{}, err
	}

	limitByID := make(map[string]float64, len(limits))
	for _, l := range limits {
		limitByID[l.CategoryID] = l.Limit
	}

	rows := make([]views.LimitRow, len(active))
	for i, c := range active {
		rows[i] = views.LimitRow{CategoryID: c.ID, Name: c.Name, Limit: limitByID[c.ID]}
	}

	residual, _ := budget.ComputeResidual(b.IncomeTarget, b.SavingsTarget, limits)

	return views.BudgetProps{
		Income:   b.IncomeTarget,
		Savings:  b.SavingsTarget,
		Limits:   rows,
		Residual: residual,
		Balance:  budget.BalanceCheck(b.IncomeTarget, b.SavingsTarget, limits),
	}, nil
}

// parseAmount reads a money field: a blank value is zero, anything else is parsed
// as a non-negative float. A non-numeric or negative value is an error the
// handler surfaces inline.
func parseAmount(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, strconv.ErrRange
	}
	return v, nil
}
