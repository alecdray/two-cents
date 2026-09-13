# Vision

**What it is.** Two Cents is a personal finance app for pulling, categorizing, and reviewing my own bank transactions — "your two cents on your own spending." It aggregates transactions and balances across my accounts (via Plaid), makes spending legible (clean merchant names, categories, a budget, a live tracker, a month wrap), and answers *where does my money go* and *am I on track this month* without wrangling exports by hand.

**Who it's for.** One person — me. Built for personal use, not (yet) as a product. There is a single user; no data is partitioned by user and authorization is binary.

**Design philosophy.**
- **A spending / cash-flow tool, not a net-worth tracker.** Accounts that aren't spendable (loans, mortgage, investments) are tracked as `other` — listed but excluded from net cash. See [ADR-0005](../adr/0005-spending-tool-three-bucket-account-kind.md).
- **The bank is an adapter, not the app.** All bank data flows through a `BankProvider` interface returning our own domain types, so a provider swap is an adapter change, not a rewrite. See [ADR-0002](../adr/0002-bankprovider-abstraction.md).
- **The API category is a default, never the truth.** Categorization is never 100%; the user overrides, and overrides persist across re-sync.
- **Mobile-first, HTMX-first, server-rendered.** Stack and conventions mirror the sibling `wax` project.

**Deployment context.** Self-hosted, single-user service: one Go + SQLite binary in a Docker container on infra the user controls, always-on so the ~6h sync runs ([ADR-0001](../adr/0001-self-hosted-single-user-service.md)). The whole app sits behind one password-only local login ([ADR-0007](../adr/0007-single-local-login.md)) — not exposed as a multi-tenant public service. Scale is one user and ~10 bank connections. Security calibration for reviews: the sensitive assets are the stored Plaid credentials + per-Item access tokens (**encrypt at rest, never commit**); the outbound logo fetch is bounded to the provider's HTTPS CDN to cap SSRF surface ([ADR-0019](../adr/0019-transaction-row-avatars.md)); auth is binary, so the threat model is a single trusted user rather than mutual isolation.

**What it is not (non-goals).**
- Investments / holdings detail, and loan APR / credit-card interest breakdown. Credit-card **billing-cycle** facts — statement balance, issue date, payment due date — are in scope, because the cash sweep needs to know when a card's balance is actually due ([ADR-0024](../adr/0024-cash-flow-timeline-sweep.md)); what the card *costs* to carry remains out.
- Payments / money movement initiated from the app.
- Multi-user / managing other people's accounts.
- A native mobile app.
- Non-USD / non-US banks (US-only coverage, accepted).
- Budget rollover / envelope carry-over (v1 is monthly, no rollover).
