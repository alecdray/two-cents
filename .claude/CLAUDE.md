# Claude Guidelines — Two Cents

Stack and architecture mirror the sibling project `wax` (see [ADR-0001](../docs/adr/0001-self-hosted-single-user-service.md)).

## Code generation

- After editing `.templ` files: `task build/templ` (generated files end in `_templ.go`, gitignored).
- After editing `db/queries/*.sql`: `task build/sqlc`. After adding a migration in `db/migrations/`: `task db/up`. Create migrations with `task db/create -- <name>`.

> sqlc is configured but not yet wired into `core/db` — it lands with the first domain module's queries. Until then `core/db.DB` exposes raw `*sql.DB` + `WithTx`.

## Architecture

`src/internal/` is organized by archetype. Every module declares its archetype in its own `CLAUDE.md`. Full rules: [`docs/architecture/`](../docs/architecture/). Pick an archetype before writing a new module.

- **domain module** — `service.go` + `repo.go` (only `repo.go` touches sqlc) + optional `task.go` + `adapters/`.
- **external-client** — `client.go` + `entities.go` + `service.go`; no persistence (e.g. `teller`).
- **utility** — pure, no persistence (e.g. `tracker`, `reporting`).
- **singletons** — `core/` (shared infra) and `server/` (composition root).

## Design

Every `.templ` is one of three archetypes (page / fragment / primitive), by location. Cross-cutting rules (HTMX-first, fragments over pages, inline errors, theme tokens) and the visual vocabulary (Tailwind + DaisyUI `twocents` theme) live in [`docs/design/`](../docs/design/).

## Development

- Use `task` for all build/run/test ops (`task` with no args lists targets). Prefer `task <name>` over invoking tools directly.
- All `go build` output goes to `./bin/` via `-o ./bin/<name>` — never the project root.
- Env vars documented in `.env.template`. Default dev port is **4690**.

## Testing

Strategy, conventions, and the gate: [`docs/testing.md`](../docs/testing.md).

## Documentation map

| Topic | Location |
|---|---|
| Scope & direction | `docs/scope.md` |
| PRD (v1 features, modules) | `docs/prd.md` |
| Domain model & glossary | `docs/domain/` |
| Architecture rules | `docs/architecture/` |
| Cross-cutting data model | `docs/architecture/data-model.md` |
| Design rules | `docs/design/` |
| Decision log (ADRs) | `docs/adr/` |
| Per-module behaviour, entities | `src/internal/<module>/README.md` |
| Per-module agent rules | `src/internal/<module>/CLAUDE.md` |
