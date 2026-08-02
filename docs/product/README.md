# Product

The model of the **experience** — organized by feature / user journey. Each row below is a feature as it exists today: what a user can do and why it exists. Present-state only — roadmap lives in [`../backlog/`](../backlog/), history in commit messages and [`../adr/`](../adr/).

Start with [vision.md](vision.md) for the product philosophy and deployment context.

**Product is the entry point; [domain](../domain/) is the deep reference.** A reader meets a feature here — the why and the what at a level a newcomer follows — then follows a link into the owning module's `README.md`, the [domain model](../domain/README.md), or the [decision log](../adr/) for the complete model. Features and the domain model are **orthogonal decompositions of the same system**: one feature composes several domain concepts, one concept surfaces across many features.

**Two boundaries keep this doc from rotting:**
- **Link, don't redefine.** For any term or rule with a specific meaning, link to [domain](../domain/) — never restate the definition here.
- **What and why, not how.** Technical structure belongs in [architecture](../architecture/) and the module READMEs.

## Features

| Feature | What it does | Deep reference |
|---|---|---|
| **Bank connections** | Connect / disconnect / reconnect banks via Plaid Link (or a deterministic fake selected by config); a broken connection surfaces a needs-reconnect state, never silent | [accounts](../../src/internal/accounts/README.md), [plaid](../../src/internal/plaid/AGENTS.md); [ADR-0002](../adr/0002-bankprovider-abstraction.md), [ADR-0006](../adr/0006-bank-provider-selected-by-config.md) |
| **Transaction sync** | Cursor-based incremental sync on a ~6h cron + on demand; dedupe by provider id, pending→posted reconcile, `removed`-set deletes | [transactions](../../src/internal/transactions/README.md) |
| **Accounts overview** | Net cash (cash − credit), free cash + total savings, per-account balances; user overrides for kind (cash/credit/other), counts-as-savings, custom names, and hide/unhide | [accounts](../../src/internal/accounts/README.md); [ADR-0005](../adr/0005-spending-tool-three-bucket-account-kind.md), [ADR-0008](../adr/0008-account-kind-and-savings-overrides.md), [ADR-0017](../adr/0017-custom-account-names.md) |
| **Categorization** | Cleaned merchant names + a precedence engine (manual override > rule > bank category > uncategorized); built-in + custom categories (archive-not-delete); merchant Rules edited in a reusable modal | [categorization](../../src/internal/categorization/README.md); [ADR-0016](../adr/0016-rule-editor-modal-and-cross-modal-return.md) |
| **Transfers & savings** | Transfers between own accounts are excluded from spending/income; subtype paired by amount + date window; contributions into savings-flagged accounts count as savings | [categorization](../../src/internal/categorization/README.md); [ADR-0003](../adr/0003-two-layer-transfer-detection.md) |
| **Transaction views** | `/transactions` list with merchant search + a deep-linkable needs-attention worklist, month dividers, and a shared editable row (leading merchant-logo / category-glyph avatar) reused on every surface | [transactions](../../src/internal/transactions/README.md); [ADR-0011](../adr/0011-reusable-transaction-editing-modal.md), [ADR-0013](../adr/0013-richer-bank-transaction-detail.md), [ADR-0019](../adr/0019-transaction-row-avatars.md) |
| **Budget** | A single rolling monthly budget (income target, savings target, per-category limits) with an "Everything else" residual; no rollover | [budget](../../src/internal/budget/README.md) |
| **Month-navigable home** | One month-rail surface: the current month shows the budget-relative Tracker (remaining, weekly/daily pace, income/savings progress); earlier months show actuals-only wraps with spend-by-category and figure drill-downs | [tracker](../../src/internal/tracker/AGENTS.md), [reporting](../../src/internal/reporting/AGENTS.md), [home](../../src/internal/home/README.md); [ADR-0009](../adr/0009-category-spend-drill-down.md), [ADR-0012](../adr/0012-wrap-income-savings-and-month-list-drill-ins.md), [ADR-0018](../adr/0018-month-navigable-home.md) |
| **Cash-sweep recommendation** | Advisory only (never moves money): a monthly job computes `suggested_sweep = checking − reserve − safety margin` with a checking↔savings direction; `/sweep` shows the latest with its full breakdown, plus empty-first-run and needs-attention states | [sweep](../../src/internal/sweep/README.md); [ADR-0020](../adr/0020-monthly-cash-sweep-recommendation.md) |
| **App chrome** | Fixed bottom-bar navigation with a More overflow; app-wide request feedback (top progress bar + "Sync now" in-progress state and inline confirmation) | [home](../../src/internal/home/README.md); [ADR-0014](../adr/0014-bottom-bar-navigation.md), [ADR-0015](../adr/0015-app-wide-request-feedback.md) |
| **Login** | The whole single-user app sits behind one password-only local login; sliding `HttpOnly` session cookie | [auth](../../src/internal/auth/README.md); [ADR-0007](../adr/0007-single-local-login.md) |
