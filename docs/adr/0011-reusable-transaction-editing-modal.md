# Reusable transaction-editing modal

Editing a Transaction's categorization is **one modal editor reused by every view that lists transactions** — replacing the per-view inline pickers each surface used to carry. On save it issues the existing write operations and announces a change event so each view's regions self-refresh ([ADR-0010](0010-event-driven-cross-region-refresh.md)).

Editing had been duplicated: the activity list and the spend drill each carried their own inline classification/category and transfer-destination controls, and the wrap was about to add a third copy. The duplication is what made "the same editing experience everywhere" impossible to hold — three pickers drift. One editor invoked across views removes the copies and makes the shared experience structural.

A few consequences shape the decision:

- **The editor stays context-free, which is what lets it be shared.** It writes and emits the change event; it never renders or imports a caller's regions — each view's aggregates self-refresh on the event from their own endpoints ([ADR-0010](0010-event-driven-cross-region-refresh.md)). This is the boundary inline pickers could not cross: an inline picker that also refreshed the wrap's figures would have to know the wrap.
- **Shell vs content split by archetype.** The modal *shell* is a domain-free primitive (it knows nothing of transactions); the editing *content* speaks classifications, categories, and transfer subtypes, so it is owned by the transactions module and served by a transactions edit endpoint, loaded into the shared shell. A view reuses the editor by emitting a trigger that opens it — not by importing transactions' view layer.
- **It composes the existing write operations, each independently validated — not a merged write.** Re-categorization and the transfer-destination mark stay distinct operations the editor invokes in sequence, each enforcing its own coupling (a Spending choice requires a Category; only an outflow Transfer carries a destination, and the mark writes only the transfer facet). The editor unifies the *presentation* of editing; it does not collapse the operations into a single write that validates once.

Rejected: per-view inline pickers (the duplication this removes, and unshareable across views without coupling the editor to each caller); a single *merged* write that validates once and sets both facets together (it would lose the per-write coupling validation — the editor instead invokes the two validated operations in turn). The drill's own reconciliation invariant is unchanged ([ADR-0009](0009-category-spend-drill-down.md)) — only how its rows are edited changes.

How the editor is opened and how it lays out its controls are presentation details, not part of this decision; they live with the owning module (`src/internal/transactions/README.md`) and the templ.
