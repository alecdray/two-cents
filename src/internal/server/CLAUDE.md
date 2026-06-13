# server — composition root (singleton)

The **composition root**. Exactly one; no archetype.

## Responsibilities

- Build all services in `NewServices(app, db)` (manual DI).
- Set up the root `*httpx.Mux` and any sub-muxes (mounted via `rootMux.Use(prefix, subMux)`).
- Register cron tasks with the `core/task` task manager.
- Call each domain module's `adapters.RegisterRoutes(mux, handler)` — one call per module.
- Run lifecycle: open DB, start task manager, start HTTP listener, handle shutdown.

## Rules

- **No domain logic.** Server wires things together; it does not implement features.
- **No URL patterns** beyond mounting sub-muxes. Concrete paths live in each module's `adapters/routes.go`.
- **Allowed imports:** every domain module, every external client, all of `core/*`. Server is the only place this is allowed.
- **No tests** — the app is integration-tested via e2e.
