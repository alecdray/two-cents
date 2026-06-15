package adapters

import (
	"fmt"
	"net/http"

	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/categorization/adapters/views"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// HttpHandler serves the categorization module's two management pages — the
// Category taxonomy and the user Rules. It reads and mutates through the
// categorization Service; it never reaches storage or the bank directly.
type HttpHandler struct {
	service *categorization.Service
}

// NewHttpHandler builds the handler over the categorization Service.
func NewHttpHandler(service *categorization.Service) *HttpHandler {
	return &HttpHandler{service: service}
}

// --- Categories ---

// GetCategoriesPage renders the Category management surface: active and archived
// Categories listed separately, with create / rename / archive / unarchive.
func (h *HttpHandler) GetCategoriesPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	props, err := h.categoriesProps(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.CategoriesPage(props).Render(ctx, w)
}

// PostCategory creates a custom Category and swaps the refreshed region back in.
// A validation failure (blank/duplicate name) renders inline in the create form.
func (h *HttpHandler) PostCategory(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	if _, err := h.service.CreateCategory(ctx, r.FormValue("name")); err != nil {
		if ve, ok := categorization.IsValidationError(err); ok {
			h.renderCategories(ctx, w, func(p *views.CategoriesProps) { p.CreateError = ve.Message })
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderCategories(ctx, w, nil)
}

// PostRenameCategory renames a Category (id stable) and swaps the region back in;
// a validation failure renders inline beside that row.
func (h *HttpHandler) PostRenameCategory(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")

	if _, err := h.service.RenameCategory(ctx, id, r.FormValue("name")); err != nil {
		if ve, ok := categorization.IsValidationError(err); ok {
			h.renderCategories(ctx, w, func(p *views.CategoriesProps) {
				p.RenameError = views.NewFormFailure(id, ve.Message)
			})
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderCategories(ctx, w, nil)
}

// PostArchiveCategory archives a Category and swaps the region back in.
func (h *HttpHandler) PostArchiveCategory(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	if _, err := h.service.ArchiveCategory(ctx, r.PathValue("id")); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderCategories(ctx, w, nil)
}

// PostUnarchiveCategory restores an archived Category and swaps the region back in.
func (h *HttpHandler) PostUnarchiveCategory(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	if _, err := h.service.UnarchiveCategory(ctx, r.PathValue("id")); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderCategories(ctx, w, nil)
}

// renderCategories re-reads the taxonomy and renders the categories content
// fragment, applying an optional decorator to surface an inline error.
func (h *HttpHandler) renderCategories(ctx contextx.ContextX, w http.ResponseWriter, decorate func(*views.CategoriesProps)) {
	props, err := h.categoriesProps(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	if decorate != nil {
		decorate(&props)
	}
	views.CategoriesContentFrag(props).Render(ctx, w)
}

// categoriesProps loads the taxonomy and splits it into active and archived.
func (h *HttpHandler) categoriesProps(ctx contextx.ContextX) (views.CategoriesProps, error) {
	all, err := h.service.ListCategories(ctx, true)
	if err != nil {
		return views.CategoriesProps{}, err
	}
	var props views.CategoriesProps
	for _, c := range all {
		if c.Archived {
			props.Archived = append(props.Archived, c)
		} else {
			props.Active = append(props.Active, c)
		}
	}
	return props, nil
}

// --- Rules ---

// GetRulesPage renders the Rule management surface: the Rules list with create,
// edit, and delete, each surfacing the re-categorized count.
func (h *HttpHandler) GetRulesPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	props, err := h.rulesProps(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.RulesPage(props).Render(ctx, w)
}

// PostRule creates a Rule, re-categorizes the matching transactions, and swaps
// the region back in with the re-categorized count. A validation failure renders
// inline in the create form.
func (h *HttpHandler) PostRule(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	_, count, err := h.service.CreateRule(ctx, r.FormValue("merchant_substring"), classificationFromForm(r), categoryIDFromForm(r))
	if err != nil {
		if ve, ok := categorization.IsValidationError(err); ok {
			h.renderRules(ctx, w, func(p *views.RulesProps) { p.CreateError = ve.Message })
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderRules(ctx, w, func(p *views.RulesProps) { p.Feedback = recategorizedMessage(count) })
}

// PostEditRule edits a Rule, re-categorizes the affected transactions, and swaps
// the region back in with the count. A validation failure renders inline beside
// that row.
func (h *HttpHandler) PostEditRule(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")

	_, count, err := h.service.EditRule(ctx, id, r.FormValue("merchant_substring"), classificationFromForm(r), categoryIDFromForm(r))
	if err != nil {
		if ve, ok := categorization.IsValidationError(err); ok {
			h.renderRules(ctx, w, func(p *views.RulesProps) { p.EditError = views.NewFormFailure(id, ve.Message) })
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderRules(ctx, w, func(p *views.RulesProps) { p.Feedback = recategorizedMessage(count) })
}

// PostDeleteRule deletes a Rule, re-categorizes the transactions it had matched,
// and swaps the region back in with the count.
func (h *HttpHandler) PostDeleteRule(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	count, err := h.service.DeleteRule(ctx, r.PathValue("id"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderRules(ctx, w, func(p *views.RulesProps) { p.Feedback = recategorizedMessage(count) })
}

// renderRules re-reads the Rules and active Categories and renders the rules
// content fragment, applying an optional decorator for inline error / feedback.
func (h *HttpHandler) renderRules(ctx contextx.ContextX, w http.ResponseWriter, decorate func(*views.RulesProps)) {
	props, err := h.rulesProps(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	if decorate != nil {
		decorate(&props)
	}
	views.RulesContentFrag(props).Render(ctx, w)
}

// rulesProps loads the Rules and the active spending Categories, building the
// view rows (resolving each spending Rule's Category display name).
func (h *HttpHandler) rulesProps(ctx contextx.ContextX) (views.RulesProps, error) {
	rules, err := h.service.ListRules(ctx)
	if err != nil {
		return views.RulesProps{}, err
	}
	categories, err := h.service.ListCategories(ctx, false)
	if err != nil {
		return views.RulesProps{}, err
	}

	nameByID := make(map[string]string, len(categories))
	for _, c := range categories {
		nameByID[c.ID] = c.Name
	}

	rows := make([]views.RuleRow, len(rules))
	for i, rule := range rules {
		row := views.RuleRow{Rule: rule}
		if rule.CategoryID != nil {
			row.CategoryName = nameByID[*rule.CategoryID]
		}
		rows[i] = row
	}

	return views.RulesProps{Rules: rows, Categories: categories}, nil
}

// classificationFromForm reads the outcome the form posted.
func classificationFromForm(r *http.Request) categorization.Classification {
	return categorization.Classification(r.FormValue("classification"))
}

// categoryIDFromForm reads the chosen Category id, returning nil when none was
// selected (the empty option), so income/transfer rules carry no Category.
func categoryIDFromForm(r *http.Request) *string {
	id := r.FormValue("category_id")
	if id == "" {
		return nil
	}
	return &id
}

// recategorizedMessage renders the re-categorized count as the feedback line.
func recategorizedMessage(count int) string {
	if count == 1 {
		return "1 transaction re-categorized."
	}
	return fmt.Sprintf("%d transactions re-categorized.", count)
}
