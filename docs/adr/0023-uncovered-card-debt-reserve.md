# Uncovered card debt in the sweep reserve

The cash-sweep reserve gains a **third term: what is owed on the cards beyond what this
month's budget already reserves** — the summed credit-account balances, less the remaining
budget reserve, floored at zero. This **reverses [ADR-0020](0020-monthly-cash-sweep-recommendation.md)'s
refusal to read a card balance**, because the reason for that refusal no longer applies:
0020 rejected "projected checking minus the upcoming card balance" for double-counting the
cycle's spend, and netting the budget reserve out of the card term removes that double-count
by construction. The reserve model, the account derivation, and the advisory-only boundary
are unchanged. Liabilities — statement balances, due dates, APR — remain a non-goal; this
reads the ordinary balance the accounts sync already stores.

**The budget reserves what you *planned* to spend; the card records what you *did*.** ADR-0020
holds back this month's unspent budget, and because month-to-date spend counts only what left
checking, spending charged to a card stays reserved forward. That works while a cycle lands
near its budget. It breaks when spending runs past it: the budget term reserves the budget and
stops there, while the bill keeps climbing. A user who has spent $6,000 against a $5,000 budget
is short $1,000 at the moment the sweep tells them how much they can move — and the sweep says
it with a straight face, because nothing in the formula has ever looked at what is owed.

**Netting the budget reserve out is what makes reading the balance legitimate.** The card
balance and the remaining budget reserve are two views of the same upcoming money: the budget
is the plan for it, the balance is the record of it. Reserving both is the double-count 0020
correctly refused. Reserving the balance *only to the extent the budget does not already cover
it* holds each dollar exactly once, and the term collapses to zero for a user who stays inside
their budget — which is why this does not change the number for the ordinary month. The
consequence is that the three reserve terms are no longer mutually independent: each is still
floored at zero, but the card term is **defined net of** the budget term. That is deliberate
overlap-removal, not the term-cancellation the independent flooring exists to prevent — nothing
here can drag another term negative or manufacture a surplus.

**The balance is read, never inferred.** Charges minus payments, accumulated over all time, *is*
the card balance; reconstructing it from our own transaction ledger would rebuild a figure we
already hold exactly, and rebuild it badly — history runs out at the backfill edge, where we
hold charges whose payments predate anything we synced. Reading the synced balance also means
the sweep needs no notion of which transfers were card payments: a payment reduces the balance,
so it clears the reserve the moment it settles, whenever that happens.

**Timing is observed, not assumed — which is why this is not a prior-month calculation.** The
tempting cheaper shape is to carry the *previous* month's overspend forward. It fails on when
the sweep runs. The scheduled run is on the 7th, chosen in 0020 precisely because cards have
"closed and autopaid on the 1st and settled" — by which time last month's overrun has already
left checking and reserving it again would double-count. But a snapshot can now be produced at
any instant ([ADR-0022](0022-on-demand-navigable-sweep-snapshots.md)), so a run may equally land
*before* autopay clears, when the debt is entirely outstanding. Any fixed assumption about
whether the bill has been paid is right roughly half the time. The balance carries the answer
at every instant, so the term needs no assumption at all.

**An unknown or stale card balance blocks.** The balance is now a term in the formula, so a
wrong one produces a confidently wrong number, in the direction that understates what is owed
and overstates what can be swept. It therefore joins the needs-attention reasons on the same
footing as checking, and for the same reason — including staleness, since a card balance that
has not refreshed is precisely the one missing recent spending ([ADR-0021](0021-fault-isolating-sync-pass.md)).
Savings keeps its existing tolerance; it is still not a term in the formula.

**Every active credit account counts, summed.** Unlike checking and savings — where the
derivation demands exactly one and treats ambiguity as needs-attention — debt is additive and
carries no ambiguity about which account is meant. Several cards are simply several balances to
cover, so they sum, and no count of them is a failure.

## Rejected alternatives

- **Carrying the prior month's overspend forward**, measured from our own spending. Avoids the
  card balance, but assumes a payment state that on-demand running makes unpredictable, and
  measures the prior month against the *current* budget config because the budget keeps no
  per-month history.
- **Reconstructing what is owed from the transaction ledger** (charges less direct spend less
  identified card payments). Respects 0020's boundary to the letter, and needs card payments
  modelled as their own transfer subtype — but approximates, through a long chain of inference,
  a number already held exactly, and degrades at the backfill edge.
- **Reserving against projected income** (holding back less because a paycheck is expected).
  Income already received is already in the live checking balance and already raises the sweep;
  reserving against income not yet arrived advises moving money on the strength of an unlanded
  paycheck, and lets one term cancel the others.
- **Treating a missing card balance as zero owed.** Keeps a number on screen more often, at the
  cost of silently understating the reserve — the one direction that risks an overdraft.

## Consequences

- 0020's "it reads **no card or liability balance at all**" no longer holds, and its deferred
  reconciliation of budget against real card spend is closed. The distinction that survives is
  narrower and worth stating: the sweep reads a **balance the accounts sync already stores**, and
  still reaches no liabilities product and no new provider endpoint.
- The reserve's terms are floored independently but are no longer independent in content: the
  card term is net of the budget term.
- For a user inside their budget the term is zero and the recommendation is unchanged; it moves
  the number only for the overspending case it exists to catch.
- Modelling card payments as their own transfer subtype is no longer needed for the sweep. It
  remains worthwhile on its own terms — the app currently cannot tell a card payment from any
  other plain transfer — and is now separable work.
