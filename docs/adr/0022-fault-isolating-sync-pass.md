# Fault-isolating sync pass and surfaced balance staleness

The bank sync runs every six hours over every connection. Until now a single
connection's failure ended the entire pass: `SyncAccounts` returned on the first
connection that errored, and `SyncTransactions` returned on the first stage that
errored. One Item stuck in a permanent provider-side state therefore stopped
*every* connection from refreshing its balances, pulling transactions, running
the categorize sweep, and resolving transfer pairings — indefinitely, since the
condition never cleared on its own. This ADR changes the pass to isolate faults,
widens the provider error classification that decides a connection's fate, and
surfaces the staleness that a silently failing connection produces.

**Every connection is attempted, and every stage runs.** A failure is tagged with
its connection id, collected, and the joined error returned once the pass
finishes. The alternative — keep failing fast and rely on the next tick — is what
produced the outage: the next tick fails identically, so "temporarily degraded"
is indistinguishable from "permanently stopped". Isolation is a property the
per-connection cursor model already supports: each connection's rows and cursor
advance in their own transaction, so one connection's failure was never a
correctness reason to abandon the others, only an unexamined control-flow
default. The one thing that still ends a pass early is failing to enumerate what
to sync at all, which leaves nothing to iterate.

**A returned error now means "something in this pass failed", not "this pass did
nothing".** The cron still logs the failure — visibility is not traded away for
resilience — but the log line no longer implies the work was skipped. Anything
reading the sync's error to decide whether data is current must read the
per-account sync stamp instead, which is what the staleness surfacing below does.

**Provider errors are classified by whether the user can act on them, not by
whether they are Plaid errors.** The client previously translated exactly one
code (`ITEM_LOGIN_REQUIRED`) into `banking.ErrReauthRequired`; everything else
became an unclassified generic status error. That is the wrong axis. The
distinction that matters downstream is whether relinking resolves the condition:
if it does, the connection should be flagged needs-reconnect and skipped; if it
does not, the failure is transient and the connection must stay active so the
next tick retries it. `NO_ACCOUNTS` — the institution reporting no valid accounts
for the Item — is user-actionable and joins the first group. Membership stays
deliberately narrow: misclassifying a transient failure tells the user to
reconnect a perfectly valid login, and the set is expanded per-code with a stated
reason rather than by pattern-matching on `error_type`.

**`NO_ACCOUNTS` reuses needs-reconnect rather than getting its own state.**
Relinking through Link's update mode is genuinely what resolves it — the user
reselects accounts, or discovers there are none — so it shares the existing
state's exit path, UI, and reconnect control. The cost is copy: the badge says
"reconnect" when the truer statement is "confirm accounts exist at your bank". A
distinct state would say it better at the price of a migration and a second
lifecycle to maintain, which a single-user app does not earn. Revisit if a third
user-actionable code arrives wanting different guidance.

**Balance staleness is surfaced, because fault isolation alone would hide the
problem better than it found it.** A connection failing on an *unclassified*
error stays active by design — nothing flags it, and with the pass no longer
failing loudly the only remaining symptom is a balance that quietly stops
moving. A stale figure is indistinguishable from an unchanged one: both render
as an authoritative number. The overview therefore marks an account whose balance
has gone longer than **24 hours** without a refresh — four consecutive failed
six-hourly passes, long enough that a transient outage never cries wolf, short
enough that a stuck connection is visible within a day. A never-synced account
counts as stale: its balance has never been confirmed against the bank at all.

**Needs-reconnect and stale are independent facts.** A connection can be stale
without needing reconnection (the unclassified-error case) and freshly flagged
while its balance is still current. The read model carries both and the view
picks: a row already showing the reconnect badge suppresses the stale mark, so it
never carries two explanations of one thing.

## Rejected alternatives

- **Fail fast and alert.** Keeps the pass simple and makes failures loud, but the
  loudness is indiscriminate — one broken Item denies every healthy connection
  its sync. Resilience here costs only error aggregation.
- **Swallow per-connection failures entirely** (log, return nil). Healthy
  connections would sync, but the cron would report success while a connection
  rotted, trading a visible outage for an invisible one.
- **Classify by `error_type: ITEM_ERROR` wholesale.** Mechanically simpler, but
  the type spans conditions with opposite handling — some clear on retry, some
  never do — so it would flag valid logins as needing reconnection.
- **A distinct `no_accounts` connection state.** More accurate messaging, but a
  migration plus a second lifecycle for one code, in an app with one user.
- **Deriving staleness from the last sync's error** rather than a per-account
  stamp. The error says a pass failed, not which accounts went un-refreshed —
  and with isolation those are no longer the same set.

## Consequences

- `SyncTransactions` and `SyncAccounts` return aggregated errors; a non-nil error
  no longer implies the pass did nothing. The `transactions` module doc's
  per-connection isolation claim, previously true only of the pull stage and only
  for re-auth, now describes the whole pass.
- The accounts overview gains a stale-balance mark and `AccountRow` carries
  `LastSyncedAt` + `Stale`. The deferred "surfaced last-synced time"
  backlog item is partly delivered: staleness is shown, an always-visible
  last-synced time is not.
- Adding a user-actionable provider error code is now a one-line change with a
  required reason, rather than a new branch in the client's error path.
