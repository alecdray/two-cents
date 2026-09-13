package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the sweep snapshot page at /sweep, the per-snapshot deep
// link the older/newer steps use, the Run now action, and the schedule the next
// run is computed from.
//
// The schedule sits under /sweep because that is the only surface it appears on.
// Its mutations are POSTs rather than PUT/DELETE because they are submitted by
// plain HTML forms.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /sweep", httpx.HandlerFunc(h.GetPage))
	mux.HandleFunc("POST /sweep/run", httpx.HandlerFunc(h.PostRun))
	mux.HandleFunc("POST /sweep/schedule", httpx.HandlerFunc(h.PostScheduleItem))
	mux.HandleFunc("POST /sweep/schedule/{id}", httpx.HandlerFunc(h.PutScheduleItem))
	mux.HandleFunc("POST /sweep/schedule/{id}/delete", httpx.HandlerFunc(h.DeleteScheduleItem))
	mux.HandleFunc("GET /sweep/{id}", httpx.HandlerFunc(h.GetSnapshot))
}
