package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the dashboard pages: the current-month Tracker at the
// application root, the wraps list, a single month wrap, and the spend drill-down
// (a bucket's transactions; the same GET serves the region's transaction-changed
// self-refresh).
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /{$}", httpx.HandlerFunc(h.GetTrackerPage))
	mux.HandleFunc("GET /wraps", httpx.HandlerFunc(h.GetWrapsPage))
	mux.HandleFunc("GET /wraps/{ym}", httpx.HandlerFunc(h.GetWrapPage))
	mux.HandleFunc("GET /wraps/{ym}/spend/{bucket}", httpx.HandlerFunc(h.GetSpendDrillPage))
}
