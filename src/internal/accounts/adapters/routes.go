package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the accounts pages. The overview is the application's
// root page (GET /).
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /{$}", httpx.HandlerFunc(h.GetOverviewPage))
	mux.HandleFunc("GET /accounts/connections/link-token", httpx.HandlerFunc(h.GetConnectLinkToken))
	mux.HandleFunc("POST /accounts/connections", httpx.HandlerFunc(h.PostConnection))
}
