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
- **Tests are wiring-level only.** The composition root hosts the tests that only hold once modules are composed: construction smokes (the right services are built over the right seams) and assembled cross-module invariants that drive more than one module's HTTP handlers together (the one place a peer's `adapters/` may be imported). Per-module behaviour is tested inside each module; full-stack flows are covered by e2e. No domain logic means no domain-logic tests here.
