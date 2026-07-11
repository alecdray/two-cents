package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
	"github.com/alecdray/two-cents/src/internal/transactions"
)

// RegisterRoutes mounts the transactions pages: the recent-activity list, the
// on-demand sync, the shared edit modal, and the cached merchant-logo image endpoint.
// All mount on the passed mux, so they inherit whatever middleware it carries — the
// composition root mounts them on the session-guarded mux, like every other authed
// route.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /transactions", httpx.HandlerFunc(h.GetTransactionsPage))
	mux.HandleFunc("POST /transactions/sync", httpx.HandlerFunc(h.PostSync))
	mux.HandleFunc("GET /transactions/{id}/edit", httpx.HandlerFunc(h.GetEditModal))
	mux.HandleFunc("POST /transactions/{id}/edit", httpx.HandlerFunc(h.PostEdit))
	mux.HandleFunc("GET "+transactions.MerchantLogoRoutePrefix+"{key}", httpx.HandlerFunc(h.GetMerchantLogo))
}
