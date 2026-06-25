package adapters

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/categorization/adapters/views"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/core/templates"
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

// GetRulesPage renders the Rule management surface: the read-only Rules list with a
// New rule opener and per-row Edit / delete controls.
func (h *HttpHandler) GetRulesPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	props, err := h.rulesProps(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.RulesPage(props).Render(ctx, w)
}

// GetNewRuleModal opens the rule editor in create mode: the shared modal shell
// loaded with an empty editor body, optionally prefilled from the query (merchant /
// classification / category_id) and carrying a validated same-origin return handle
// ([ADR-0016]). The opener hx-gets this with hx-swap="none"; the Modal's OOB
// container mounts it.
func (h *HttpHandler) GetNewRuleModal(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	categories, err := h.service.ListCategories(ctx, false)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.RuleEditorModalFrag(views.RuleEditorProps{
		Substring:      r.FormValue("merchant"),
		Classification: classificationFromForm(r),
		CategoryID:     categoryIDFromForm(r),
		Categories:     categories,
		ReturnTo:       validReturnHandle(r.FormValue("return_to")),
	}).Render(ctx, w)
}

// GetEditRuleModal opens the rule editor in edit mode for one Rule, prefilled with
// its current substring / outcome / Category and carrying any validated return
// handle. It coexists with POST /rules/{id}/edit — the method-specific patterns make
// them distinct routes.
func (h *HttpHandler) GetEditRuleModal(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")

	rule, err := h.service.Rule(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	categories, err := h.service.ListCategories(ctx, false)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.RuleEditorModalFrag(views.RuleEditorProps{
		RuleID:         rule.ID,
		Substring:      rule.MerchantSubstring,
		Classification: rule.Classification,
		CategoryID:     rule.CategoryID,
		Categories:     categories,
		ReturnTo:       validReturnHandle(r.FormValue("return_to")),
	}).Render(ctx, w)
}

// PostRule creates a Rule from the editor modal and responds modal-style. A
// validation failure re-renders the editor body inline (the modal stays open); a
// success dispatches to ruleSaved (return to origin, or close + refresh the list).
func (h *HttpHandler) PostRule(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	returnTo := validReturnHandle(r.FormValue("return_to"))

	_, count, err := h.service.CreateRule(ctx, r.FormValue("merchant_substring"), classificationFromForm(r), categoryIDFromForm(r))
	if err != nil {
		if ve, ok := categorization.IsValidationError(err); ok {
			h.renderRuleEditor(ctx, w, views.RuleEditorProps{
				Substring:      r.FormValue("merchant_substring"),
				Classification: classificationFromForm(r),
				CategoryID:     categoryIDFromForm(r),
				ReturnTo:       returnTo,
				Error:          ve.Message,
			})
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.ruleSaved(ctx, w, returnTo, count)
}

// PostEditRule edits a Rule from the editor modal and responds modal-style — the
// same three cases as PostRule, re-rendering the edit form (RuleID set) on a
// validation failure so a corrected resubmit still targets the right Rule.
func (h *HttpHandler) PostEditRule(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")
	returnTo := validReturnHandle(r.FormValue("return_to"))

	_, count, err := h.service.EditRule(ctx, id, r.FormValue("merchant_substring"), classificationFromForm(r), categoryIDFromForm(r))
	if err != nil {
		if ve, ok := categorization.IsValidationError(err); ok {
			h.renderRuleEditor(ctx, w, views.RuleEditorProps{
				RuleID:         id,
				Substring:      r.FormValue("merchant_substring"),
				Classification: classificationFromForm(r),
				CategoryID:     categoryIDFromForm(r),
				ReturnTo:       returnTo,
				Error:          ve.Message,
			})
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.ruleSaved(ctx, w, returnTo, count)
}

// PostDeleteRule deletes a Rule, re-categorizes the transactions it had matched,
// and swaps the region back in with the count. It also announces transaction-changed
// so the transaction views a re-categorization touched self-refresh ([ADR-0010]).
func (h *HttpHandler) PostDeleteRule(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	count, err := h.service.DeleteRule(ctx, r.PathValue("id"))
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	if err := httpx.SetHXTrigger(w, "transaction-changed", nil); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderRules(ctx, w, func(p *views.RulesProps) { p.Feedback = recategorizedMessage(count) })
}

// ruleSaved writes the success response for a modal save, in two cases. It always
// announces transaction-changed (a Rule change re-categorizes, so transaction views
// refresh, [ADR-0010]). With a valid return handle it returns the self-firing return
// loader, which re-mounts the origin's modal on load. With no handle (opened from
// /rules) it closes the modal and refreshes the list with the re-categorized count in
// one response: it retargets the swap to the rules region (innerHTML), emits an
// OOB-empty modal container to clear the modal, then the refreshed rules content.
func (h *HttpHandler) ruleSaved(ctx contextx.ContextX, w http.ResponseWriter, returnTo string, count int) {
	if err := httpx.SetHXTrigger(w, "transaction-changed", nil); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	if returnTo != "" {
		views.RuleEditorReturnLoader(returnTo).Render(ctx, w)
		return
	}

	props, err := h.rulesProps(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	props.Feedback = recategorizedMessage(count)

	w.Header().Set("HX-Retarget", "#"+views.RulesRegionID())
	w.Header().Set("HX-Reswap", "innerHTML")
	templates.ModalContainer(true).Render(ctx, w)
	views.RulesContentFrag(props).Render(ctx, w)
}

// renderRuleEditor loads the active Categories and re-renders the editor body region
// in place (outerHTML) with the inline validation error carried in props, so the
// modal stays open and a corrected resubmit still carries any return handle.
func (h *HttpHandler) renderRuleEditor(ctx contextx.ContextX, w http.ResponseWriter, props views.RuleEditorProps) {
	categories, err := h.service.ListCategories(ctx, false)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	props.Categories = categories
	views.RuleEditorContentFrag(props).Render(ctx, w)
}

// validReturnHandle echoes a return handle only when it is a same-origin relative
// path — a single leading slash (rejecting protocol-relative "//host") and no scheme
// ("://"). Anything else (absent, absolute, scheme-bearing) yields the empty string,
// so a handle carrying a host or scheme is never followed. This is a hard security
// property: the editor round-trips the handle without inspecting where it points, so
// the validation is the only guard.
func validReturnHandle(raw string) string {
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "://") {
		return ""
	}
	return raw
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

	return views.RulesProps{Rules: rows}, nil
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
