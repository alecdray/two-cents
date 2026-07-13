package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the sweep recommendation page at /sweep.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /sweep", httpx.HandlerFunc(h.GetPage))
}
