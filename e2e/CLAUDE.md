# E2E tests

Playwright-driven end-to-end tests. Read [`README.md`](./README.md) before writing or modifying tests — it covers structure, the run recipe, the cold-start checklist, seeding helpers, and debugging.

## Suite rules

Invariants the suite holds itself to. Honour them when adding or editing specs:

- **Feature ↔ spec 1:1.** Every `e2e/feat/<name>.feature` has a matching `e2e/spec/<name>.spec.ts`, and every `Scenario:` has a `test()` with the exact same name in the paired spec. The feature file is the source of truth — edit it first.
- **No orphan testids.** Every `getByTestId('foo')` must resolve to a `data-testid="foo"` declared under `src/internal/`. An orphan testid silently makes negative assertions pass for the wrong reason. After adding one, grep `src/internal/` to confirm it exists.
- **Testid naming** follows [`docs/design/testids.md`](../docs/design/testids.md).
- **Selectors are `data-testid` only.** Allowed factories: `getByTestId`, `getByRole`, `getByLabel` (the latter two only for semantic assertions on standard form controls). No `getByText` / `getByPlaceholder` / `getByAltText` / `getByTitle`, even for content — add a testid first. `page.locator(...)` only for `'dialog[open]'` and `[data-testid="..."]` selectors.
- **Wait on observable DOM signals, never on time.** No `page.waitForTimeout(...)`, `sleep(N)`, `delay(N)`, or `setTimeout(...)`. HTMX swaps complete when the new DOM is present — assert on that.
- **Real backend, seed the database.** No faked responses or data shapes — `page.fulfill(...)`, MSW/nock/sinon, `jest.mock` / `vi.mock` are forbidden; seed the SQLite DB via a `helpers/` seeder (the only data-setup path — there is no write API to drive). **Narrow exception:** `page.route(...)` may shape *timing/transport only* (delay via `continue()`, or `abort()`) of an otherwise-real request, solely to make a transient client-only UI state observable when it has no DB representation — never to fabricate a response. See [`README.md`](./README.md) § Real backend.
- **Auth happens once, in global setup.** The app gates every page behind the single local login (ADR-0007). `helpers/global-setup.ts` seeds the password through `bin/setpassword` and logs in once, persisting the session to `storageState`; every spec inherits it via the config's `use.storageState`, so tests start authenticated and never repeat the login. The login spec — the one that exercises the gate itself — opts out with `test.use({ storageState: { cookies: [], origins: [] } })`.
- **Fail loud on missing required data.** Guard fixtures/env with `expect(value, '<msg>').toBeTruthy()`, not `test.skip()`.
- **Self-contained tests.** Each test sets up its own starting state (seed + `page.goto`); never depend on another test's leftovers.

## Gate

The app must be running separately (`task dev` or `task run` on `:4690` — the Playwright config has no `webServer`). Then `task test/e2e` must pass before considering a test or change done.
