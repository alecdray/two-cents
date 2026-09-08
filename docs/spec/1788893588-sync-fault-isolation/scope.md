# Fault-isolating sync pass and surfaced balance staleness

Triggered by a production outage: the six-hourly `transactions-sync` cron failed
every tick with a Plaid `NO_ACCOUNTS` (`ITEM_ERROR`) on `/accounts/get`, and the
failure took the whole pass down — no balances, no transaction pulls, no
categorize sweep, no transfer pairing, for every connection, indefinitely.

Root cause was two layered defects, not the Plaid error itself:

1. **`plaid/client.go` classified exactly one error code.** `ITEM_LOGIN_REQUIRED`
   mapped to `banking.ErrReauthRequired`; every other code — including
   `NO_ACCOUNTS`, which never clears on retry — fell through to a generic
   `unexpected status 400`.
2. **`accounts.SyncAccounts` returned on the first failing connection**, and
   `transactions.SyncTransactions` returned on the first failing stage. Only
   `ErrReauthRequired` was isolated. The `transactions` module doc's claim that
   "per-connection failures are isolated" was true only of the pull stage.

A third gap made it hard to diagnose: the error carried no connection or item id,
so the log could not identify which Item was stuck.

Rationale, the classification axis, the needs-reconnect reuse, and the rejected
alternatives are in [ADR-0022](../../adr/0022-fault-isolating-sync-pass.md).

## In scope

- **Classify provider errors by user-actionability.** A named set of Plaid error
  codes maps to `banking.ErrReauthRequired`; `NO_ACCOUNTS` joins
  `ITEM_LOGIN_REQUIRED`. Transient/institution errors deliberately stay out, so a
  valid login is never flagged for reconnection.
- **Fault isolation across the whole pass.** `SyncAccounts` attempts every
  connection; `SyncTransactions` runs every stage. Failures are collected and
  returned joined. Only `ConnectionsToSync` failing still ends a pass early.
- **Connection identity in failures.** Each collected error names its connection
  id (and item id at the accounts stage), so a cron log points at the Item.
- **Partial vs total failure at the caller seam.** Isolation makes "returned an
  error" and "achieved nothing" two different questions, so a pass that failed
  but still synced at least one connection reports itself as partial. Without
  this the manual sync control renders a total-failure message over a region
  holding rows it just synced — caught by the pre-merge audit, not the original
  pass.
- **Surfaced balance staleness.** `AccountRow` carries `LastSyncedAt` and a
  derived `Stale`; the overview marks an account un-refreshed for more than 24
  hours. A row already showing the needs-reconnect badge suppresses the mark.
- Reconciling the canonical docs: ADR-0022, the `plaid` / `accounts` /
  `transactions` `AGENTS.md`, the `accounts` overview behaviour, and the backlog
  entries this supersedes.

## Out of scope

- **A distinct `no_accounts` connection state.** `NO_ACCOUNTS` reuses
  needs-reconnect; the copy says "reconnect" where "confirm accounts exist at
  your bank" would be truer. Deliberate — see ADR-0022's rejected alternatives.
- **An always-visible last-synced time** on every account. Only the stale case is
  surfaced; the general "last synced at" display stays a
  [backlog item](../../backlog/features.md).
- **Retry / backoff policy.** A failing connection is retried on the next
  six-hourly tick, unchanged. No per-connection backoff, no immediate retry.
- **Alerting.** The cron log remains the only failure channel; no notification.
- **Broadening the classified set beyond `NO_ACCOUNTS`.** Other codes are added
  per-code with a stated reason as they are actually observed.
- **The `.env`-dependent `core/app` config test failure** surfaced while running
  the gate (`task test/unit` loads the real `.env` via `dotenv:`, so
  `config_test.go` reads the live `PLAID_SECRET` and prints it). Pre-existing and
  unrelated; recorded in [bugs.md](../../backlog/bugs.md).
