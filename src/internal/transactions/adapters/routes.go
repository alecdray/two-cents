package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the transactions pages: the recent-activity list and the
// on-demand sync.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /transactions", httpx.HandlerFunc(h.GetTransactionsPage))
	mux.HandleFunc("POST /transactions/sync", httpx.HandlerFunc(h.PostSync))
	mux.HandleFunc("POST /transactions/{id}/categorize", httpx.HandlerFunc(h.PostCategorize))
	mux.HandleFunc("POST /transactions/{id}/transfer-destination", httpx.HandlerFunc(h.PostTransferDestination))
}
