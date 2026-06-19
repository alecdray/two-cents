# Bank access behind a BankProvider abstraction

All bank data flows through a `BankProvider` interface that returns our own `Account` / `Transaction` domain types, never provider-native shapes, so the concrete provider is an adapter swap rather than a rewrite. The provider is Plaid, following wax's external-client archetype — a leaf with no persistence and no domain imports.

We first chose Teller, then switched to Plaid when Teller closed self-serve developer signup, blocking us from the credentials needed to reach the bank network; Plaid's auto-approved Trial plan unblocked signup and its `personal_finance_category` upgraded categorization. Because the seam was already in place the switch touched only the provider leaf, and the sync engine and business logic stay testable against a fake provider with no network. Provider quirks that don't map cleanly (e.g. Plaid exposes no destination-account reference on transfers — [ADR-0003](0003-two-layer-transfer-detection.md)) are resolved inside the provider/sync layer so they never leak into the domain.

Rejected: calling the provider directly throughout — cheap now, untestable without live calls, and exactly the unwind cost the seam avoided when Teller had to go.
