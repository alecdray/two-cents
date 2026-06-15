package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the budget editor page and its save action.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /budget", httpx.HandlerFunc(h.GetBudgetPage))
	mux.HandleFunc("POST /budget", httpx.HandlerFunc(h.PostBudget))
}
