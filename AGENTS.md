# Agent Guidelines — Two Cents

> **Convention note:** This file is the tool-agnostic entry point. Claude Code loads it via the sibling `CLAUDE.md` symlink; other agents load `AGENTS.md` directly. The same pattern holds throughout the repo — every `AGENTS.md` has a generated `CLAUDE.md` symlink beside it, and `.claude/` is a symlink to the canonical `.agents/`. Git hooks in `.githooks/` regenerate these symlinks automatically on checkout/merge/rewrite (enable with `git config core.hooksPath .githooks`).

Stack and architecture mirror the sibling project `wax` (see [ADR-0001](docs/adr/0001-self-hosted-single-user-service.md)).

## Starting new work

All new work ships through the four-phase process in [`docs/process.md`](docs/process.md) — read it before planning a feature. **Spec is phase 1:** create the work's spec folder [`docs/spec/<timestamp>-<name>/`](docs/spec/) (scope, goals, spec) and codify the design into the canonical docs (ADRs, module READMEs) on a branch. `/build` and similar tools are phase-2 (Implement) tools, never the entry point.

## Skills

Project skills live in [`.agents/skills/`](.agents/skills/).

| Skill | When |
|---|---|
| `/spec` | phase 1 — start new work, create the spec folder |
| `/implement` | phase 2 — build, after canonical docs are updated |
| `/audit` | phase 3 — pre-merge gate, code + docs in one pass |
| `/code-review` | adversarial review of an implementation, or when `/audit` isn't the right tool |
| `/prose-compact` | tighten a wall of prose without losing facts |

## Code generation

- After editing `.templ` files: `task build/templ` (generated files end in `_templ.go`, gitignored).
- After editing `db/queries/*.sql`: `task build/sqlc`. After adding a migration in `db/migrations/`: `task db/up`. Create migrations with `task db/create -- <name>`.

## Architecture

`src/internal/` is organized by archetype. Every module declares its archetype in its own `AGENTS.md`. Full rules: [`docs/architecture/`](docs/architecture/). Pick an archetype before writing a new module.

- **domain module** — `service.go` + `repo.go` (only `repo.go` touches sqlc) + optional `task.go` + `adapters/`.
- **external-client** — `client.go` + `entities.go` + `service.go`; no persistence (e.g. `plaid`).
- **utility** — pure, no persistence (e.g. `tracker`, `reporting`).
- **singletons** — `core/` (shared infra) and `server/` (composition root).

## Design

Every `.templ` is one of three archetypes (page / fragment / primitive), by location. Cross-cutting rules (HTMX-first, fragments over pages, inline errors, theme tokens, and **cross-region updates — events vs OOB**, [ADR-0010](docs/adr/0010-event-driven-cross-region-refresh.md)) and the visual vocabulary (Tailwind + DaisyUI `twocents` theme) live in [`docs/design/`](docs/design/).

## Development

- Use `task` for all build/run/test ops (`task` with no args lists targets). Prefer `task <name>` over invoking tools directly.
- All `go build` output goes to `./bin/` via `-o ./bin/<name>` — never the project root.
- Env vars documented in `.env.template`. Default dev port is **4690**.

## Testing

Strategy, conventions, and the gate: [`docs/testing.md`](docs/testing.md).

## Documentation map

| Topic | Location |
|---|---|
| Development process (spec→implement→audit→merge) | `docs/process.md` |
| Per-chunk spec record (scope, goals, spec; frozen at merge) | `docs/spec/<timestamp>-<name>/` |
| Reading guide to the whole organizing system | `PATTERNS.md` |
| Product vision & philosophy | `docs/product/vision.md` |
| Product features (present-state, what + why) | `docs/product/` |
| Backlog (features and bugs) | `docs/backlog/` |
| Domain model & glossary | `docs/domain/` |
| Architecture rules | `docs/architecture/` |
| Cross-cutting data model | `docs/architecture/data-model.md` |
| Design rules | `docs/design/` |
| Decision log (ADRs) | `docs/adr/` |
| Per-module behaviour, entities | `src/internal/<module>/README.md` |
| Per-module agent rules | `src/internal/<module>/AGENTS.md` |

