package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the sweep snapshot page at /sweep, the per-snapshot deep
// link the older/newer steps use, and the Run now action.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /sweep", httpx.HandlerFunc(h.GetPage))
	mux.HandleFunc("POST /sweep/run", httpx.HandlerFunc(h.PostRun))
	mux.HandleFunc("GET /sweep/{id}", httpx.HandlerFunc(h.GetSnapshot))
}
