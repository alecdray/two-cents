# Two Cents Architecture

This directory documents the architectural rules for `src/internal/`. Most directories under `src/internal/` are classified into one of three archetypes; the rest declare a role of their own. Every directory declares what it is on the first line of its own `AGENTS.md` — that declaration is the truth, and the [grep below](#listing-archetypes-at-a-glance) lists them.

## Shape

Single Go binary backed by SQLite, deployed as one container. Server-rendered HTML via Templ + HTMX; no external database, cache, or message bus. Static assets bundled at build time. The only network dependency is the bank provider, reached through a single external-client module.

## Data model

Cross-cutting design decisions live in [data-model.md](data-model.md). Per-entity meaning lives in each owning module's `README.md`. The schema itself is the truth — the migrations under [`db/migrations/`](../../db/migrations/), surfaced as type-safe queries in [`db/queries/`](../../db/queries/).

## Archetypes

| Archetype | Doc | What it owns |
|---|---|---|
| Domain module | [archetypes/domain-module.md](archetypes/domain-module.md) | A slice of business logic + persistence + (optionally) HTTP, end to end |
| External client | [archetypes/external-client.md](archetypes/external-client.md) | A wrapped third-party API; no domain concepts, no DB |
| Utility | [archetypes/utility.md](archetypes/utility.md) | Stateless, domain-shaped helpers; pure functions and/or embedded data |

See *Listing archetypes at a glance* below to find each existing module's classification.

## Directories that are not archetypes

An archetype describes a category with multiple instances. A directory that is one of a kind is not forced into one — that would require carving exceptions into the archetype's import rules. Each states its role and its rules in its own `AGENTS.md`:

- **`server/`** — composition root. Builds services, sets up middleware and sub-muxes, calls each domain module's `RegisterRoutes`, runs lifecycle (including registering the sync task on the cron). See [`src/internal/server/AGENTS.md`](../../src/internal/server/AGENTS.md).
- **`core/`** — shared infrastructure. Framework-level sub-packages used by 2+ modules. See [`src/internal/core/AGENTS.md`](../../src/internal/core/AGENTS.md).

Others declare a narrower role the same way (a provider seam, a home for structural tests, a read-side composing module). Run the grep below rather than maintaining a second list here; a one-off that grows a second instance is the signal to write an archetype doc for it.

## Encoding mechanism

Architectural rules are encoded as a layered set of `AGENTS.md` files that Claude Code auto-loads when working in a relevant subtree:

- **Root `AGENTS.md`** — points at this directory.
- **Per-directory `src/internal/<dir>/AGENTS.md`** — declares the directory's archetype (or, where it has none, its own role and rules) plus any module-specific notes.
- **Archetype docs in `archetypes/`** — full rules for each category.

## Listing archetypes at a glance

To see which archetype every directory is classified as:

```bash
grep -h "^# " src/internal/*/AGENTS.md
```

(There is no separate module registry — that would duplicate what each `AGENTS.md` already declares and would drift the same way a hand-maintained wiki does.)

## Known gaps

Current architectural violations are tracked in [known-gaps.md](known-gaps.md). The archetype docs describe the target; that file records where reality diverges and what closing each gap would require.
