# auth

The application's gate. It owns the single local credential and issues the
session every other page requires. It is a **supporting module**, not part of
the financial domain model — its decision and rationale live in
[ADR-0007](../../../docs/adr/0007-single-local-login.md); the domain map only
points here ([domain README](../../../docs/domain/README.md)).

The reusable session machinery — minting and validating the signed token, the
cookie, and the middleware that guards protected routes — lives in `core`
(`core/app` + `core/httpx`), used by the composition root. This module owns the
**login flow** and the **credential store**.

## Pages

- `GET /login` — the login screen, a chromeless page (no navbar). Renders a
  "no account configured" state when no credential has been seeded.
- `POST /login` — verifies the submitted password against the stored hash; on
  success issues the session cookie and redirects to `/`, on failure re-renders
  the form with an inline error.
- `GET /logout` — clears the session cookie and returns to `/login`.

Unauthenticated requests to any protected route redirect to `/login` (HTMX
requests via `HX-Redirect`); there is no separate unauthorized page.

## Credential

One password-only credential, stored hashed in this module's own table — a
stable id plus a `username` retained for identity and display, though login does
not challenge for it. The single-user rule is a design constraint, not
schema-enforced.

Bootstrapping and rotation happen out-of-band through a command the operator
runs (`task auth/set-password`), which upserts the credential — the same path
the e2e suite uses to establish its shared test login. There is no in-app
password management in v1.
