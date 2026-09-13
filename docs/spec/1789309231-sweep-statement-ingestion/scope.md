# Scope — statement ingestion (chunk B)

Chunk B of the cash-flow timeline sweep. The model, its rationale and the slice boundary are
[ADR-0024](../../adr/0024-cash-flow-timeline-sweep.md) and the chunk-A record
([`../1789161803-sweep-reserve-foundation/`](../1789161803-sweep-reserve-foundation/)) — this
folder does not restate them.

Chunk A ships a timeline in which every card contributes its whole balance at the run instant,
because no statement detail is held: the no-statement path falls out of the missing-data rule
rather than being a stub, so the sweep is correct but deliberately over-reserved. B gives the
card obligation its real shape — a provider seam for statement detail, storage for what it
returns, a sync step that keeps it current, and the per-card payment schedule that splits a
balance by when it is actually due. Occurrence handling is untouched; that is chunk C.

The number moves both ways as a result: lower for a statement falling in the next cycle,
**higher** when a heavy statement lands before payday — which the whole-balance-at-now proxy
did not so much under-reserve as mis-date.

**Carried assumption, unconfirmed.** The chunk-A coverage check confirmed every linked issuer
reports the liabilities product, but left login consent open: all three are OAuth institutions,
where adding a product to an existing login generally means re-consent, and settling it needs a
live credential only the deployed instance holds. The chunk-A spec's instruction was "plan for
re-establishing logins; confirm before building it." **That confirmation is being skipped by
decision**, so this chunk plans for re-establishing logins as user-facing work and must not
treat its absence as evidence it is unnecessary.

## Boundary

**In:** the provider seam for billing-cycle facts and the client that satisfies it; storage of
statement detail on the credit Account, refreshed inside the existing sync pass; the per-card
payment schedule; the dated card events that result; and the per-card record of a login that
will not serve statement detail, with its remedy offered where the consequence appears.

**Out:** any bulk re-consent campaign or a consent-specific link mode — nothing has yet
demonstrated either is needed, and the chunk-A instruction was to confirm before building
([ADR-0026](../../adr/0026-statement-detail-is-an-enhancement.md) is what makes that deferral
safe: the gap degrades rather than breaks). Occurrence matching remains chunk C.

## Decisions taken

Recorded in [ADR-0026](../../adr/0026-statement-detail-is-an-enhancement.md); the reasoning is
there, not here.

- A login that will not serve statement detail is a fact about the **cards**, not a state of the
  **connection** — departing from [0021](../../adr/0021-fault-isolating-sync-pass.md)'s reuse of
  needs-reconnect.
- The timeline carries only the **billed** obligation; this cycle's unbilled spend is dropped
  rather than projected. This is the one place the model holds back less than chunk A did.
- Because the gap degrades safely, the consent question stops being a prerequisite for this
  chunk and becomes a response to an observed condition.

Settled by invariants already in the model, not re-decided here: every blocking reason is a
missing dollar value, so statement **dates** degrade and a missing statement balance falls back
to the current balance; and statement detail inherits the balance's staleness stamp, so it needs
no second staleness rule.
