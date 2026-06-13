# Out-of-band swaps

When a server response needs to update a region of the page that the request did not target — typically because a single user action affects multiple parts of the UI — HTMX uses out-of-band (OOB) swaps. The response includes additional fragments marked `hx-swap-oob="true"`; HTMX matches each one by `id` and replaces the existing element in place.

A natural Two Cents case: re-categorizing a Transaction updates the row itself **and** the spending-by-category totals and the budget-remaining figure elsewhere on the page — one action, several regions.

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

## Relationship to testids

The shared region carries the testid in exactly one place (see [testids.md](testids.md)). Both the initial render and the OOB swap inherit it.
