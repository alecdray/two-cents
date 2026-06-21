# Two Cents Design

This directory documents the UI/UX rules for `.templ` files and the conventions that shape them. Every `.templ` file in the project is one of three archetypes; the design principles and design-system vocabulary apply across all of them.

## Archetypes

| Archetype | Doc | What it owns |
|---|---|---|
| Page templ | [archetypes/page-templ.md](archetypes/page-templ.md) | A full-page HTML response, wrapped in the shared layout |
| Fragment templ | [archetypes/fragment-templ.md](archetypes/fragment-templ.md) | An HTML fragment for HTMX swap; no layout wrapper |
| Primitive | [archetypes/primitive.md](archetypes/primitive.md) | A reusable visual building block; no domain concepts |

## Archetype by location and name

Archetype is determined by where the file lives and its suffix — no per-file declaration:

- `src/internal/<module>/adapters/views/<surface>_page.templ` — **page templ**. Wraps its content in `templates.PageLayoutComponent`. Exported component matches the file: `<Surface>Page`.
- `src/internal/<module>/adapters/views/<surface>_frag.templ` — **fragment templ**. No layout wrapper. Exported component: `<Surface>Frag`.
- `src/internal/core/templates/*.templ` — **primitive**.

The suffix names the archetype; the layout wrapper (or absence of it) is the structural consequence. A `.templ` file in `adapters/views/` without a `_page` or `_frag` suffix is missing its archetype — pick one before adding the file.

The `views/` sub-package contains every `.templ` in a module and is its own Go package (`package views`). The handler code in `adapters/http.go` imports it and calls components by qualified name (`views.OverviewPage(...)`). This keeps the rendering layer cleanly separated from the HTTP wiring (`http.go`, `routes.go`) at the package level.

## Cross-cutting rules

- **[principles.md](principles.md)** — design rules that apply across every archetype (HTMX-first interaction, fragments over pages, error handling, theme tokens).
- **[design-system.md](design-system.md)** — the visual vocabulary: theme tokens, typography, animations, client-side libraries.
- **[testids.md](testids.md)** — `data-testid` naming convention for templ roots and descendants.
- **[oob-swaps.md](oob-swaps.md)** — choosing between OOB swaps and event-driven self-refresh for cross-region updates, and single-sourcing the swapped/refreshed region.

## Singletons

- **`src/internal/core/templates/`** — the home of primitives. Its own [`CLAUDE.md`](../../src/internal/core/templates/CLAUDE.md) declares the rules.

## Relationship to architecture docs

The architecture docs ([`docs/architecture/`](../architecture/)) describe how modules are structured — including the `adapters/` directory at a structural level (handler shape, import rules, route registration). The design docs pick up where the architecture docs stop: *what the templ files inside `adapters/` actually look like*, and the visual / interaction conventions that bind them together.
