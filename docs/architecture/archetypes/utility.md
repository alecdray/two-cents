# Utility Module

## Purpose

A utility module provides stateless, domain-shaped helpers: pure functions and/or embedded data. It has no `Service` struct, no database access, and no mutable state. The current-month calculators are the canonical examples — `tracker` (remaining budget, pace targets, income/savings progress) and `reporting` (month wrap totals, spend-by-Category aggregation) — each a pure consumer of data fetched through other modules' services.

## File layout

```
src/internal/<name>/
├── <name>.go       # exported types and functions — required
├── *_test.go       # tests live next to the file under test
├── data.json       # optional embedded data (loaded via //go:embed)
└── CLAUDE.md       # required; declares archetype
```

No `service.go`. No `repo.go`. No `adapters/`. README is optional — a package doc comment in `<name>.go` is sufficient.

## Allowed imports

- `core/*` sub-packages.
- Stdlib.
- Small, focused third-party libraries.

## Forbidden imports

- Any domain module. Utility modules are leaves in the dependency graph — they produce, they do not consume.
- Any external client module. Utility modules must not pull in network-facing code.
- `core/db/sqlc` — utility modules own no tables and must not touch the database.

## When to use this archetype vs. core

Use **utility** when the code is domain-shaped but stateless: it encodes knowledge about the application's domain (e.g. pace-target math, month-wrap aggregation, a default category taxonomy) yet needs no persistence or lifecycle management. Place it in `src/internal/<name>/`.

Use **core** instead when the code is framework-level (HTTP primitives, database handles, context extensions, time/crypto helpers) and carries no domain concepts. Core sub-packages are allowed to carry state and lifecycle responsibility; utility modules are not.

A practical rule: if the package name would mean something to a domain user (e.g. someone tracking their own spending in Two Cents), it belongs in `src/internal/` as a utility. If the package name is structural plumbing, it belongs in `core/`.

## Where new code goes

| Change | File |
|---|---|
| New exported type or pure function | `<name>.go` (or a new topic file at the module root) |
| New or updated embedded dataset | `data.json` (or similarly named file), loaded via `//go:embed` |
| Tests for any of the above | `*_test.go` next to the file under test |
