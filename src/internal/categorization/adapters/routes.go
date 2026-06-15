package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the categorization pages: the Category taxonomy and the
// user Rules, each with its create / edit / archive / delete mutations.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /categories", httpx.HandlerFunc(h.GetCategoriesPage))
	mux.HandleFunc("POST /categories", httpx.HandlerFunc(h.PostCategory))
	mux.HandleFunc("POST /categories/{id}/rename", httpx.HandlerFunc(h.PostRenameCategory))
	mux.HandleFunc("POST /categories/{id}/archive", httpx.HandlerFunc(h.PostArchiveCategory))
	mux.HandleFunc("POST /categories/{id}/unarchive", httpx.HandlerFunc(h.PostUnarchiveCategory))

	mux.HandleFunc("GET /rules", httpx.HandlerFunc(h.GetRulesPage))
	mux.HandleFunc("POST /rules", httpx.HandlerFunc(h.PostRule))
	mux.HandleFunc("POST /rules/{id}/edit", httpx.HandlerFunc(h.PostEditRule))
	mux.HandleFunc("POST /rules/{id}/delete", httpx.HandlerFunc(h.PostDeleteRule))
}
