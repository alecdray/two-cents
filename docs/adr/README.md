# Architecture Decision Records

Short entries that capture **why** a decision was made, when the rationale would otherwise be lost once the old approach is gone.

## Decision log

| # | Decision | Summary |
|---|---|---|
| [0001](0001-self-hosted-single-user-service.md) | Self-hosted single-user service, mirroring wax | One Go + SQLite binary in a Docker container the user controls; stack and archetypes adopted wholesale from the sibling `wax` project, with single local login instead of third-party OAuth. |
| [0002](0002-bankprovider-abstraction.md) | Bank access behind a BankProvider abstraction | All bank data flows through a `BankProvider` interface returning our own domain types; v1 ships a single Teller external-client, so a future Plaid provider is an adapter swap, not a rewrite. |
| [0003](0003-two-layer-transfer-detection.md) | Two-layer transfer detection | Transfers are detected first by the bank-provided transaction `type`, then by pairing the inflow leg on another connected account — because Teller exposes no destination-account reference. |

## Format

Each ADR opens with an `# h1` title naming the decision, then states the decision and its rationale in a few short paragraphs: what was decided, why (the constraint or trade-off that forced it), the consequences that follow, and what was rejected.

The **current** state is the codebase — don't restate it. Keep implementation details out: no file names, class names, function names, or exact UI strings that a routine refactor would invalidate. If a sentence would need to change after such a refactor, it doesn't belong here.

## Naming

`NNNN-short-slug.md` — four-digit zero-padded prefix, never renumbered. A decision that replaces an earlier one is a new ADR; reference the prior number in the body. Don't edit the old one.

## When to write one

Write an ADR when a change replaces a meaningful prior approach, or locks in a foundational choice, that a future reader would otherwise wonder *"why is it like this?"* about. Routine changes don't qualify.
