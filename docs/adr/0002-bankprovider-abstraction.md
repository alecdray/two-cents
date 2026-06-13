# Bank access behind a BankProvider abstraction

All bank data access goes through a `BankProvider` interface that returns our own `Account` / `Transaction` domain types, never provider-native shapes. v1 ships a single Teller client; the rest of the app depends only on the interface.

The Teller client follows wax's **external-client archetype** (see `projects/wax/docs/architecture/archetypes/external-client.md`): a `client.go` (raw HTTP + cert auth), `entities.go` (Teller's wire shapes + conversions to our domain types), and `service.go` (the internal-facing operations domain modules call). Like all external clients, it is a leaf — **no persistence, no `repo.go`, no imports of domain modules**. The `BankProvider` interface is the seam the `service.go` satisfies, so swapping Teller for Plaid means a second external-client module, not a rewrite.

We chose **Teller** for v1 (free for personal use, cert-based direct REST, US-only — sufficient). But Plaid is the likely upgrade if we ever need richer categorization, investments/liabilities, non-US coverage, or go multi-user. Putting a thin interface between the app and Teller now means a future `PlaidProvider` is an adapter swap, not a rewrite, and lets the sync engine and all business logic be tested against a fake provider with no network.

Consequence: provider-specific concepts that don't map cleanly (e.g. Teller's lack of a destination-account reference on transfers — see ADR-0003) are resolved *inside* the provider/sync layer so they don't leak into the domain.

Rejected: calling Teller directly throughout — cheap now, expensive to unwind later, and untestable without live calls or HTTP mocking.
