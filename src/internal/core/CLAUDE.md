# core — shared infrastructure (singleton)

The **shared infrastructure** of the application. Exactly one of it; no archetype (an archetype describes a category, `core/` is unique).

## Sub-packages

Each is a focused, framework-level utility used by 2+ modules:

- `app` — application config (and, later, the single local-login auth)
- `contextx` — `ContextX` wrapper over `context.Context`; carries app config, request id, user id
- `db` — SQLite connection, goose migrations, `WithTx` (sqlc query code wires in with the first domain queries)
- `httpx` — custom mux, middleware, error handling
- `task` — background task scheduling (robfig/cron); defines the `Task` interface
- `templates` — shared Templ primitives (root layout, page layout)

## Rules for adding to core

- **Used by 2+ modules.** Single-consumer code stays in the consumer.
- **Framework-level, not domain.** No business concepts here. If it mentions accounts, transactions, budgets, categories — it doesn't belong in core.
- **`x` suffix** marks extension packages over a stdlib counterpart (`contextx`; future `timex`, `sqlx`).

Domain modules, external clients, and `server` may all import `core/*`.
