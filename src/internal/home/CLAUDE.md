# home — domain module (presentational stub)

Archetype: **domain module** — full rules in [`docs/architecture/archetypes/domain-module.md`](../../../docs/architecture/archetypes/domain-module.md).

Currently a thin presentational surface: it serves the root page (`GET /`) to validate the skeleton and has **no `service.go`/`repo.go` yet** because it owns no data. When the overview lands (total cash, credit debt, net cash), this module gains a service that composes the `accounts` / `tracker` / `reporting` services, and the page becomes the real dashboard.

- `adapters/views/home_page.templ` — the page (page-templ archetype).
- `adapters/http.go`, `adapters/routes.go` — handler + route registration.
