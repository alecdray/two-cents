# Two Cents — Scope

> A personal finance app for pulling, categorizing, and reviewing my own bank transactions. ("Your two cents on your own spending.")

Status: **draft** — starting point, expect churn.

## Goal

Aggregate my bank transactions in one place and make spending legible — clean merchant names, categories, balances, and basic spending trends. Built for personal use first, not as a product (yet).

## Bank data source

**Decision: start with [Teller](https://teller.io), keep Plaid as a later upgrade if warranted.**

Why Teller:
- Free for personal use (no linked-account ceiling).
- Cert-based direct REST API — simplest mental model, no hosted-widget token dance, full control of requests.
- Transactions + balances + coarse categories cover ~90% of what this app needs.
- US-only — fine, we're US-based.

When we'd reach for Plaid instead:
- Need richer/granular categorization (two-level taxonomy, merchant logos, confidence scores).
- Need investments or liabilities data.
- Go outside the US, or turn this into a real product.

Design implication: **hide bank access behind a thin `BankProvider` interface** with our own `Transaction` / `Account` types. Write `TellerProvider` now; `PlaidProvider` later is an adapter swap, not a rewrite.

## In scope (v1)

- [ ] Connect bank account(s) via Teller Connect
- [ ] Pull accounts + balances
- [ ] Pull transactions (with pending vs. complete handling)
- [ ] Store transactions locally
- [ ] Display transactions with category + cleaned merchant name
- [ ] Manual re-categorization, with overrides remembered
- [ ] Basic spending summary (by category, by month)

## Out of scope (v1)

- Investments / holdings
- Liabilities (loans, credit-card APR detail)
- Payments / money movement
- Multi-user / accounts for other people
- Mobile app

## Known constraints & risks

- **Re-auth is not optional.** Bank connections expire/break (password change, MFA, bank-side OAuth). Model connections as refreshable, never permanent; design a "needs re-linking" state.
- **Secrets are on us.** Direct model means we store the Teller client certificate + access tokens. Encrypt at rest, never commit them.
- **Categorization is never 100%.** Treat the API category as a default; let user override; persist the override.
- **Teller is US-only** — accepted.

## Open questions

All resolved in the design grill — see [prd.md](./prd.md), [CONTEXT.md](../CONTEXT.md), and [docs/adr/](./adr):

- [x] Project name — **Two Cents** (revisit if it goes public; the `.app`/category space is crowded).
- [x] Stack — **mirrors [`wax`](../../wax)**: Go + templ + htmx + Tailwind/DaisyUI/Bootstrap Icons + SQLite (goose/sqlc), self-hosted single-user via Docker ([ADR-0001](./adr/0001-self-hosted-single-user-service.md)).
- [x] Local-only vs hosted — **self-hosted single-user** (always-on for interval sync; cert on owned infra).
- [x] Initial history depth — **backfill max provider history**; mark incomplete months "partial."
- [x] Category taxonomy — **our own, mapped onto Teller's buckets.**

## Next steps

1. Get a Teller account + client certificate; pull first transactions in a spike (the `teller` external-client).
2. Scaffold the wax-style skeleton: `core/` + `server/`, `taskfile.yml`, goose/sqlc, the `BankProvider` interface, and core `Account` / `Transaction` types.
3. Mirror wax's `docs/architecture/` + `docs/design/` archetype docs into this repo as we start building.
