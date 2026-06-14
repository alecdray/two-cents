# fakebank — external client

Rules: ../../../docs/architecture/archetypes/external-client.md

Satisfies the `banking.BankProvider` seam (`src/internal/banking`); returns only `banking` domain types. The deterministic test/dev stand-in for a real provider, selected at the composition root by `BANK_PROVIDER=fake`. Decision: [ADR-0006](../../../docs/adr/0006-bank-provider-selected-by-config.md).

Module-specific notes:
- No remote service to wrap, so no `client.go`/`entities.go` — a single `service.go` returns canned data with no network. Still a leaf: imports only `banking` + `core/contextx` + stdlib.
- The accounts, balances, and tokens are **fixed on purpose**; end-to-end tests assert on these exact values. Change them only with the dependent tests.
- `CreateLinkToken` tags its token `Mode: "fake"` so the front end opens the simulated connect flow; `SyncTransactions` reports no transactions (accounts + balances only).
