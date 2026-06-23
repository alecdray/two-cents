package adapters

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/categorization"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/transactions"
	"github.com/alecdray/two-cents/src/internal/transactions/adapters/views"
)

// recentTransactionsLimit caps the activity list at the most recent N
// transactions across all accounts; the read returns them already ordered
// newest-first (date desc, then stable provider id desc).
const recentTransactionsLimit = 100

// HttpHandler serves the transactions module's pages. It reads recent
// transactions through the transactions Service, asks the accounts Service
// whether any bank is connected (so the page can pick the right empty state),
// and reads the active Category taxonomy through the categorization Service to
// populate each row's re-categorize picker; it never reaches the bank provider
// directly.
type HttpHandler struct {
	transactionsService   *transactions.Service
	accountsService       *accounts.Service
	categorizationService *categorization.Service
}

// NewHttpHandler builds the handler over the transactions Service it reads the
// activity from, the accounts Service it asks whether a bank is connected, and
// the categorization Service whose active Categories populate the re-categorize
// picker.
func NewHttpHandler(transactionsService *transactions.Service, accountsService *accounts.Service, categorizationService *categorization.Service) *HttpHandler {
	return &HttpHandler{
		transactionsService:   transactionsService,
		accountsService:       accountsService,
		categorizationService: categorizationService,
	}
}

// GetTransactionsPage renders the recent-activity surface: the month-grouped list
// of transactions (filtered by the request's search + view), or the appropriate
// empty state. It serves both the full page (a normal navigation) and the bare
// content fragment (an htmx search/toggle swap that targets the region), keyed off
// the HX-Request header.
func (h *HttpHandler) GetTransactionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	view := listViewFromRequest(r)

	page, err := h.activity(ctx, view)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	if isRegionSwap(r) {
		views.TransactionsContentFrag(page.HasConnections, page.Rows, "", view.controls()).Render(ctx, w)
		return
	}
	views.TransactionsPage(page.HasConnections, page.Rows, view.controls()).Render(ctx, w)
}

// PostSync runs an on-demand sync and swaps the refreshed activity region back in,
// preserving the request's current search + view. An unexpected sync failure
// renders the same region with a recoverable inline error beside the sync control —
// no redirect, no full-page replacement — leaving any already-loaded transactions
// in view.
func (h *HttpHandler) PostSync(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	view := listViewFromRequest(r)

	if err := h.transactionsService.SyncTransactions(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to sync transactions", "error", err)
		page, aerr := h.activity(ctx, view)
		if aerr != nil {
			httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
				Status: http.StatusInternalServerError,
				Err:    aerr,
			})
			return
		}
		views.TransactionsContentFrag(page.HasConnections, page.Rows, "We couldn't sync your transactions. Please try again.", view.controls()).Render(ctx, w)
		return
	}

	page, err := h.activity(ctx, view)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.TransactionsContentFrag(page.HasConnections, page.Rows, "", view.controls()).Render(ctx, w)
}

