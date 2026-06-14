# Two Cents — Scope

> A personal finance app for pulling, categorizing, and reviewing my own bank transactions. ("Your two cents on your own spending.")

Status: **draft** — starting point, expect churn.

## Goal

Aggregate my bank transactions in one place and make spending legible — clean merchant names, categories, balances, and basic spending trends. Built for personal use first, not as a product (yet).

## Bank data source

**Decision: use [Plaid](https://plaid.com) (Trial plan). We originally chose Teller, then switched to Plaid — see [ADR-0002](./adr/0002-bankprovider-abstraction.md).**

Why we switched off Teller: Teller closed self-serve developer signup (only login remains; the signup pages 404), so we can no longer get credentials to access the bank network at all — blocking the app at its data source. Plaid was always the documented eventual upgrade.

Why Plaid works now:
- Plaid's free, auto-approved **Trial plan** gives real production data and self-serve "Personal use" signup, with up to ~10 Items (≈10 bank logins) — ample for a single-user self-hosted app.
- Richer categorization: the two-level **`personal_finance_category`** `{primary, detailed}` taxonomy supersedes Teller's flat `type` string — exactly the categorization upgrade this section always named as the reason to reach for Plaid.
- Transactions + balances cover what this app needs; US coverage is fine, we're US-based.

When we'd reach for more of Plaid (post-v1):
- Holdings-level investment data or liabilities detail (loan APR, interest). v1 already tracks loan / investment accounts as `other` from balances alone — this is about the richer per-instrument data.
- Non-US coverage, or turning this into a real product.

Design implication (unchanged): **hide bank access behind a thin `BankProvider` interface** with our own `Transaction` / `Account` types. The provider is an external-client adapter, so Plaid replacing Teller is an adapter swap, not a rewrite.

## In scope (v1)

- [ ] Connect bank account(s) via Plaid Link
- [ ] Pull accounts + balances
- [ ] Pull transactions (with pending vs. complete handling)
- [ ] Store transactions locally
- [ ] Display transactions with category + cleaned merchant name
- [ ] Manual re-categorization, with overrides remembered
- [ ] Basic spending summary (by category, by month)

## Tracked but excluded from spending (v1)

Two Cents is a spending / cash-flow tool, not a net-worth tracker. Accounts that aren't spendable still get pulled and listed, but never enter net cash:

- Loans, mortgage, and investment / brokerage / retirement accounts are tracked as `other` — stored and shown in the account list, but excluded from the spending overview (net cash). We don't pull holdings-level detail or per-loan APR breakdowns.

## Out of scope (v1)

- Liabilities detail (loan APR, credit-card APR / interest breakdown)
- Payments / money movement
- Multi-user / accounts for other people
- Mobile app

## Known constraints & risks

- **Re-auth is not optional.** Bank connections expire/break (password change, MFA, bank-side OAuth). Model connections as refreshable, never permanent; design a "needs re-linking" state.
- **Secrets are on us.** We store the Plaid app credentials (`client_id` + `secret`) and a per-Item `access_token` per connection. Encrypt at rest, never commit them.
- **Categorization is never 100%.** Treat the API category as a default; let user override; persist the override.
- **US-only coverage** — accepted; we're US-based.

## Open questions

All resolved in the design grill — see [prd.md](./prd.md), the [domain model](./domain/README.md), and [docs/adr/](./adr):

- [x] Project name — **Two Cents** (revisit if it goes public; the `.app`/category space is crowded).
- [x] Stack — **mirrors [`wax`](../../wax)**: Go + templ + htmx + Tailwind/DaisyUI/Bootstrap Icons + SQLite (goose/sqlc), self-hosted single-user via Docker ([ADR-0001](./adr/0001-self-hosted-single-user-service.md)).
- [x] Local-only vs hosted — **self-hosted single-user** (always-on for interval sync; secrets on owned infra).
- [x] Initial history depth — **backfill max provider history**; mark incomplete months "partial."
- [x] Category taxonomy — **our own, mapped onto Plaid's `personal_finance_category` buckets.**

## Next steps

1. Get a Plaid account (Trial plan, "Personal use" signup) + `client_id`/`secret`; pull first transactions in a spike (the `plaid` external-client).
2. Scaffold the wax-style skeleton: `core/` + `server/`, `taskfile.yml`, goose/sqlc, the `BankProvider` interface, and core `Account` / `Transaction` types.
3. Mirror wax's `docs/architecture/` + `docs/design/` archetype docs into this repo as we start building.
