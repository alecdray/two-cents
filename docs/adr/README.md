# Architecture Decision Records

Short entries that capture **why** a decision was made, when the rationale would otherwise be lost once the old approach is gone.

## Decision log

| # | Decision | Summary |
|---|---|---|
| [0001](0001-self-hosted-single-user-service.md) | Self-hosted single-user service, mirroring wax | One Go + SQLite binary in a Docker container the user controls; stack and archetypes adopted wholesale from the sibling `wax` project, with single local login instead of third-party OAuth. |
| [0002](0002-bankprovider-abstraction.md) | Bank access behind a BankProvider abstraction | All bank data flows through a `BankProvider` interface returning our own domain types. We chose Teller, then switched to Plaid (Teller closed self-serve signup; Plaid's Trial plan unblocks signup and its `personal_finance_category` upgrades categorization) — the interface made it an adapter swap, not a rewrite. |
| [0003](0003-two-layer-transfer-detection.md) | Two-layer transfer detection | Transfers are detected first by the bank-provided category's primary level, then by pairing the inflow leg on another connected account — because Plaid exposes no destination-account reference. |
| [0004](0004-configured-app-timezone.md) | Single configured timezone for time reckoning | "Today," days-left-in-month, and the current month are reckoned in one configured app timezone (default EST), not a per-request browser zone — so scheduled jobs share the clock and month boundaries stay stable. |
| [0005](0005-spending-tool-three-bucket-account-kind.md) | Spending tool — three-bucket account kind | Two Cents is a spending tool, not a net-worth tracker. Account kind is `cash | credit | other` (depository / cards / everything else); loans, mortgage, and investments are tracked as `other` and excluded from net cash. |
| [0006](0006-bank-provider-selected-by-config.md) | Bank provider selected by configuration | The composition root picks the `BankProvider` by config (`BANK_PROVIDER`): the default reaches the live bank network, a "fake" value selects a deterministic in-process stand-in. Promotes the unit-test fake to a wiring option so the connection flows run end to end against the real server with no browser mocking and no live bank. |
| [0007](0007-single-local-login.md) | Single local login | The whole single-user app sits behind one password-only login; the credential is a hashed row in the DB (not config, rotated by a CLI command), the session is a sliding `HttpOnly` cookie, and no domain data is partitioned by user, so authorization is binary. |
| [0008](0008-account-kind-and-savings-overrides.md) | Account kind and counts-as-savings overrides | The kind/counts-as-savings overrides are surfaced as an inline per-row picker on the accounts overview. The savings toggle is offered on every bucket but `credit`; overriding an Account to `credit` force-clears its counts-as-savings flag (a deliberate coupling that preserves the pairing engine's no-kind-check invariant); and an effective flag change eagerly re-pairs existing Transfers. |

## Format

Each ADR opens with an `# h1` title naming the decision, then states the decision and its rationale in a few short paragraphs: what was decided, why (the constraint or trade-off that forced it), the consequences that follow, and what was rejected.

The **current** state is the codebase — don't restate it. Keep implementation details out: no file names, class names, function names, or exact UI strings that a routine refactor would invalidate. If a sentence would need to change after such a refactor, it doesn't belong here.

## Naming

`NNNN-short-slug.md` — four-digit zero-padded prefix, never renumbered. A decision that replaces an earlier one is a new ADR; reference the prior number in the body. Don't edit the old one.

## When to write one

Write an ADR when a change replaces a meaningful prior approach, or locks in a foundational choice, that a future reader would otherwise wonder *"why is it like this?"* about. Routine changes don't qualify.
