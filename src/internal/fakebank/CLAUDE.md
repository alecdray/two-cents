# fakebank — external client

Rules: ../../../docs/architecture/archetypes/external-client.md

Satisfies the `banking.BankProvider` seam (`src/internal/banking`); returns only `banking` domain types. The deterministic test/dev stand-in for a real provider, selected at the composition root by `BANK_PROVIDER=fake`. Decision: [ADR-0006](../../../docs/adr/0006-bank-provider-selected-by-config.md).

Module-specific notes:
- No remote service to wrap, so no `client.go`/`entities.go` — a single `service.go` returns canned data with no network. Still a leaf: imports only `banking` + `core/contextx` + `core/timex` + stdlib.
- The accounts, balances, and tokens are **fixed on purpose**; end-to-end tests assert on these exact values. Change them only with the dependent tests.
- `CreateLinkToken` tags its token `Mode: "fake"` so the front end opens the simulated connect flow.
- `SyncTransactions` backfills a **fixed** set on the first pull (empty cursor), spanning the fixed accounts, and returns a non-empty resume cursor; presented that cursor it reports no further changes. The set spans the categorization ladder so e2e can exercise it: a posted spending outflow, a posted income inflow (negative), a pending spending charge, a transfer-signal outflow (→ Transfer), and an inflow with no usable bank category whose merchant a seeded Rule can match (→ needs-review until re-categorized). The set is fixed on purpose (tests assert on it); change it only with the dependent tests. Transaction **dates** are the one exception to "fixed": each is anchored to the current month (day-of-month 1–5, midnight UTC) reckoned in the configured app timezone (`timex.CurrentMonth`), so the set always lands in the month the read-side treats as "now" and month-relative views (tracker, wrap) hold whatever month the suite runs in — a fixed month would fall out of "this month" at the next calendar roll.
