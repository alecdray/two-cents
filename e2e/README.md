# E2E Tests

End-to-end tests drive the real application through a browser using [Playwright](https://playwright.dev). They verify full-stack behaviour from a user's perspective — server-rendered HTML plus HTMX — and are the primary way the app is verified end to end (see [`docs/testing.md`](../docs/testing.md)).

## Structure

```
e2e/
├── feat/       # Gherkin-style feature files — what is tested and why, in plain English
├── helpers/    # Reusable logic shared across specs (DB seeding, etc.)
└── spec/       # Playwright test files — how each scenario is implemented
```

Every feature file in `feat/` has a corresponding spec file in `spec/` with the same base name. Scenarios in the feature map 1:1 to `test()` cases in the spec, matched by name.

## Running

The app must already be running — the Playwright config has **no `webServer` block**, so the suite never starts the server for you. Start it in a separate terminal, then run the suite:

```bash
# Terminal 1 — start the app (with the templ/tailwind watchers)
task dev          # or `task run` for a one-shot build-and-serve

# Terminal 2 — run the suite
task test/e2e

# Run a specific spec file
task test/e2e -- e2e/spec/accounts-overview.spec.ts
```

Playwright targets `http://127.0.0.1:${PORT}`, where `PORT` is read from `.env` (default `4690`). The seeding helpers write to the SQLite file at `GOOSE_DBSTRING` / `DB_PATH` (default `./tmp/db.sql`) — the same file the running app reads from.

### Authentication

The app gates every page behind the single local login ([ADR-0007](../docs/adr/0007-single-local-login.md)). You don't set this up by hand: `globalSetup` (`helpers/global-setup.ts`) runs once before the suite, seeding the login password through `bin/setpassword` and logging in via the real form, then persisting the session to `e2e/.auth/state.json`. Every spec inherits it through the config's `use.storageState`, so tests start authenticated. The only spec that signs in by hand is `login.spec.ts`, which opts out of the shared session to exercise the gate itself.

### Watch / debug modes

`task test/e2e -- <flag>` forwards flags to `playwright test`:

```bash
task test/e2e -- --ui       # interactive GUI, live re-run on change — best for development
task test/e2e -- --headed   # visible browser, no pause — quick visual sanity check
task test/e2e -- --debug    # Playwright Inspector, step through actions — best for a failing test
```

### Cold start in a fresh worktree

A new worktree has no `.env`, no `node_modules`, and no browser binary. Run these once before the first `task test/e2e`:

```bash
cp /path/to/two-cents/.env .env   # worktrees don't have .env
npm install                                               # if node_modules is missing
npx playwright install chromium                           # browser binary
task build                                                # regenerate _templ.go etc. — see pitfalls
```

The app **panics on boot** without `PLAID_CLIENT_ID`, `PLAID_SECRET`, and a valid 64-char-hex `ENCRYPTION_KEY` (config requires them). For any non-Plaid page — the overview included — dummy values plus a real 64-hex `ENCRYPTION_KEY` are enough; only live-bank flows need the real Sandbox creds (which live in the main repo's gitignored `.env`).

## Writing a new test

Follow these in order. The last step is non-negotiable: **run the suite** before considering the test done.

1. **Decide the user-facing behaviour.** One feature file per surface (page or top-level fragment). A behaviour spanning two surfaces is two features.
2. **Write the feature file first** (`e2e/feat/<name>.feature`). Plain English, BDD style, from the user's point of view — no CSS classes, IDs, or implementation detail:
   ```gherkin
   Feature: <Name>

     <One-sentence description of the area being tested.>

     Scenario: <What the user does and expects>
       Given <starting state>
       When <action>
       Then <expected outcome>
   ```
3. **Create the matching spec** (`e2e/spec/<name>.spec.ts`, same base name). Add a header comment linking back to the feature, and name each `test()` to match its `Scenario:` **exactly** — the mapping is name-based 1:1:
   ```ts
   import { test, expect } from '@playwright/test';

   // Scenarios from e2e/feat/<name>.feature

   test('<Scenario name verbatim>', async ({ page }) => {
     // ...
   });
   ```
4. **Set up state by seeding the database.** Auth is already handled by global setup (see [Authentication](#authentication)); for the *data* a scenario needs there is no public write API — seed the SQLite file directly through a helper (see [Helpers](#helpers)). Each test owns its starting state; never depend on what another test left behind.
5. **Locate elements with `data-testid` only.** Use `page.getByTestId('...')`. The narrow exceptions are `getByRole(...)` / `getByLabel(...)` for semantic assertions on standard form controls, and `page.locator('dialog[open]')` for scoping inside an open modal. Never select on CSS classes, raw text, or structure — they change for non-test reasons.
6. **If a testid doesn't exist yet**, add `data-testid="..."` to the relevant `.templ` following [`docs/design/testids.md`](../docs/design/testids.md), then `task build/templ`.
7. **Wait on observable DOM signals, never on time.** HTMX swaps complete when the new DOM is present — assert on that. `page.waitForTimeout(...)` is banned: it produces flaky tests and hides races. For a specific response, use `page.waitForResponse(...)`.
8. **Run the suite.** `task test/e2e` (with the app running) must pass. The suite-wide rules in [Conventions](#conventions) aren't automated — verify them yourself, especially that every `getByTestId` you add resolves to a real declaration in `src/internal/`.

### Common pitfalls

- **A passing spec with an undeclared testid is silently wrong.** If `getByTestId('foo')` matches nothing, `expect(...).toHaveCount(0)` / `.not.toBeVisible()` passes vacuously. Grep `src/internal/` for the testid after adding a new `getByTestId` to confirm it exists.
- **Stale `_templ.go` after a branch switch.** `_templ.go` files are gitignored, so checking out a branch that added or renamed testids in `.templ` does **not** bring the generated Go with it — the source shows the new testid but the running server doesn't render it, and the spec times out. Run `task build/templ` (or `task build`) after any branch switch and before starting the app. The cold-start checklist bakes this in.
- **SQLite `CURRENT_TIMESTAMP` is second-resolution.** Two rows inserted in the same second tie on `ORDER BY created_at`. Tests that depend on insertion order should accept either ordering or insert a deliberate gap.

## Discovering existing testids

Before inventing a new `data-testid`, check what's already declared — there's often one to reuse.

```bash
# Every declared testid, sorted and deduplicated
grep -rhoE 'data-testid="[^"]+"' src/internal/ | sort -u

# Find the templ that declares a given testid
grep -rn 'data-testid="accounts-overview-net-cash"' src/internal/
```

The grep-everything approach is the source of truth: a testid's prefix doesn't always match its file's own component name (a fragment scoped to one parent uses the parent's prefix — see [`docs/design/testids.md`](../docs/design/testids.md)), so always grep.

## Helpers

Shared logic lives in `helpers/`. Import from there rather than duplicating setup across specs; keep each helper focused on one concern.

State is set up by **seeding SQLite directly**. The app only writes accounts via a live Plaid enrollment — which the suite deliberately never touches — so the helpers shell out to `sqlite3` against the same database file the app reads (`GOOSE_DBSTRING`, default `./tmp/db.sql`). This keeps the helpers dependency-free and honours the suite's *real backend, no mocks* rule.

`helpers/db.ts` is the current example: `seedOverview(accounts)` resets the DB then inserts connections and accounts in the exact shapes a scenario needs (mixed kinds, an unknown balance, a needs-reconnect connection); `resetAccounts()` wipes accounts and connections for the empty-state scenario. Add new seeding helpers as named exports here.

`helpers/auth.ts` holds the shared login constants (the test password global setup seeds, and the `storageState` path) used by the global setup and the login spec.

## Conventions

BDD style (scenarios describe behaviour, not implementation) is covered in [`docs/testing.md`](../docs/testing.md). The rules below are the suite-wide invariants — honour them when adding or editing specs. None are automated; the only gate is `task test/e2e` passing.

### Feature ↔ spec 1:1

Every `e2e/feat/<name>.feature` has a matching `e2e/spec/<name>.spec.ts`, and every `Scenario:` has a `test()` with the **exact same name** in the paired spec — no orphans in either direction. The feature file is the source of truth: when behaviour changes, edit the feature first, then the spec. Renaming a scenario means editing both files in lockstep.

### No orphan testids

Every `getByTestId('foo')` must resolve to a `data-testid="foo"` declared by some templ under `src/internal/`. An orphan testid silently makes negative assertions pass for the wrong reason. After adding a `getByTestId`, grep `src/internal/` to confirm it exists; if not, add it to the templ and run `task build/templ`.

### Selectors are `data-testid` only

Allowed locator factories: `getByTestId`, `getByRole`, `getByLabel` — the latter two reserved for semantic assertions on standard form controls. Do not use `getByText`, `getByPlaceholder`, `getByAltText`, or `getByTitle`, even for content assertions: to assert text is visible, add a testid to its container and assert text on that. `page.locator(...)` is allowed only for `'dialog[open]'` (modal scoping) and `[data-testid="..."]` attribute selectors. No CSS classes, `nth-of-type`, or XPath anywhere.

### Wait on observable DOM signals, never on time

No `page.waitForTimeout(...)`, `sleep(N)`, `delay(N)`, or `setTimeout(...)` in specs. HTMX swaps complete when the new DOM is present — assert on that with `expect(page.getByTestId(...)).toBeVisible()` or equivalent. For a specific network response, use `page.waitForResponse(...)`.

### Real backend, seed the database

Tests run against the real Go server and SQLite database — no mocking, no request interception, no test doubles. Specifically forbidden: `page.route(...)`, `page.unroute(...)`, `page.fulfill(...)`, MSW/nock/sinon, `jest.mock|fn|spyOn`, `vi.mock|fn|spyOn`. If a test needs a specific data shape, insert the fixture into the database directly via a helper. Exercising an external API call belongs in a Go unit test against the adapter, not in e2e.

### Fail loud on missing required data

When a test depends on an env var or fixture, guard with `expect(value, '<msg>').toBeTruthy()`, not `test.skip()`. Skipping silently hides a misconfigured environment; a failure surfaces it immediately. (This is the opposite of the unit-test `t.Skip` rule — the contexts differ; see [`docs/testing.md`](../docs/testing.md).)

## Logs

`task dev` tees each watcher's output to a file in `tmp/` alongside the console:

| File | Source |
|---|---|
| `tmp/dev-server.log` | Go server — HTTP requests, app errors, slog output |
| `tmp/dev-templ.log` | Templ compiler — template build errors |
| `tmp/dev-tailwind.log` | Tailwind — CSS build errors |

Logs are overwritten on each `task dev` restart. When debugging a failing spec, check `tmp/dev-server.log` first for server-side errors the browser wouldn't surface.

## Maintenance

- When a feature changes, update the feature file and spec together.
- If a scenario is no longer valid, remove it from both files.
- Keep feature files free of implementation detail — they should read like plain English.
