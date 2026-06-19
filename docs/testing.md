# Testing

## Approach

Testing in Two Cents is **agent-driven** — AI agents participate in the test authoring and execution cycle, not just humans. Tests should be machine-readable and machine-runnable, with intent expressed through their structure.

## Unit vs E2E

**Unit tests** target the service layer and the pure utility modules — business logic isolated from HTTP and the database, with dependencies injected so they can be substituted. They're cheap, fast, and the right place for any non-trivial logic that can be exercised without crossing a boundary. The `utility` modules (`tracker`, `reporting`) are the priority here: pure, no DB, highest logic density (pace-target math, remaining budget, wrap totals, settling/partial states). Domain-module logic (`categorization` precedence, `budget` residual math, `accounts` aggregation) is unit-tested with a small locally-declared repo interface; the `transactions` sync task is tested against a **fake `BankProvider`** — no network.

**E2E tests** drive the real application through a browser using Playwright. Because Two Cents renders server-side HTML and uses HTMX for interactivity, e2e is the primary way to verify the full stack works together from a user's perspective. When in doubt: if a behaviour crosses the templ boundary, it belongs in e2e.

## BDD style

E2E scenarios are expressed in terms of user-observable behaviour rather than implementation details. Feature files in `e2e/feat/` read as plain English; the spec in `e2e/spec/` is the mechanical realisation. This keeps scenarios readable by non-engineers and keeps intent legible to agents generating or reviewing them.

## The e2e gate

`task test/e2e` (with `task dev` in another terminal) must pass before considering a test or change done. There are no static checks — the suite-wide rules (feature ↔ spec pairing, no orphan testids, selector discipline, no fixed-timeout waits, single auth path, real backend) are documented in [`e2e/README.md`](../e2e/README.md) and [`e2e/CLAUDE.md`](../e2e/CLAUDE.md) and honored by hand.

## Manual Plaid sandbox verification

The automated e2e suite runs against the deterministic `fake` provider (`BANK_PROVIDER=fake`,
[ADR-0006](adr/0006-bank-provider-selected-by-config.md)), so it never touches Plaid. The connection
flows that go through the **real hosted Plaid Link modal** can't be asserted deterministically and are
verified by hand in **sandbox** (`PLAID_ENV=sandbox` + sandbox `PLAID_CLIENT_ID`/`PLAID_SECRET` in
`.env`). This split is deliberate, not a coverage gap: the fake provider exercises everything we own, and
Plaid's hosted modal is left to manual sandbox checks rather than driven with brittle UI automation. Run
the app (`task build && ./bin/app`, default port **4690**), open `/`, and connect via
Plaid Link with these sandbox test credentials:

| Field | Value |
|---|---|
| Institution | any (e.g. First Platypus Bank) |
| Username | `user_good` |
| Password | `pass_good` |
| Phone (OTP step) | `+1 415 555 0011` |
| OTP code | `123456` |

`user_good` / `pass_good` returns a healthy multi-account Item. Other Plaid sandbox test users
(e.g. `user_custom`) drive other scenarios — see Plaid's sandbox documentation.

## Unit test conventions

Go tests use the standard library. Conventions:

- **Always write tests for new logic.** Any non-trivial function should have a corresponding test.
- **Extract pure logic from `cmd` packages** into a separate file so it can be unit tested without I/O — keep `main.go` as thin orchestration only.
- **Assert external behavior, not implementation.** Given inputs (transactions, balances, budgets, rules), assert the computed outputs (classification + category, remaining amounts, pace targets, wrap totals). No assertions on call order or private structure.
- **Group tests by the function under test** using a top-level `Test<FuncName>` function. Use `t.Run` subtests to describe specific behaviours — output is scannable and the subtest names double as documentation.
- **Name subtests as plain descriptions** of the expected behaviour (e.g. `"clamps pace to zero when over budget"`).
- **Test behaviour, not implementation.** Each subtest asserts one specific outcome.
- **`t.Skip` is appropriate for unit tests** whose conditions may legitimately not be met (e.g. dataset-dependent assertions). This is the opposite of the e2e rule, where missing required data must fail loudly — the contexts differ.

## Dev flow

Testing is part of the development loop, not a separate phase. When a feature is built or changed, the corresponding unit and e2e tests are written or updated as part of the same change.

## Pointers

- **How to write or debug an e2e test, plus the suite-wide rules** — [`e2e/README.md`](../e2e/README.md).
- **Terse rules for agents working in `e2e/`** — [`e2e/CLAUDE.md`](../e2e/CLAUDE.md) (auto-loads).
