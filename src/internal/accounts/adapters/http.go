package adapters

import (
	"net/http"

	"github.com/alecdray/two-cents/src/internal/accounts"
	"github.com/alecdray/two-cents/src/internal/accounts/adapters/views"
	"github.com/alecdray/two-cents/src/internal/core/contextx"
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// HttpHandler serves the accounts module's pages. It holds the accounts Service
// and reads through it; it never reaches the bank provider directly.
type HttpHandler struct {
	accountsService *accounts.Service
}

// NewHttpHandler builds the handler over the accounts Service.
func NewHttpHandler(accountsService *accounts.Service) *HttpHandler {
	return &HttpHandler{accountsService: accountsService}
}

// GetOverviewPage renders the root accounts overview: the net cash position and
// the linked accounts grouped into cash, credit, and other.
func (h *HttpHandler) GetOverviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := contextx.NewContextX(r.Context())

	dashboard, err := h.accountsService.Dashboard(ctx)
	if err != nil {
		httpx.HandleErrorResponse(ctx, w, httpx.HandleErrorResponseProps{
			Status: http.StatusInternalServerError,
			Err:    err,
		})
		return
	}

	views.AccountsOverviewPage(dashboard).Render(ctx, w)
}
