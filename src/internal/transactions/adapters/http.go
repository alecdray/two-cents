package adapters

import (
	"log/slog"
	"net/http"

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

// GetTransactionsPage renders the recent-activity surface: the flat,
// newest-first list of transactions across all connected accounts, or the
// appropriate empty state when there are no connections or nothing synced yet.
func (h *HttpHandler) GetTransactionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	page, err := h.activity(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.TransactionsPage(page.HasConnections, page.Rows, page.Categories, page.AccountFacets).Render(ctx, w)
}

// PostSync runs an on-demand sync and swaps the refreshed activity region back
// in. An unexpected sync failure renders the same region with a recoverable
// inline error beside the sync control — no redirect, no full-page replacement —
// leaving any already-loaded transactions in view.
func (h *HttpHandler) PostSync(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	if err := h.transactionsService.SyncTransactions(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to sync transactions", "error", err)
		page, aerr := h.activity(ctx)
		if aerr != nil {
			httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
				Status: http.StatusInternalServerError,
				Err:    aerr,
			})
			return
		}
		views.TransactionsContentFrag(page.HasConnections, page.Rows, page.Categories, page.AccountFacets, "We couldn't sync your transactions. Please try again.").Render(ctx, w)
		return
	}

	page, err := h.activity(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.TransactionsContentFrag(page.HasConnections, page.Rows, page.Categories, page.AccountFacets, "").Render(ctx, w)
}

// PostCategorize records a manual re-categorization of one transaction and swaps
// that row's fragment back in. A coupling violation (a Spending choice with no
// Category) renders inline beside the row's picker without navigating; an
// unexpected failure is a 500.
func (h *HttpHandler) PostCategorize(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")

	err := h.transactionsService.ReCategorize(ctx, id, classificationFromForm(r), categoryIDFromForm(r))
	if err != nil {
		if ve, ok := transactions.IsValidationError(err); ok {
			h.renderRow(ctx, w, id, ve.Message, "")
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderRow(ctx, w, id, "", "")
}

// PostTransferDestination records a manual transfer-destination mark/correction
// for one outflow Transfer leg and swaps that row's fragment back in. A
// validation error (the row is not an outflow transfer, or the subtype is
// invalid) renders inline beside the row's transfer picker without navigating;
// an unexpected failure is a 500. It writes the transfer facet only — never the
// row's categorization.
func (h *HttpHandler) PostTransferDestination(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())
	id := r.PathValue("id")

	err := h.transactionsService.MarkTransferDestination(ctx, id, transferDestinationIDFromForm(r), transferSubtypeFromForm(r))
	if err != nil {
		if ve, ok := transactions.IsValidationError(err); ok {
			h.renderRow(ctx, w, id, "", ve.Message)
			return
		}
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	h.renderRow(ctx, w, id, "", "")
}

// renderRow re-reads one transaction with the active Categories and the
// connected-account facets and renders the single row fragment, with an optional
// inline error beside the re-categorize picker and/or the transfer-destination
// picker.
func (h *HttpHandler) renderRow(ctx contextx.ContextX, w http.ResponseWriter, id, categorizeError, transferError string) {
	row, err := h.transactionsService.RecentTransaction(ctx, id)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	categories, err := h.categorizationService.ListCategories(ctx, false)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	facets, err := h.accountsService.ConnectedAccountFacets(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{Status: http.StatusInternalServerError, Err: err})
		return
	}
	views.TransactionRowFrag(row, categories, facets, categorizeError, transferError).Render(ctx, w)
}

// activityPage carries the read model the page and its fragments render.
type activityPage struct {
	HasConnections bool
	Rows           []transactions.RecentTransaction
	Categories     []categorization.Category
	AccountFacets  []accounts.AccountFacet
}

// activity reads the page's data: whether any bank is connected (the accounts
// overview's empty-vs-populated signal), the most recent transactions, and the
// active Category taxonomy the re-categorize picker offers. The read never calls
// the provider.
func (h *HttpHandler) activity(ctx contextx.ContextX) (activityPage, error) {
	dashboard, err := h.accountsService.Dashboard(ctx)
	if err != nil {
		return activityPage{}, err
	}

	rows, err := h.transactionsService.RecentTransactions(ctx, recentTransactionsLimit)
	if err != nil {
		return activityPage{}, err
	}

	categories, err := h.categorizationService.ListCategories(ctx, false)
	if err != nil {
		return activityPage{}, err
	}

	facets, err := h.accountsService.ConnectedAccountFacets(ctx)
	if err != nil {
		return activityPage{}, err
	}

	return activityPage{HasConnections: dashboard.HasAccounts(), Rows: rows, Categories: categories, AccountFacets: facets}, nil
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
