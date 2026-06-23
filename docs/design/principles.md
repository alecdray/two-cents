# Design Principles

Cross-cutting rules that apply to every archetype. These describe what is already true of the codebase — a principle lands here when it lands in the code, not before.

## Mobile-first

The interface is designed for a phone in hand. Base styles target a narrow viewport; Tailwind's `sm:` / `md:` / `lg:` modifiers progressively enhance for larger screens — desktop is the responsive variant, not the default that mobile breaks out of. Concretely: the body sets a narrow `min-w-xs` baseline, modals anchor to the bottom on mobile and recentre on `sm:` (`modal-bottom sm:modal-middle`), and the document carries the iOS web-app meta tags and PWA manifest. New layouts follow the same pattern: write the mobile layout first, then add `sm:`/`md:` overrides for wider screens.

## Dark-mode-first

The interface is designed dark. The `twocents` theme sets `color-scheme: dark`, the document renders with `data-theme="twocents"`, and every surface, token, and `-content` pairing is tuned for light text on dark surfaces — dark is the default and the baseline that components are built against, not an inverted afterthought. A finance app gets opened often and at night; low-glare surfaces are the comfortable default. If a light theme is ever added, it is a *second* DaisyUI theme toggled at the root — dark stays the default, and components keep using semantic tokens (`bg-base-100`, `text-default`, `*-content` pairs) so they work under either theme without change. Never hard-code colors that assume a dark background; reach for the token whose role survives a theme swap.

## Server renders HTML; HTMX drives interaction

The interaction model is server-rendered HTML over HTMX, not client-rendered components. Forms submit with `hx-post` / `hx-put` and the server responds with an HTML fragment. Page navigation uses `hx-boost` on the body so links morph the relevant region instead of full-page reloading. JavaScript is reserved for genuinely client-only state (Alpine `x-data` for ephemeral UI state); it is not the medium for fetching, validating, or transforming domain data.

## Fragments over pages

When only a slice of the UI needs to change, the handler returns a fragment, not a full page. A fragment swaps into the existing DOM; a full page re-renders everything. The fragment is the reusable unit (see [archetypes/fragment-templ.md](archetypes/fragment-templ.md)); a page templ wraps a fragment in the shared layout when the same content is also reachable directly by URL.

## Errors render inline, in place

When a request fails in a way the user can recover from, the server returns an error component scoped to the relevant region — not a redirect, not a global alert, not a banner on the next page. The error appears where the action was taken. The mechanism is `httpx.HandleErrorResponse` plus an error templ component sized to the swap target.

## DOM ids belong to the templ that owns the region

If a templ defines a region that HTMX targets by id, the id-generating helper lives next to that templ (same file or its `.go` sibling). Callers obtain the id by calling the helper, not by hard-coding the string. The templ stays the single source of truth for what its swap target is named.

## Every templ root carries a testid

Every templ component's top-level root element carries a `data-testid` derived from the component name. The testid is the stable selector for tests, for `hx-target="closest [data-testid='...']"`, and for ad-hoc tooling — it does not depend on Tailwind classes that change with styling. The naming rules and the OOB/dual-use cases are in [testids.md](testids.md).

## Cross-region updates: choose OOB vs events, single-source the region

When an action must update regions beyond its target, pick the mechanism by **who owns those regions** ([ADR-0010](../adr/0010-event-driven-cross-region-refresh.md)): an **OOB swap** for tightly-coupled siblings the acting handler already renders in the same template; an **HTMX event** that each region self-refreshes on for distant, cross-concern, or cross-module regions — above all when a reusable component is invoked across views (the editor must not render or import its callers' regions). Either way the region is defined in **exactly one shared templ component**, rendered identically on first paint and on its swap/refresh (the OOB caller flips `hx-swap-oob="true"` via an `isOOB` parameter) — single-sourcing prevents drift that is otherwise invisible until the swap. The choice rule, the event mechanics, and the OOB-only-element case are in [oob-swaps.md](oob-swaps.md).

## Theme tokens, not raw colors

Styling uses the DaisyUI theme tokens defined in `static/src/main.css` (`bg-base-100`, `text-primary-content`, `border-accent`, etc.), not hex literals or one-off CSS variables in markup. When a new color is needed, it is added to the theme as a semantic token, not embedded inline at the call site.

The text emphasis scale is `text-default`, `text-muted`, `text-subtle`, `text-ghost`; the element-state utilities are `.is-disabled`, `.is-deemphasized`, `.hover-fade-out`, and `.hover-fade-in`. Raw `opacity-NN`, raw `text-base-content/NN`, and arbitrary `/NN` opacity on brand or semantic colors (e.g. `text-primary/80`, `text-error/50`) should not appear in templ markup outside these utilities. See `design-system.md` for the role of each utility and `static/CLAUDE.md` for the definitions.
