# Cross-region updates: OOB swaps vs events

When one user action must update regions of the page **beyond the element it targeted**, there are two mechanisms. Choose by **who owns the regions that must change** ([ADR-0010](../adr/0010-event-driven-cross-region-refresh.md)):

- **Out-of-band (OOB) swap** — the acting handler renders the extra regions into its own response, each marked `hx-swap-oob="true"`; HTMX matches them by `id` and replaces them in place. Use it when the regions are **tightly-coupled siblings the handler already renders** in the same template (e.g. a re-categorize updating the totals row directly above it). One round-trip; the handler is the natural source for those siblings.
- **Event-driven self-refresh** — the handler emits a change signal (`HX-Trigger`) and each region that cares **re-fetches itself** from its own endpoint (`hx-trigger="<event> from:body"` + its own `hx-get`). Use it when the regions are **distant, cross-concern, or owned by another module** — above all when a **reusable component is invoked across views** (a shared editor that must not know or render its callers' regions). The handler stays context-free; each caller subscribes on its own terms.

The same single-source-of-render rule (below) governs **both**: every swappable or self-refreshing region is defined by exactly one component.

## Out-of-band swaps

The OOB response includes additional fragments marked `hx-swap-oob="true"`; HTMX matches each one by `id` and replaces the existing element in place.

A natural Two Cents case: re-categorizing a Transaction inline updates the row itself **and** the spending-by-category totals and the budget-remaining figure on the **same** page — one action, several sibling regions one handler owns.

## Rule: the OOB fragment defines no HTML of its own

Any element that is the target of an OOB swap must be defined in **exactly one shared region templ** — one component owns the region's id, structure, classes, and testid. Both the initial render and the OOB response render through that component, with the OOB call setting `hx-swap-oob="true"` via an `isOOB` parameter:

```templ
templ trackerRemainingRegion(category string, remaining tracker.RemainingDTO, isOOB bool) {
  <div
    id={ trackerRemainingID(category) }
    class="flex flex-col gap-1"
    if isOOB {
      hx-swap-oob="true"
    }
  >
    @remainingReadout(remaining)
  </div>
}
```

Initial render — `@trackerRemainingRegion(cat, remaining, false)`.
OOB response — `@trackerRemainingRegion(cat, remaining, true)`.

"Region" is the same term used in [principles.md](principles.md) for "DOM ids belong to the templ that owns the region" — an OOB swap target is exactly such a region, viewed through its swap-time lens.

## Why

The OOB response and the initial render are the **same element** at two points in its lifecycle. If they are defined in two places, they will drift — id, class, testid, layout, structure. The drift is invisible until the moment of an OOB swap, when the element subtly changes shape under the user (different gap, missing testid, broken HTMX target). Both definitions look reasonable in isolation, so the bug is slow to spot. Single-sourcing the element makes the drift impossible.

## Single-region vs multi-region OOB

When the OOB response updates exactly one region, the region templ *is* the fragment a handler returns — no separate wrapper, no extra noun in the name. The templ is exported as `<Area>Frag` (e.g. `BudgetRemainingFrag`), the handler calls `views.BudgetRemainingFrag(budget, true).Render(...)`, and the templ's testid is `<area>` (`budget-remaining`). The `*Frag` archetype suffix already conveys "this is an HTML fragment, OOB-swappable via `isOOB`" — no extra "Region" noun in the middle.

When the response needs to update multiple regions in one round-trip, a composition fragment bundles them. Its body is a sequence of `@region(..., true)` calls and nothing else — it defines no HTML of its own. The composition fragment is exported as `*OOBFrag` (e.g. `RecategorizeOOBFrag`); the regions it composes are private templs named `<area>Region` (e.g. `transactionRowRegion`, `categoryTotalsRegion`).

## OOB-only elements

If an element only exists via OOB (e.g. a toast appended into a container that already exists in the initial render), the rule still holds. The toast itself is a shared region with a single definition; the container is the element that exists in the initial render; the toast is what gets swapped into it.

## Event-driven self-refresh

The acting handler sets an `HX-Trigger` response header naming the change (e.g. `transaction-changed`) and renders only its own target — it does **not** render the distant regions. Each region that depends on that change carries `hx-trigger="transaction-changed from:body"` alongside its own `hx-get`, so it re-fetches itself when the event fires. The event name is the contract; neither side imports the other.

The single-source rule still holds: a self-refreshing region is an `<Area>Frag` that renders the same element on first paint and on its own refresh — it is just reached by the region's own GET rather than folded into the acting handler's response. A handler may emit several event names when an action has distinct downstream concerns, and a region may listen for more than one.

Carry only an event **name** (and at most a small scalar in its detail) across the boundary — never rendered HTML or a region id the emitter shouldn't know. The emitter announces *that* something changed; each listener decides *what* to re-fetch.

## Relationship to testids

The shared region carries the testid in exactly one place (see [testids.md](testids.md)). Both the initial render and any swap — OOB or self-refresh — inherit it.
