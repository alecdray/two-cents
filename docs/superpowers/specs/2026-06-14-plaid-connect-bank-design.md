# Design — Connect & manage bank connections (Plaid Link)

Date: 2026-06-14
Project: `two-cents` (Go + templ + htmx + SQLite)
Status: approved design, feeds `/build`

## Summary

Add the user-facing flow to **connect a bank via Plaid Link**, plus the adjacent
connection-management actions: surface a connect failure inline, disconnect a
bank, and reconnect (re-auth) one flagged `needs_reconnect`. The accounts
overview already renders connections/accounts and `accounts.RegisterConnection`
already fetches+persists accounts on connect — the work is the Plaid Link wiring,
the new provider seam methods, the HTTP/templ surface, and a deterministic
test seam.

This is a **connection-management feature**, not a single tracer bullet. It is
sequenced into four independently-shippable phases; Phase 1 (connect → accounts
appear) is the spine and carries the shared infrastructure the rest reuse.

## Goals

- A user with no accounts can connect a real bank through Plaid Link and see the
  resulting accounts on the overview (`/`).
- A failed connect renders an inline, recoverable error in place.
- A user can disconnect a bank (its accounts disappear) and reconnect one whose
  login expired (the `needs_reconnect` badge clears).
- The whole flow up to and after Plaid's hosted modal is covered by the
  deterministic e2e suite; the wire calls are covered by Go tests; the real modal
  is validated manually in sandbox.

## Non-goals

- Transactions sync / categorization (separate slice; the sync engine exists).
- Account-kind editing / `SetAccountKind` UI (separate slice).
- Single local auth (separate slice).
- Driving the real Plaid Link modal in automated e2e (see Testing).

## Conventions this design conforms to

- **ADR-0002** (bank-provider abstraction): all bank access goes through the
  `banking.BankProvider` seam returning domain-agnostic shapes; the `plaid`
  external-client is the only code that talks to the bank network; `client.go`
  exchanges the Plaid Link `public_token` for a per-Item `access_token`.
- **external-client archetype** (`docs/architecture/archetypes/external-client.md`):
  leaf module, `client.go` + `entities.go` + `service.go`, no persistence, no
  `repo.go`, `CLAUDE.md` required.
- **domain-module archetype**: connect handlers/routes/views live in
  `accounts/adapters/`, call `accounts.Service`, and **never import `plaid`**
  (enforced by `architecture/isolation_test.go`).
- **`docs/design/principles.md`**: server renders HTML over htmx; JS only for
  client-only ephemeral state (Alpine); errors render inline in place via
  `httpx.HandleErrorResponse`; fragments over pages.
- **`docs/design/oob-swaps.md`**: a swap region is defined in exactly one templ.
- **`e2e/README.md` / `docs/testing.md`**: real Go server + seeded SQLite, no
  browser mocking; external API calls exercised in Go tests; fake
  `BankProvider` is the canonical no-network stand-in (`banking_test.go`).
- **`docs/design/testids.md`**: testid = kebab of the templ function name.

## Architecture

### Seam extension (`banking`)

Add three methods + small value types to the provider seam (all shapes
provider-agnostic, per ADR-0002):

```go
type LinkToken   struct { Token string; Mode string }   // Mode: "real" | "fake"
type Item        struct { AccessToken, ProviderItemID string }
type LinkOptions struct { AccessToken string }           // empty = connect; set = reconnect (update mode)

// added to BankProvider:
CreateLinkToken(ctx contextx.ContextX, opts LinkOptions) (LinkToken, error)
ExchangePublicToken(ctx contextx.ContextX, publicToken string) (Item, error)
RemoveItem(ctx contextx.ContextX, accessToken string) error
```

The provider stamps `Mode` (`plaid` → `"real"`, `fakebank` → `"fake"`), so
handlers and JS stay provider-agnostic.

### Two provider implementations

- **`plaid.Service`** gains the three methods, backed by new `client.go` calls:
  `/link/token/create`, `/item/public_token/exchange`, `/item/remove`. Plaid wire
  shapes + conversions live in `entities.go`.
- **`fakebank`** — new external-client-archetype module
  (`src/internal/fakebank`) implementing the *whole* `BankProvider`
  deterministically: a fake link token (`Mode:"fake"`), a canned `Item` on
  exchange, a fixed set of accounts on `ListAccounts`, no-op `RemoveItem`. No
  network, no persistence. `CLAUDE.md` declares the archetype.

### Config-selected provider (new pattern → ADR-0004)

`server/services.go` selects the provider by a new env knob:
`BANK_PROVIDER` (`GetEnvWithDefault("BANK_PROVIDER", "plaid")`). `plaid` is the
default; `fake` wires `fakebank`. **ADR-0004** records this decision and frames
it against the e2e philosophy: the fake is server-side Go (the documented
unit-test fake promoted to a wiring option) — the real server, SQLite, HTTP, and
templ all run; nothing is mocked in the browser. This is the same spirit as
"seed the database directly."

### Accounts service orchestration

New methods on `accounts.Service` wrapping the existing
`RegisterConnection`/`SyncAccounts`/repo:

- `BeginConnect(ctx) (banking.LinkToken, error)` → `provider.CreateLinkToken({})`
- `CompleteConnect(ctx, publicToken) (Connection, error)` →
  `provider.ExchangePublicToken` → `RegisterConnection`
- `BeginReconnect(ctx, connID) (banking.LinkToken, error)` → decrypt token →
  `provider.CreateLinkToken({AccessToken})` (update mode)
