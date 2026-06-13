package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /{$}", httpx.HandlerFunc(h.Home))
}
