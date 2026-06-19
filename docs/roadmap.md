# Two Cents — Roadmap

Status overview of what's **built**, what's **committed but not yet built**, and the **deferred
backlog**. This is a navigational summary, not a new source of truth: the *what/why* lives in
[`prd.md`](./prd.md) + [`domain/README.md`](./domain/README.md), decisions in [`adr/`](./adr/), and the
detail of each shipped feature in its build's `as-built.md` (`~/workshop/builds/two-cents-*/`). When this
list and those docs disagree, those docs win — update this file.

**How work ships:** each feature is a vertical slice taken through the four-phase
[development process](./process.md) — spec → implement → audit → merge.

Legend: ✅ shipped · 🔜 committed, not built · 🧊 deferred backlog · ⚠️ known issue / tech debt.

---

## ✅ Shipped (on `main`)

| Capability | Notes | Refs |
|---|---|---|
| **Bank-provider abstraction** | `BankProvider` seam returning our own `Account`/`Transaction` types; provider is an adapter swap. | [ADR-0002](./adr/0002-bankprovider-abstraction.md) |
| **Plaid client + provider selection** | `plaid` external-client (Link token, public-token exchange, `/transactions/sync`, item remove); `fakebank` deterministic stand-in selected by `BANK_PROVIDER`. | [ADR-0006](./adr/0006-bank-provider-selected-by-config.md) |
| **Accounts & connections persistence** | sqlc + SQLite; per-Item access token encrypted at rest (`core/cryptox`). | `two-cents-accounts-slice` |
| **Connect / disconnect / reconnect** | Plaid Link (real) + direct (fake); needs-reconnect state surfaced; inline errors. Real Link validated in Plaid **sandbox**. | `two-cents-connect-bank` |
| **Accounts overview** | Net cash = cash − credit; 3-bucket `kind` (cash/credit/other, user-seeded, `other` excluded). Now at `/accounts`. | [ADR-0005](./adr/0005-spending-tool-three-bucket-account-kind.md) |
| **Transactions sync** | Cursor-based incremental, ~6h cron + on-demand, dedupe by provider id, pending→posted reconcile, `removed`-set deletes; `/transactions` list. | `two-cents-transactions-sync` |
| **Categorization** | Pure precedence engine (override > Rule > bank PFC > direction), built-in PFC taxonomy + custom categories (archive-not-delete), Rules, `/categories` + `/rules` + re-categorize picker. | `two-cents-categorization` |
| **Transfer subtype pairing** | `ResolveTransferSubtype` (exact-cent amount, ±3-day window, conservative); savings-contribution detection; sticky `MarkTransferDestination`; transactions-page chip + picker. | [ADR-0003](./adr/0003-two-layer-transfer-detection.md), `two-cents-transfer-subtype` |
| **Budget + Tracker + Month-wrap** | Rolling budget config + `/budget` editor; current-month Tracker at `/` (remaining, pace, income/savings progress, Everything-else); `/wraps` list + month wrap (actuals only, settling/partial). Configured app timezone. | [ADR-0004](./adr/0004-configured-app-timezone.md), `two-cents-budget-tracker-wrap` |
| **Budget UI polish** | Budget page hides empty categories + add-category control + live residual/balance; Everything-else rendered as a category row on the Tracker. | direct commits (not a `/build`) |
| **Single local login** | Password-only login gating the whole app; hashed credential in a single `users` row, set/rotated via `task auth/set-password`; sliding `HttpOnly` session cookie; session machinery in `core`, login flow in a new `auth` module. e2e authenticates once via global setup. | [ADR-0007](./adr/0007-single-local-login.md) |
| **Account kind & savings overrides** | Inline per-row picker on `/accounts`: kind (cash/credit/other) re-buckets + recomputes net cash; counts-as-savings toggle on cash/other rows. Overriding to `credit` force-clears the savings flag; an effective flag change eagerly re-pairs transfers through an injected seam. | [ADR-0008](./adr/0008-account-kind-and-savings-overrides.md) |

Covers PRD user stories 1–25, 27–44 and spending-by-category aggregation (the wrap).

---

## 🔜 Committed, not yet built

Things v1 intends (named in the PRD/ADRs) that aren't built yet:

- **Category → transactions drill-down** (PRD story 26). Spend-by-category totals exist (wrap); drilling
  from a total into the underlying transactions does not.
- **Real-Plaid (production) validation.** Connect is proven in **sandbox**; categorization, transfer
  pairing, and budget/tracker/wrap have only been exercised against the fake provider + Go tests — not
  against real production-bank data.

---

## 🧊 Backlog (deferred / out of scope for v1)

From the PRD's *Out of Scope*, the domain model's deferred notes, and the slices' *Known gaps*:

**Near-term candidates (usability):**
- Transactions **filtering / search / pagination** (list is flat, ≤100) and per-account drill-down.
- **Refund → prior-outflow pairing** (a refund inflow matched to its original purchase) — a named post-v1 gap.
- **External-account entity** for transfers to *unconnected* accounts (today you can mark a subtype, not a real destination).
- **Precise provider history window** for the wrap's `partial` flag (today a connect-month heuristic; under-flags later connections — see tech debt).

**Explicitly deferred (PRD out-of-scope):**
- **Budget rollover** / envelope carry-over (v1 is monthly, no rollover).
- **Historical per-week/day actual breakdown** (the week/day view is forward pace only).
- **Goal-based named savings** (beyond a single monthly savings target).
- **Bill / subscription detection & reminders.**
- **Category merge** (archive/rename only in v1).
- **Fuzzy / pattern merchant matching** in rules (v1 is substring on the cleaned merchant).
- **Investments / holdings & liabilities detail** (loan APR, interest breakdown).
- **Payments / money movement** initiated from the app.
- **Multi-user** / managing others' accounts.
- **Mobile app.**
- **Non-USD / non-US banks.**

---

## ⚠️ Known issues / tech debt

- **Flaky e2e** `transaction-categorization.spec.ts:67` ("manual re-categorization survives a later sync")
  — an htmx `selectOption → waitForResponse` race; predates the budget slices. Suite is green with
  `--retries=2`. Candidate for a `/diagnose`.
- **Disconnect hard-deletes accounts** instead of the domain's terminal `closed` state (a dangling
  transfer-destination FK) — see [`architecture/known-gaps.md`](./architecture/known-gaps.md).
- **`PartialFlag` under-flags** later-added connections' backfill-edge months (correct for the common
  single-connection case; a precise history window is in the backlog above).
- Architectural violations, as they arise, are tracked in
  [`architecture/known-gaps.md`](./architecture/known-gaps.md).

---

## Pointers

- Feature intent & v1 scope — [`prd.md`](./prd.md) · direction — [`scope.md`](./scope.md)
- Domain language & decomposition — [`domain/README.md`](./domain/README.md)
- Decisions — [`adr/`](./adr/) · architecture rules — [`architecture/`](./architecture/)
- Per-feature build records — Scope→…→Done, as-built, and the per-slice design spec — `~/workshop/builds/two-cents-*/`
