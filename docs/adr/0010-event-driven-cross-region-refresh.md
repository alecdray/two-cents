# Event-driven cross-region refresh

When one user action must update regions of the page **beyond the element it targeted**, the acting handler announces the change with an HTMX event and **each affected region re-fetches itself**, rather than the handler rendering every region into one response. Out-of-band ([OOB](../design/oob-swaps.md)) swaps stay the tool for the narrow case where the regions are tightly coupled siblings the acting handler **already renders** in the same template.

The deciding axis is **who owns the regions that must change**:

- **The acting handler already owns and renders them** (same composer, same template — e.g. a re-categorize that also updates the totals row directly above it) → OOB. One round-trip, and the handler is the natural single source for those siblings anyway.
- **They are distant, cross-concern, or owned by another module** (a shared editor opened from many views; an aggregate on a page the editor knows nothing about) → an event. The handler emits a change signal; every region that cares listens and refreshes itself from **its own** endpoint.

The forcing case is a **reusable editor invoked across views**. With OOB, the editor's save handler would have to know, import, and render every caller's regions — the wrap's figures, the tracker's totals, the activity row — collapsing the reuse it was built for. With an event it stays context-free: it writes, emits the signal, and is done; the wrap, the tracker, and the activity list each subscribe and re-render their own regions on their own terms. This keeps the cross-module boundary intact — the editor's module never imports a caller's view layer.

The trade-off is request count: an event fans out into one self-refresh request per listening region, where an OOB response would have folded them into a single round-trip. For a self-hosted single-user app ([ADR-0001](0001-self-hosted-single-user-service.md)) that cost is irrelevant, and the decoupling is worth far more than the saved requests. The single-source-of-render rule that governs OOB fragments holds for self-refreshing regions too: each region is rendered by exactly one component, returned both on first paint and on its own refresh.

Rejected: OOB for everything (couples every acting handler to every region it touches, and makes a shared editor impossible to keep context-free); a full-page reload on each edit (loses the in-place feel and resets scroll/modal state).
