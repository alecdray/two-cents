package adapters

import (
	"log/slog"
	"net/http"

	"github.com/alecdray/two-cents/src/internal/accounts"
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
// transactions through the transactions Service and asks the accounts Service
// whether any bank is connected (so the page can pick the right empty state); it
// never reaches the bank provider directly.
type HttpHandler struct {
	transactionsService *transactions.Service
	accountsService     *accounts.Service
}

// NewHttpHandler builds the handler over the transactions Service it reads the
// activity from and the accounts Service it asks whether a bank is connected.
func NewHttpHandler(transactionsService *transactions.Service, accountsService *accounts.Service) *HttpHandler {
	return &HttpHandler{transactionsService: transactionsService, accountsService: accountsService}
}

// GetTransactionsPage renders the recent-activity surface: the flat,
// newest-first list of transactions across all connected accounts, or the
// appropriate empty state when there are no connections or nothing synced yet.
func (h *HttpHandler) GetTransactionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	hasConnections, rows, err := h.activity(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.TransactionsPage(hasConnections, rows).Render(ctx, w)
}

// PostSync runs an on-demand sync and swaps the refreshed activity region back
// in. An unexpected sync failure renders the same region with a recoverable
// inline error beside the sync control — no redirect, no full-page replacement —
// leaving any already-loaded transactions in view.
func (h *HttpHandler) PostSync(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	if err := h.transactionsService.SyncTransactions(ctx); err != nil {
		slog.ErrorContext(ctx, "failed to sync transactions", "error", err)
		hasConnections, rows, aerr := h.activity(ctx)
		if aerr != nil {
			httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
				Status: http.StatusInternalServerError,
				Err:    aerr,
			})
			return
		}
		views.TransactionsContentFrag(hasConnections, rows, "We couldn't sync your transactions. Please try again.").Render(ctx, w)
		return
	}

	hasConnections, rows, err := h.activity(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.TransactionsContentFrag(hasConnections, rows, "").Render(ctx, w)
}

// activity reads the page's data: whether any bank is connected (the accounts
// overview's empty-vs-populated signal) and the most recent transactions. The
// read never calls the provider.
func (h *HttpHandler) activity(ctx contextx.ContextX) (bool, []transactions.RecentTransaction, error) {
	dashboard, err := h.accountsService.Dashboard(ctx)
	if err != nil {
		return false, nil, err
	}

	rows, err := h.transactionsService.RecentTransactions(ctx, recentTransactionsLimit)
	if err != nil {
		return false, nil, err
	}

	return dashboard.HasAccounts(), rows, nil
}
