# Claude Guidelines — Two Cents

Stack and architecture mirror the sibling project `wax` (see [ADR-0001](../docs/adr/0001-self-hosted-single-user-service.md)).

## Code generation

- After editing `.templ` files: `task build/templ` (generated files end in `_templ.go`, gitignored).
- After editing `db/queries/*.sql`: `task build/sqlc`. After adding a migration in `db/migrations/`: `task db/up`. Create migrations with `task db/create -- <name>`.

> sqlc is configured but not yet wired into `core/db` — it lands with the first domain module's queries. Until then `core/db.DB` exposes raw `*sql.DB` + `WithTx`.

## Architecture

`src/internal/` is organized by archetype. Every module declares its archetype in its own `CLAUDE.md`. Full rules: [`docs/architecture/`](../docs/architecture/). Pick an archetype before writing a new module.

- **domain module** — `service.go` + `repo.go` (only `repo.go` touches sqlc) + optional `task.go` + `adapters/`.
- **external-client** — `client.go` + `entities.go` + `service.go`; no persistence (e.g. `plaid`).
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
| Development process (spec→implement→audit→merge) | `docs/process.md` |
| Roadmap (status, committed work, backlog) | `docs/roadmap.md` |
| Scope & direction | `docs/scope.md` |
| PRD (v1 features, modules) | `docs/prd.md` |
| Domain model & glossary | `docs/domain/` |
| Architecture rules | `docs/architecture/` |
| Cross-cutting data model | `docs/architecture/data-model.md` |
| Design rules | `docs/design/` |
| Decision log (ADRs) | `docs/adr/` |
| Per-module behaviour, entities | `src/internal/<module>/README.md` |
| Per-module agent rules | `src/internal/<module>/CLAUDE.md` |

## Documentation practices

The `audit` skill (and its `docs-audit` child) enforces these. Run `/audit` before any merge or PR.

- After changing a module's logic, update its `README.md` and `CLAUDE.md` if anything they assert changed. Keep `CLAUDE.md` tight — it's auto-loaded into context.
- A module's `CLAUDE.md` describes **current state only** — no historical context, no forward-looking "lands in a later slice" notes, no comparative claims about other modules. History lives in commit messages and the build record; a brief transitional note is fine only while a migration is mid-flight.
- **Link, don't restate.** Before defining a concept in a doc, grep the canonical homes for an existing definition and link to it instead of re-prosing it. Canonical homes: domain language → `docs/domain/README.md`; decisions + rationale → `docs/adr/`; schema → `db/migrations/`.
- **No exhaustive lists.** The rule in [`docs/architecture/CLAUDE.md`](../docs/architecture/CLAUDE.md) applies equally to module READMEs, `CLAUDE.md` files, and everything under `docs/`.
- Add inline code comments only for context not evident from the code; never restate what the code does. A non-obvious invariant a refactor could silently break is exactly the kind of comment worth writing — and the code is its durable home (see the table below).

### Synchronized content

A few topics intentionally live in more than one place. **Edit every listed location when changing any of them:**

- **Data model** — cross-cutting decisions live in `docs/architecture/data-model.md`; per-entity meaning and key types live in each owning module's `README.md`; the domain glossary in `docs/domain/README.md` is canonical for term meaning. When adding, renaming, or removing an entity, update all three.
- **Design tokens** — token and named-role utility definitions live in `static/src/main.css` (truth); their conceptual roles live in `docs/design/design-system.md`. Update the doc when a token group or named-role utility changes, not when individual values shift.

Anything else that ends up duplicated should be removed from one location, not kept in sync.

### Working artifacts (not committed)

Spec, plan, and build-record files produced by skills (`/build`, `/to-issues`, `/to-prd`, `/grill-me`, etc.) are scratch artifacts. They live with the per-feature build record under `~/workshop/builds/two-cents-*/` (machine-local) and **must not be committed** to the repo. When the work merges, fold any durable learnings into the appropriate permanent home:

| Type of learning | Goes to |
|---|---|
| A reusable architectural rule | `docs/architecture/` (or a module's `CLAUDE.md`) |
| A reusable design rule or token | `docs/design/` (and `static/src/main.css` if applicable) |
| User-facing behaviour of a feature | the owning module's `README.md` |
| A decision worth preserving the "why" of | `docs/adr/NNNN-short-slug.md` |
| A subtle invariant a refactor could silently break | a doc-comment next to the code it guards |
| A known architectural divergence | `docs/architecture/known-gaps.md` |
| Status, backlog, or future direction | `docs/roadmap.md` |

If a learning doesn't fit any of these, it probably isn't worth persisting — let it die with the working file.
