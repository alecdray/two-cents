package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the dashboard pages: the current-month Tracker at the
// application root, the wraps list, a single month wrap, and the spend
// drill-down (a bucket's transactions, plus its in-place re-categorize).
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /{$}", httpx.HandlerFunc(h.GetTrackerPage))
	mux.HandleFunc("GET /wraps", httpx.HandlerFunc(h.GetWrapsPage))
	mux.HandleFunc("GET /wraps/{ym}", httpx.HandlerFunc(h.GetWrapPage))
	mux.HandleFunc("GET /wraps/{ym}/spend/{bucket}", httpx.HandlerFunc(h.GetSpendDrillPage))
	mux.HandleFunc("POST /wraps/{ym}/spend/{bucket}/categorize/{id}", httpx.HandlerFunc(h.PostDrillCategorize))
}
