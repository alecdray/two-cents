# Bottom-bar navigation

Primary navigation is a **fixed bottom bar carrying a few primary destinations plus a "More" overflow**, replacing the top strip that listed every destination in a row. The bar is the thumb-reachable mobile idiom for a phone-first app; the overflow absorbs the secondary destinations and sign-out that a single row could not hold without crowding.

The strip listed all destinations equally and pushed sign-out to the far edge. As destinations grew past a handful, the row stopped fitting a narrow viewport — the failure this replaces. A bottom bar fixes a small set of primary destinations in reach of the thumb and moves the rest behind one overflow control, so adding a destination never again pressures the primary row.

A few consequences shape the decision:

- **One layout at every width, not a per-breakpoint swap.** The bar tracks the content column — flush across the bottom on a phone, floating and centered to that column's width on a wider screen — rather than reverting to a top strip on desktop. Chosen so there is a single navigation to build, style, and test; the phone-first app does not earn a second desktop-only idiom ([principles: mobile-first](../design/principles.md)).
- **The navbar gains the active destination, and stays a primitive.** Highlighting the current destination requires the bar to know it; the current request path is not in the request context, and threading it there would be a heavier cross-cutting change. Instead each page names its own destination to the bar as a checked value from the bar's *own* set of destinations — not a domain type — so the bar stays domain-free. A destination that lives in the overflow highlights the overflow control when active.
- **The overflow menu is client-only, distinct from the app's HTMX modal.** It is static links to destinations, so it opens with no server round-trip — unlike the reusable modal that fetches domain content ([ADR-0011](0011-reusable-transaction-editing-modal.md)) and unlike the event-driven cross-region refresh ([ADR-0010](0010-event-driven-cross-region-refresh.md)). It reuses the app's bottom-anchored sheet shape for consistency, but as ephemeral client-only UI state, not a domain interaction. Sign-out remains a full-page navigation (it opts out of HTMX boosting so the server's redirect takes the whole page).

Rejected: keeping the top strip with horizontal scrolling or wrapping (defers the crowding rather than resolving it, and abandons thumb reach); a desktop-only top bar paired with the mobile bottom bar (two layouts to keep in sync for no gain on a single-user phone-first app); loading the overflow menu through the HTMX modal (a server round-trip and domain-content machinery for what is a static link list).

Which destinations are primary versus overflow, the bar's exact layout, and how the sheet is opened are presentation details that live with the owning primitive and the templ, not part of this decision.
