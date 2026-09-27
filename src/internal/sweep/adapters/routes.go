package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the sweep snapshot page at /sweep, the per-snapshot deep
// link the older/newer steps use, the Run now action, the schedule the next run
// is computed from, its per-item editor modal, and the decisions recorded
// against one of its occurrences.
//
// The schedule sits under /sweep because that is the only surface it appears on.
// Its mutations are POSTs rather than PUT/DELETE because they are submitted by
// plain HTML forms. The editor modal's own GET open route is /edit-suffixed so
// it never collides with the save POST at the bare {id}, the same shape the
// Rule editor's edit route uses ([ADR-0016]).
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /sweep", httpx.HandlerFunc(h.GetPage))
	mux.HandleFunc("POST /sweep/run", httpx.HandlerFunc(h.PostRun))
	mux.HandleFunc("POST /sweep/schedule", httpx.HandlerFunc(h.PostScheduleItem))
	mux.HandleFunc("GET /sweep/schedule/{id}/edit", httpx.HandlerFunc(h.GetEditScheduleItemModal))
	mux.HandleFunc("POST /sweep/schedule/{id}", httpx.HandlerFunc(h.PutScheduleItem))
	mux.HandleFunc("POST /sweep/schedule/{id}/delete", httpx.HandlerFunc(h.DeleteScheduleItem))
	mux.HandleFunc("POST /sweep/schedule/{id}/occurrence/{date}/match", httpx.HandlerFunc(h.PostOccurrenceMatch))
	mux.HandleFunc("POST /sweep/schedule/{id}/occurrence/{date}/confirm", httpx.HandlerFunc(h.PostOccurrenceConfirm))
	mux.HandleFunc("POST /sweep/schedule/{id}/occurrence/{date}/clear", httpx.HandlerFunc(h.PostOccurrenceClear))
	mux.HandleFunc("GET /sweep/{id}", httpx.HandlerFunc(h.GetSnapshot))
}
