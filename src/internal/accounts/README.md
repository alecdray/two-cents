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
- **Disconnect** — removes a linked bank: decrypts the connection's token,
  severs the login at the provider (`RemoveItem`), then in one transaction
  deletes the connection's accounts and the connection itself.
- **BeginReconnect / CompleteReconnect** — reconnect a connection whose login
  expired (`needs_reconnect`). `BeginReconnect` mints an update-mode link token
  for the existing login (real mode only). `CompleteReconnect` confirms the
  refreshed login works by re-reading its accounts/balances through the provider
  (reusing the per-connection sync), then sets the connection active. If the
  provider still rejects the login (any error, including
  `banking.ErrReauthRequired`), it returns the error and leaves the connection
  `needs_reconnect`.
- **SetAccountKind / ToggleCountsAsSavings** — the per-account overrides behind
  the overview's kind picker. `SetAccountKind` sets `kind` and marks it
  overridden so sync never reseeds it; choosing `credit` also force-clears
  `counts-as-savings` (a credit destination is never a savings contribution —
  [ADR-0008](../../../docs/adr/0008-account-kind-and-savings-overrides.md)).
  `ToggleCountsAsSavings` flips the flag and marks it overridden. Both are pure
  accounts writes; each reports whether the effective `counts-as-savings` value
  changed, so the adapter can fire the re-pair seam without the service reaching
  into transactions.
- **Overview** — total cash (savings included), total credit debt, and net cash
  (cash − debt) over active accounts only; accounts with an unknown balance are
  excluded, not counted as zero.
- **Dashboard** — the read model behind the overview page (`GET /accounts`): the
  `Overview` totals (reusing the same `computeOverview`) plus the active accounts
  grouped into cash / credit / other. Each row carries its account id and current
  kind / counts-as-savings (so the row's picker can target the account and render
  its state) alongside its owning connection's id and needs-reconnect state.

## Connection management on the overview

The whole connection lifecycle is surfaced on the overview at `/accounts`,
swapping the shared overview region in place rather than reloading:

- **Connect** — the connect control links a bank; on success its accounts appear
  in their groups. A failed link renders a recoverable inline error in the
  control, leaving any already-linked accounts in view.
- **Disconnect** — each account row carries a disconnect control. Because
  removal is irreversible it requires an explicit confirmation (a dialog) before
  firing; confirming deletes the bank and its accounts, cancelling removes
  nothing. It is a server action in both bank modes.
- **Reconnect** — a `needs_reconnect` row shows a badge and a reconnect control.
  A successful reconnect clears the badge in place; a still-failing login renders
  a recoverable inline error beside the control with the badge intact.
- **Kind & savings override** — each row carries an inline `kind` picker
  (cash / credit / other) and, on `cash` and `other` rows only, a
  counts-as-savings toggle. Both apply on change and swap the shared region, so a
  kind change recomputes net cash, re-buckets the row, and re-renders its own
  controls (the savings toggle appears or vanishes as the kind crosses the
  `credit` boundary). A change that alters the effective counts-as-savings flag
  re-pairs existing transfers immediately through the injected re-pair seam (see
  below), so the Tracker reflects it at once. Server actions in both bank modes.

Every bank interaction goes through the injected `banking.BankProvider` seam —
the module never imports a concrete provider. The `BANK_PROVIDER=fake`
deterministic stand-in (one connection, a fixed three-account set, no-op
`RemoveItem`) is selected at the composition root and drives the browser-level
e2e of connect, disconnect, reconnect, and the kind/savings override.

The kind/savings override re-pair is an injected seam, mirroring the
connect/reconnect transaction backfill: the adapter calls it after an override
that changed the effective counts-as-savings flag, the composition root wires it
to the transactions re-pair, and a nil seam skips the post-action. The accounts
service never imports transactions.

The module's `adapters/` serve the overview page at `/accounts`.

Tokens are protected with `core/cryptox` under the configured encryption key.
