package adapters

import (
	"github.com/alecdray/two-cents/src/internal/core/httpx"
)

// RegisterRoutes mounts the sweep snapshot page at /sweep, the per-snapshot deep
// link the older/newer steps use, the Run now action, the schedule the next run
// is computed from, its per-item editor modal (including its create-from-
// transaction and attach-to-existing-item modes, [ADR-0031]), and the decisions
// recorded against one of its occurrences.
//
// The schedule sits under /sweep because that is the only surface it appears on.
// Its mutations are POSTs rather than PUT/DELETE because they are submitted by
// plain HTML forms. The editor modal's own GET open routes are literal
// (/new, /attach, /{id}/edit) so none collides with the save POST at the bare
// {id} — Go's mux resolves a literal segment over a wildcard at the same
// position, the same shape the Rule editor's edit route uses ([ADR-0016]).
//
// GET /sweep/schedule is a plain read of the schedule region alone — no page
// shell, no snapshot — the endpoint the region's own hidden listener hx-gets
// on schedule-changed ([ADR-0010]) so a currently-loaded /sweep page picks up
// a mutation whose response went to a transaction's editor instead.
func RegisterRoutes(mux *httpx.Mux, h *HttpHandler) {
	mux.HandleFunc("GET /sweep", httpx.HandlerFunc(h.GetPage))
	mux.HandleFunc("POST /sweep/run", httpx.HandlerFunc(h.PostRun))
	mux.HandleFunc("POST /sweep/schedule", httpx.HandlerFunc(h.PostScheduleItem))
	mux.HandleFunc("GET /sweep/schedule", httpx.HandlerFunc(h.GetScheduleRegion))
	mux.HandleFunc("GET /sweep/schedule/new", httpx.HandlerFunc(h.GetNewScheduleItemModal))
	mux.HandleFunc("GET /sweep/schedule/attach", httpx.HandlerFunc(h.GetAttachScheduleItemModal))
	mux.HandleFunc("POST /sweep/schedule/attach", httpx.HandlerFunc(h.PostAttachScheduleItem))
	mux.HandleFunc("GET /sweep/schedule/{id}/edit", httpx.HandlerFunc(h.GetEditScheduleItemModal))
	mux.HandleFunc("POST /sweep/schedule/{id}", httpx.HandlerFunc(h.PutScheduleItem))
	mux.HandleFunc("POST /sweep/schedule/{id}/delete", httpx.HandlerFunc(h.DeleteScheduleItem))
	mux.HandleFunc("POST /sweep/schedule/{id}/occurrence/{date}/match", httpx.HandlerFunc(h.PostOccurrenceMatch))
	mux.HandleFunc("POST /sweep/schedule/{id}/occurrence/{date}/confirm", httpx.HandlerFunc(h.PostOccurrenceConfirm))
	mux.HandleFunc("POST /sweep/schedule/{id}/occurrence/{date}/clear", httpx.HandlerFunc(h.PostOccurrenceClear))
	mux.HandleFunc("GET /sweep/{id}", httpx.HandlerFunc(h.GetSnapshot))
}
