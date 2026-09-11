# Goals

What has to be true when this work merges, and the decisions that got settled before
any of it was built. The boundary is in [`scope.md`](scope.md); the durable rationale is
[ADR-0023](../../adr/0023-uncovered-card-debt-reserve.md).

## Outcomes

1. **Overspending stops being offered up.** What is owed on the cards beyond what the
   month's budget already reserves is held back in checking.
2. **The ordinary month is unchanged.** Inside budget, the new term is zero and the
   recommendation is exactly what it was.
3. **The same dollars are never reserved twice.** The card term is net of the budget
   term, which is what makes reading the balance legitimate at all.
4. **The sweep never advises on a card figure it cannot stand behind.** An unknown or
   stale card balance produces a needs-attention snapshot.
5. **No new provider capability.** Credit balances come from the sync the app already
   runs; liabilities stay a non-goal.

## Decisions settled during Spec

Reached by grilling, each closing off a shape that looks reasonable from the outside.

- **Read the card balance; do not reconstruct it.** Charges minus payments over all time
  *is* the balance. Rebuilding it from our own ledger would approximate a figure held
  exactly and break at the backfill edge, where we hold charges whose payments predate
  our history.
- **Net the card term against the budget term.** This is the whole reason ADR-0020's
  refusal to read a card balance no longer applies: its objection was double-counting the
  cycle's spend, and subtracting the budget reserve removes that by construction.
- **No assumption about autopay timing.** The scheduled run is after cards settle, but an
  on-demand run ([ADR-0022](../../adr/0022-on-demand-navigable-sweep-snapshots.md)) can
  land before. Any prior-month or already-paid heuristic is right about half the time; the
  balance carries the answer at every instant. *Rejected:* carrying the prior month's
  overspend forward.
- **No projected-income term.** Income received is already in the live checking balance;
  reserving against income not yet arrived would advise moving money on an unlanded
  paycheck. *Rejected* and recorded, because it is the obvious thing to propose next.
- **Cards sum, with no single-account requirement.** Unlike checking and savings, debt is
  additive and no count of cards is ambiguous.
- **A card-payment transfer subtype is not needed here.** Reading the balance makes it
  unnecessary — a payment reduces the balance and clears the reserve on its own. The app
  still cannot tell a card payment from any other plain transfer, which remains worth
  fixing as separate work.

## How it landed

All five outcomes shipped and no decision was revisited under contact with the code. What
implementation added:

- **A card with an unreported or stale balance blocks rather than contributing zero**, and
  does so as two distinct reasons — "not reporting its balance" and "has not refreshed" are
  different problems with different fixes, the same split checking already makes.
- **Hidden cards are excluded**, falling out of the active filter the other account reads
  already use: a hidden card is out of the picture everywhere else too.
- **The stored snapshot carries the card balance**, defaulting to 0 for snapshots that
  predate the term — which is what they were computed with, so none misrepresents its own
  arithmetic.
- The invariant comment at the arithmetic warns against "simplifying" the card term into
  an independent one, since the deliberate coupling reads like an inconsistency next to the
  other two floored terms.

Coverage: unit tests at each layer — the arithmetic (including the two cases that must
*not* change the number), the derivation and its blocking rules, persistence round-trip,
and the page's figure and reason labels — plus two e2e scenarios: overspending held back,
and a stale card blocking. Gate green: `go build ./...`, `go test ./src/...`, and
`task test/e2e` (84 passing).

One e2e lesson worth recording: the first version of the overspending scenario asserted a
reserve that the shared database's existing budget legitimately covered. The code was
right and the test was not self-contained; it now clears the budget so the term is
measured against a known baseline.
