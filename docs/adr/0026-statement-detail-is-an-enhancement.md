# Card statement detail is an enhancement, never a dependency

Statement detail refines *when* a card's balance is due, and every way of not having it
degrades to the behaviour that preceded it rather than blocking a result or raising an alarm.
Two consequences follow, and they are the decision: a bank login that will not serve statement
detail is recorded as a fact about the affected cards rather than as a state of the connection,
and the cash-flow timeline carries only the billed obligation — this cycle's unbilled spend is
dropped rather than projected.

This extends the model in [0024](0024-cash-flow-timeline-sweep.md), which narrowed the
liabilities non-goal to loan APR and interest detail so that billing-cycle facts could date a
card's obligation.

**Why not a connection state.** [0021](0021-fault-isolating-sync-pass.md) classifies provider
failures by whether the user can act on them rather than by provider vocabulary, and on that
basis a connection reporting no accounts reuses needs-reconnect instead of earning its own
state. That fit because such a connection yields nothing. A login missing this product still
serves balances and transactions in full; the only consequence is a more conservative sweep.
Marking the connection broken would put a breakage badge on a working connection — and a badge
that cries wolf is one the user learns to ignore on the day it means their data actually
stopped. The gap is therefore surfaced where its consequence appears, with the remedy offered
there.

**Why unbilled spend is dropped.** Placing it would mean projecting the billing cycle that will
eventually bill it, and this model forecasts nothing — it carries only what is owed, scheduled,
or declared. This is the one place the model holds back *less* than the proxy it replaced, which
reserved a card's whole balance at once; for a card whose next statement issues early in the
window, the safety margin is the only headroom. Accepted because the alternative is a guess the
bank never made, and because every *other* degradation in the model runs the conservative way.