> **Transitional:** `docs/scope.md`, `docs/prd.md`, and `docs/roadmap.md` are the prior planning docs. Their durable content now lives in `docs/product/` (vision + features) and `docs/backlog/` (features + bugs) — the going-forward homes. The three legacy docs remain until their inbound references and the `scripts/check-docs-provider.sh` guard are repointed, then they're retired.

## Documentation practices

The `audit` skill (and its `docs-audit` child) enforces these. Run `/audit` before any merge or PR.

- After changing a module's logic, update its `README.md` and `AGENTS.md` if anything they assert changed. Keep `AGENTS.md` tight — it's auto-loaded into context.
- A module's `AGENTS.md` describes **current state only** — no historical context, no forward-looking "lands in a later slice" notes, no comparative claims about other modules. History lives in commit messages and the spec record; a brief transitional note is fine only while a migration is mid-flight.
- **Link, don't restate.** Before defining a concept in a doc, grep the canonical homes for an existing definition and link to it instead of re-prosing it. Canonical homes: domain language → `docs/domain/README.md`; decisions + rationale → `docs/adr/`; schema → `db/migrations/`.
- **Make a new reference doc discoverable.** A convention or reference doc is scoped to one topic and **registered in its directory's index** (`README.md`) with a terse, accurate one-line scope — that index is what `audit` reads to discover what to check, so an unregistered doc is invisible to the gate and a registered one whose scope has outgrown its line now misleads. Broaden a doc and fix its index line in the same change; the line and the doc's real scope must never drift. A rule that must reach implementers or auditors is stated **once** in its scoped doc, then reached by terse pointers from the surfaces those agents load (the audit's area docs, e.g. `docs/design/principles.md`; the relevant `AGENTS.md`, auto-loaded at the point of work) — never a second copy.
- **No exhaustive lists.** The rule in [`docs/architecture/AGENTS.md`](docs/architecture/AGENTS.md) applies equally to module READMEs, `AGENTS.md` files, and everything under `docs/`.
- Add inline code comments only for context not evident from the code; never restate what the code does. A non-obvious invariant a refactor could silently break is exactly the kind of comment worth writing — and the code is its durable home (see the table below).

### Synchronized content

A few topics intentionally live in more than one place. **Edit every listed location when changing any of them:**

- **Data model** — cross-cutting decisions live in `docs/architecture/data-model.md`; per-entity meaning and key types live in each owning module's `README.md`; the domain glossary in `docs/domain/README.md` is canonical for term meaning. When adding, renaming, or removing an entity, update all three.
- **Design tokens** — token and named-role utility definitions live in `static/src/main.css` (truth); their conceptual roles live in `docs/design/design-system.md`. Update the doc when a token group or named-role utility changes, not when individual values shift.

Anything else that ends up duplicated should be removed from one location, not kept in sync.

### Working docs and durable outputs

The working docs for a chunk of work — scope, goals, spec, supporting documents — are **committed** under [`docs/spec/<timestamp>-<name>/`](docs/spec/), reconciled to what shipped, and frozen at merge (see [`docs/spec/README.md`](docs/spec/README.md)). Genuinely throwaway scratch (raw exploration, dead-end notes) stays outside the repo (`/tmp`, …) and is never committed.

A spec folder is a point-in-time record, **not** a canonical home. During Implement, durable outputs are codified into their canonical homes:

| Type of learning | Goes to |
|---|---|
| A reusable architectural rule | `docs/architecture/` (or a module's `AGENTS.md`) |
| A reusable design rule or token | `docs/design/` (and `static/src/main.css` if applicable) |
| User-facing behaviour of a feature | the owning module's `README.md` (and `docs/product/` for the composed view) |
| A decision worth preserving the "why" of | `docs/adr/NNNN-short-slug.md` |
| A subtle invariant a refactor could silently break | a doc-comment next to the code it guards |
| A known architectural divergence | `docs/architecture/known-gaps.md` |
| Backlog items or future direction | `docs/backlog/` |

The spec folder is preserved beside these as the narrative of how the work landed — not a substitute for them.
