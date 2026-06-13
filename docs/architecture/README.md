# Two Cents Architecture

This directory documents the architectural rules for `src/internal/`. Most directories under `src/internal/` are classified into one of three archetypes; two directories are singletons documented in their own `CLAUDE.md`.

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

## Singletons

Two directories are exactly-one-of-them. Their rules live next to the code:

- **`server/`** — composition root. Builds services, sets up middleware and sub-muxes, calls each domain module's `RegisterRoutes`, runs lifecycle (including registering the sync task on the cron). See [`src/internal/server/CLAUDE.md`](../../src/internal/server/CLAUDE.md).
- **`core/`** — shared infrastructure. Framework-level sub-packages used by 2+ modules. See [`src/internal/core/CLAUDE.md`](../../src/internal/core/CLAUDE.md).

A singleton is *not* an archetype: archetypes describe categories with multiple instances. Trying to fit `server` into `utility` (or any other archetype) would require carving out exceptions to that archetype's import rules.

## Encoding mechanism

Architectural rules are encoded as a layered set of `CLAUDE.md` files that Claude Code auto-loads when working in a relevant subtree:

- **Root `.claude/CLAUDE.md`** — points at this directory.
- **Per-directory `src/internal/<dir>/CLAUDE.md`** — declares the directory's archetype (or, for singletons, documents rules directly) plus any module-specific notes.
- **Archetype docs in `archetypes/`** — full rules for each category.

## Listing archetypes at a glance

To see which archetype every directory is classified as:

```bash
grep -h "^# " src/internal/*/CLAUDE.md
```

(There is no separate module registry — that would duplicate what each `CLAUDE.md` already declares and would drift the same way a hand-maintained wiki does.)

## Known gaps

Current architectural violations are tracked in [known-gaps.md](known-gaps.md). The archetype docs describe the target; that file records where reality diverges and what closing each gap would require.
