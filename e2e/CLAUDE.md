# E2E tests

Playwright-driven end-to-end tests. Read [`README.md`](./README.md) before writing or modifying tests — it covers structure, the run recipe, the cold-start checklist, seeding helpers, and debugging.

## Suite rules

Invariants the suite holds itself to. Honour them when adding or editing specs:

- **Feature ↔ spec 1:1.** Every `e2e/feat/<name>.feature` has a matching `e2e/spec/<name>.spec.ts`, and every `Scenario:` has a `test()` with the exact same name in the paired spec. The feature file is the source of truth — edit it first.
- **No orphan testids.** Every `getByTestId('foo')` must resolve to a `data-testid="foo"` declared under `src/internal/`. An orphan testid silently makes negative assertions pass for the wrong reason. After adding one, grep `src/internal/` to confirm it exists.
- **Testid naming** follows [`docs/design/testids.md`](../docs/design/testids.md).
- **Selectors are `data-testid` only.** Allowed factories: `getByTestId`, `getByRole`, `getByLabel` (the latter two only for semantic assertions on standard form controls). No `getByText` / `getByPlaceholder` / `getByAltText` / `getByTitle`, even for content — add a testid first. `page.locator(...)` only for `'dialog[open]'` and `[data-testid="..."]` selectors.
- **Wait on observable DOM signals, never on time.** No `page.waitForTimeout(...)`, `sleep(N)`, `delay(N)`, or `setTimeout(...)`. HTMX swaps complete when the new DOM is present — assert on that.
- **Real backend, seed the database.** No `page.route(...)` / `page.unroute(...)` / `page.fulfill(...)`, no MSW/nock/sinon, no `jest.mock` / `vi.mock`. Need a specific data shape? Insert it into the SQLite DB directly via a `helpers/` seeder. There is no auth flow and no write API to drive — seeding is the only setup path.
- **Fail loud on missing required data.** Guard fixtures/env with `expect(value, '<msg>').toBeTruthy()`, not `test.skip()`.
- **Self-contained tests.** Each test sets up its own starting state (seed + `page.goto`); never depend on another test's leftovers.

## Gate

The app must be running separately (`task dev` or `task run` on `:4690` — the Playwright config has no `webServer`). Then `task test/e2e` must pass before considering a test or change done.
