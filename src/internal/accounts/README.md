# accounts

Owns bank **Connections** and the **Accounts** they expose. It is the writer of
connection state, account rows, balances, `kind`, `counts-as-savings`, and
account state; it derives the cash/credit overview consumed by the dashboard.

See the domain model: [`docs/domain/README.md`](../../../docs/domain/README.md) (Accounts section).

## Boundaries

The module is provider-agnostic. It depends on the `banking.BankProvider` seam
(injected) and `banking.ErrReauthRequired`, plus `core/*` — never on a concrete
provider client such as `plaid`. The provider isolation test in
`src/internal/architecture` fails if that boundary is crossed.

## Entities

- **Connection** — a linked bank login (one provider enrollment/Item) and its
  sync-health state: `active`, `needs_reconnect`, or `disconnected`. The access
  token is held encrypted at rest and is never exposed on the entity.
- **Account** — one financial account under a Connection, with its seeded
  cash/credit `kind`, `counts-as-savings` flag, latest balance, display state
  (`active`/`hidden`/`closed`), and the override flags that protect a user's
  choices from being reseeded on sync.

## Behaviour

- **RegisterConnection** — stores the access token encrypted alongside the
  provider item id, lists the login's accounts via the provider, and creates one
  active account per provider account, seeding kind + counts-as-savings and
  loading the initial balance.
- **SyncAccounts** — for each active/needs-reconnect connection, decrypts the
  token, refreshes balances + last-synced, discovers and seeds newly appearing
  accounts, and never duplicates or reseeds existing accounts. A provider
  re-auth signal (`banking.ErrReauthRequired`) flips the connection to
  needs-reconnect (accounts retained); a later clean sync returns it to active.
- **Overview** — total cash (savings included), total credit debt, and net cash
  (cash − debt) over active accounts only; accounts with an unknown balance are
  excluded, not counted as zero.
- **Dashboard** — the read model behind the overview page (`GET /`): the
  `Overview` totals (reusing the same `computeOverview`) plus the active accounts
  grouped into cash / credit / other, each row joined to its connection's
  needs-reconnect state.

The module's `adapters/` serve the overview page at the application root.

Tokens are protected with `core/cryptox` under the configured encryption key.
