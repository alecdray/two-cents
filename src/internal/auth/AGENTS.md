# auth — domain module

Rules: ../../../docs/architecture/archetypes/domain-module.md

Module-specific notes:
- A **supporting** module, not a financial domain ([ADR-0007](../../../docs/adr/0007-single-local-login.md)); the domain map only points here. Owns the single `users` credential row and reaches no other module.
- `repo.go` is the only file that touches `core/db/sqlc`. The bcrypt hash never leaves the package — the `credential` type's `hash` field is unexported and never returned to a caller.
- Login is **password-only** against the single seeded credential (a fixed row id, mirroring budget's single-config convention); the `username` column is identity/display only, never challenged. `Authenticate` returns the same inline message for "no credential" and "wrong password" so a probe can't tell them apart.
- The session token, cookie, and the route-guard middleware live in `core/app` + `core/httpx` (the composition root mounts every non-public route behind `httpx.JwtMiddleware`). This module owns only the login flow (`GET`/`POST /login`, `GET /logout`) and the credential store.
- Bootstrapping and rotation are out-of-band via `cmd/setpassword` (`task auth/set-password`, reads `AUTH_PASSWORD`) — no in-app password management. With no credential seeded the login page shows a "not configured" prompt.
- e2e seeds its login through that same `setpassword` binary in global setup, then logs in once and shares the session via storageState (see `e2e/README.md`).