- `CompleteReconnect(ctx, connID) error` → set state `active` → `SyncAccounts`
- `Disconnect(ctx, connID) error` → `provider.RemoveItem(decrypted)` → repo
  delete accounts + connection

Repo gains `DeleteAccountsByConnection` + `DeleteConnection` (new sqlc queries;
**no schema change** — connect/reconnect use existing tables, disconnect is
deletes).

## HTTP routes & frontend

### Routes (`accounts/adapters/routes.go`)

| Method + path | Handler | Returns |
|---|---|---|
| `GET /accounts/connections/link-token` | `BeginConnect` | link token (real mode; widget plumbing for the Alpine interceptor) |
| `POST /accounts/connections` | `CompleteConnect` (body: `public_token`) | re-rendered overview fragment |
| `GET /accounts/connections/{id}/relink-token` | `BeginReconnect` | update-mode link token |
| `POST /accounts/connections/{id}/reconnect` | `CompleteReconnect` | overview fragment |
| `DELETE /accounts/connections/{id}` | `Disconnect` | overview fragment |

### templ

- **Fragment refactor:** extract the overview body into one shared
  `overviewContent` fragment rendered by both `AccountsOverviewPage` (initial)
  and every post-action handler (swap). One swap region, per `oob-swaps.md`.
- **Connect control (one markup, both modes):** a single
  `hx-post="/accounts/connections"` form rendered as the empty-state CTA and a
  persistent "Add account" affordance, carrying server-rendered `data-bank-mode`.
  - *fake mode:* no interceptor — posts directly (sentinel `public_token`).
  - *real mode:* a thin Alpine `x-data` interceptor on submit:
    `GET link-token` → `Plaid.create({token, onSuccess}).open()` → on success set
    the hidden `public_token` and fire the same htmx post. Plaid Link SDK
    `<script>` loaded only in real mode.
  Same testid/DOM both modes; only the irreducible modal step differs.
- **Per-connection controls:** a disconnect control on account rows
  (`hx-delete`); a reconnect control beside the existing `needs_reconnect` badge
  (same real/fake interceptor pattern, against the relink-token + reconnect
  routes).

### Errors inline

`CompleteConnect` failure returns an error component scoped to the connect
region via `httpx.HandleErrorResponse(...).SetComponent(...)` — no redirect,
no next-page banner.

### testids (kebab of templ func, overview family)

`accounts-overview-connect`, `accounts-overview-connect-error`,
`accounts-overview-account-disconnect`, `accounts-overview-account-reconnect`.

## Phasing

Each phase is independently shippable; shared infra lands in Phase 1.

1. **Connect → accounts appear (spine).** Seam `CreateLinkToken` /
   `ExchangePublicToken` + types; `plaid` `link/token/create` +
   `public_token/exchange`; `fakebank` module + `BANK_PROVIDER` wiring +
   ADR-0004; `accounts.Service.BeginConnect`/`CompleteConnect`; link-token +
   connections routes; `connect-bank` form + Alpine interceptor + real-mode SDK;
   `overviewContent` fragment refactor; update `accounts/README.md` + `CLAUDE.md`.
2. **Connect-failure error UI.** Inline error component + `CompleteConnect`
   returns it via `httpx.HandleErrorResponse` on provider failure.
3. **Disconnect.** Seam `RemoveItem`; `plaid` `/item/remove`; fakebank no-op;
   `accounts.Service.Disconnect` + repo deletes (new sqlc queries); `DELETE`
   route + disconnect control.
4. **Reconnect (update mode).** `CreateLinkToken` honors
   `LinkOptions{AccessToken}`; `plaid` passes `access_token`;
   `accounts.Service.BeginReconnect`/`CompleteReconnect` (state `active` +
   `SyncAccounts`); relink-token + reconnect routes; reconnect control beside the
   badge.

## Testing

| Layer | Covers | Notes |
|---|---|---|
| **e2e (fake mode)** | connect→appear, disconnect, reconnect — happy paths | `task dev` / `task test/e2e` run with `BANK_PROVIDER=fake` + dummy `PLAID_*` + `ENCRYPTION_KEY`; reconnect seeds a `needs_reconnect` connection via existing SQLite helpers; 1:1 `feat/`↔`spec/` per file |
| **Go service tests** | `Begin/Complete*`, `Disconnect` orchestration | inject the hand-written fake `BankProvider` (`banking_test.go` pattern) |
| **Go adapter test** | real `plaid` wire calls (`link/token/create`, `public_token/exchange`, `item/remove`) | can target Plaid sandbox; skippable without creds |
| **Go handler test** | connect-failure renders the inline error component | provider returns error → assert error templ in recorder |
| **Manual** | the real Plaid Link modal in sandbox | validated by hand |

**Harness change:** the e2e environment runs the app with `BANK_PROVIDER=fake`.
The existing overview spec is unaffected (overview rendering never calls the
provider).

## Risks / notes

- **Plaid Link is irreducibly external.** The fake covers everything we own; the
  real modal is Plaid's hosted UI and is validated manually. This split is the
  documented e2e philosophy, not a compromise.
- **`link_token` freshness:** minted on connect-button action (not on every page
  render) to avoid a Plaid call + latency on the home page.
- **Update-mode reconnect** does not exchange a new `public_token`; the existing
  `access_token` is reused, so `CompleteReconnect` only flips state + syncs.

## Open questions

None — all design decisions resolved during brainstorming (e2e via fake provider;
inline connect on `/`; all four capabilities in scope; manual modal validation).