// GetEditModal opens the transaction-editing modal for one row: it reads the row,
// the active Categories, and the connected-account facets and returns the shared
// modal shell loaded with the editor body. Every surface that lists transactions
// opens this same view-agnostic editor; the opening control hx-gets this with
// hx-swap="none" and the Modal's OOB container mounts it ([ADR-0011]).
func (h *HttpHandler) GetEditModal(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")

	row, categories, facets, err := h.editorData(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.TransactionEditModalFrag(row, categories, facets, editorLocation(ctx)).Render(ctx, w)
}

// PostEdit saves one transaction from the shared modal. It issues the existing two
// writes in turn — the re-categorization, then (for an outflow row chosen as a
// Transfer) the transfer-destination mark — each keeping its own validation rather
// than a merged write ([ADR-0011]). On success it announces transaction-changed (so
// each list region self-refreshes, [ADR-0010]) and swaps the editor body back in so
// the open modal reflects the new state. A coupling violation (a Spending choice
// with no Category) or an invalid transfer mark renders inline in the editor without
// announcing a change; an unexpected failure is a 500.
func (h *HttpHandler) PostEdit(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")
	classification := classificationFromForm(r)

	if err := h.transactionsService.ReCategorize(ctx, id, classification, categoryIDFromForm(r)); err != nil {
		if ve, ok := transactions.IsValidationError(err); ok {
			h.renderEditor(ctx, w, id, ve.Message, "")
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}

	// Only an outflow Transfer carries a destination. The transfer fields post even
	// when Alpine hides them, so gate the mark on the chosen outcome and the row's
	// direction rather than the fields' presence; a non-transfer outcome already
	// cleared the transfer facet in the re-categorize write.
	row, err := h.transactionsService.RecentTransaction(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	if classification == categorization.Transfer && row.Amount.Amount > 0 {
		if err := h.transactionsService.MarkTransferDestination(ctx, id, transferDestinationIDFromForm(r), transferSubtypeFromForm(r)); err != nil {
			if ve, ok := transactions.IsValidationError(err); ok {
				h.renderEditor(ctx, w, id, "", ve.Message)
				return
			}
			httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
			return
		}
	}

	if !h.announceChange(ctx, w, id) {
		return
	}
	h.renderEditor(ctx, w, id, "", "")
}

// announceChange sets the transaction-changed event header so every region that
// lists or aggregates transactions re-fetches itself ([ADR-0010]); the editor stays
// context-free and never renders a caller. It reports whether the header was set so
// the caller can stop before writing a body on the rare marshal failure (a 500).
func (h *HttpHandler) announceChange(ctx contextx.ContextX, w http.ResponseWriter, id string) bool {
	if err := httpx.SetHXTrigger(w, "transaction-changed", nil); err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return false
	}
	return true
}

// renderEditor re-reads the transaction with the active Categories and connected
// facets and swaps the editor body region back in place (outerHTML), optionally with
// an inline error beside the re-categorize and/or transfer-destination control. It is
// view-agnostic: the open modal is the same on every surface, so it carries no view
// state.
func (h *HttpHandler) renderEditor(ctx contextx.ContextX, w http.ResponseWriter, id, categorizeError, transferError string) {
	row, categories, facets, err := h.editorData(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.TransactionEditContentFrag(row, categories, facets, editorLocation(ctx), categorizeError, transferError).Render(ctx, w)
}

// editorLocation returns the configured app timezone ([ADR-0004]) the editor
// renders transaction timestamps in, falling back to UTC if the app config is
// somehow absent from the context.
func editorLocation(ctx contextx.ContextX) *time.Location {
	a, err := ctx.App()
	if err != nil {
		return time.UTC
	}
	if loc := a.Config().AppTimezone; loc != nil {
		return loc
	}
	return time.UTC
}

// editorData reads the inputs the editor renders: the transaction's current state,
// the active Category taxonomy its picker offers, and the connected-account facets
// its transfer-destination picker offers.
func (h *HttpHandler) editorData(ctx contextx.ContextX, id string) (transactions.RecentTransaction, []categorization.Category, []accounts.AccountFacet, error) {
	row, err := h.transactionsService.RecentTransaction(ctx, id)
	if err != nil {
		return transactions.RecentTransaction{}, nil, nil, err
	}
	categories, err := h.categorizationService.ListCategories(ctx, false)
	if err != nil {
		return transactions.RecentTransaction{}, nil, nil, err
	}
	facets, err := h.accountsService.ConnectedAccountFacets(ctx)
	if err != nil {
		return transactions.RecentTransaction{}, nil, nil, err
	}
	return row, categories, facets, nil
}

// activityPage carries the read model the page and its fragments render.
type activityPage struct {
	HasConnections bool
	Rows           []transactions.RecentTransaction
}

// activity reads the page's data for the given view: whether any bank is connected
// (the accounts overview's empty-vs-populated signal) and the transactions to show.
// With an active filter (search and/or needs-attention) it reads the full history;
// otherwise the recent-capped default list. The Category taxonomy and account facets
// the editor offers are read on demand by the edit endpoint, not by the list. The
// read never calls the provider.
func (h *HttpHandler) activity(ctx contextx.ContextX, view listView) (activityPage, error) {
	dashboard, err := h.accountsService.Dashboard(ctx)
	if err != nil {
		return activityPage{}, err
	}

	var rows []transactions.RecentTransaction
	if filter := view.filter(); filter.Active() {
		rows, err = h.transactionsService.FilteredTransactions(ctx, filter)
	} else {
		rows, err = h.transactionsService.RecentTransactions(ctx, recentTransactionsLimit)
	}
	if err != nil {
		return activityPage{}, err
	}

	return activityPage{HasConnections: dashboard.HasAccounts(), Rows: rows}, nil
}

// listView is the parsed /transactions view state from a request: the merchant
// search text and whether the needs-attention worklist is selected. It maps onto
// both the domain Filter (what to query) and the view's ListControls (how to render).
type listView struct {
	Query          string
	NeedsAttention bool
}

// listViewFromRequest reads the view state from the request — http.Request.FormValue
// covers both the URL query (a GET search/toggle) and the posted form (a resolve or
// sync). `q` is the merchant search; `view` selects the needs-attention worklist.
func listViewFromRequest(r *http.Request) listView {
	return listView{
		Query:          strings.TrimSpace(r.FormValue("q")),
		NeedsAttention: r.FormValue("view") == views.ViewNeedsAttentionParam,
	}
}

// filter maps the view onto the domain read filter.
func (v listView) filter() transactions.Filter {
	return transactions.Filter{Merchant: v.Query, NeedsAttention: v.NeedsAttention}
}

// controls maps the view onto the rendering controls the templ reads.
func (v listView) controls() views.ListControls {
	return views.ListControls{Query: v.Query, NeedsAttentionView: v.NeedsAttention}
}

// isRegionSwap reports whether a GET /transactions is one of the page's own
// targeted region swaps (the search box or the view toggle) — so it returns just
// the content fragment. A boosted navbar navigation also carries HX-Request, but it
// swaps the whole <body> and must get the full page; it is distinguished by the
// HX-Boosted header the explicit hx-get search/toggle controls never send.
func isRegionSwap(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true"
}

// classificationFromForm reads the outcome the picker posted.
func classificationFromForm(r *http.Request) categorization.Classification {
	return categorization.Classification(r.FormValue("classification"))
}

// categoryIDFromForm reads the chosen Category id, returning nil when none was
// selected (the empty option), so income/transfer/needs-review choices carry no
// Category.
func categoryIDFromForm(r *http.Request) *string {
	id := r.FormValue("category_id")
	if id == "" {
		return nil
	}
	return &id
}

// transferDestinationIDFromForm reads the chosen destination account id from the
// transfer picker, returning nil when none was selected (the empty option) so the
// user can attribute a subtype without recording a connected destination.
func transferDestinationIDFromForm(r *http.Request) *string {
	id := r.FormValue("transfer_destination_account_id")
	if id == "" {
		return nil
	}
	return &id
}

// transferSubtypeFromForm reads the chosen transfer subtype the picker posted (a
// savings contribution or a plain transfer).
func transferSubtypeFromForm(r *http.Request) categorization.TransferSubtype {
	return categorization.TransferSubtype(r.FormValue("transfer_subtype"))
}
