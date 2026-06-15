package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the accounts pages. The overview lives at /accounts (the
// dashboard Tracker owns the application root /).
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /accounts", httpx.HandlerFunc(h.GetOverviewPage))
	mux.HandleFunc("GET /accounts/connections/link-token", httpx.HandlerFunc(h.GetConnectLinkToken))
	mux.HandleFunc("POST /accounts/connections", httpx.HandlerFunc(h.PostConnection))
	mux.HandleFunc("DELETE /accounts/connections/{id}", httpx.HandlerFunc(h.DeleteConnection))
	mux.HandleFunc("GET /accounts/connections/{id}/relink-token", httpx.HandlerFunc(h.GetReconnectLinkToken))
	mux.HandleFunc("POST /accounts/connections/{id}/reconnect", httpx.HandlerFunc(h.PostReconnect))
}
