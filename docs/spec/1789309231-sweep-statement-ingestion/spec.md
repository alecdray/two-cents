# Spec

What changes relative to [what chunk A shipped](../1789161803-sweep-reserve-foundation/spec.md).
The model, the computation, and the card-statement and payment-schedule entities are already
specified there and are **not restated**; this records what B adds, what it decides differently,
and what it deliberately leaves alone.

## The one-line change

Chunk A's card branch is written but only its first line is reachable — with no statement held,
every card takes the no-statement path and contributes its whole balance at the run instant. B
supplies the statement detail, so the rest of that branch becomes live: the obligation is capped
at what was billed and dated by the card's payment schedule.

Nothing about the evaluator, the horizon, the occurrence window, or the same-day ordering moves.

## What B adds

### Billing-cycle facts, through the seam

The provider seam gains a card-statement shape and one method to read it. Named for what is
taken — billing-cycle facts for a credit card — never for the provider product that happens to
carry them; a provider exposing the same facts under another name satisfies it unchanged. Each
field is independently unknown until reported, following the balance's precedent, because an
unknown must reach the sweep *as* an unknown.

Loan and APR detail are not in the shape and are never decoded. That is what keeps
[the narrowed non-goal](../../adr/0024-cash-flow-timeline-sweep.md) narrow rather than
reopened, and an Item exposing no supported credit account is an ordinary empty result rather
than a failure.

### Storage and refresh

Statement detail lives on the credit Account and refreshes inside the **existing** sync pass,
in the same per-connection attempt as balances and under the same fault isolation
([0021](../../adr/0021-fault-isolating-sync-pass.md)) — a statement read that fails leaves the
stored detail untouched and denies no other connection its refresh. It is fetched only for
connections holding at least one active credit Account, and inherits the same staleness stamp,
which is what keeps one staleness rule rather than growing a second.

### The payment schedule

A per-card user setting for *when* the card is paid, sitting with the other per-account user
facets. Sync never touches it. Its default is the model's one deliberately optimistic
assumption, already argued and accepted in
[0024](../../adr/0024-cash-flow-timeline-sweep.md) — not revisited here.

### A capability gap that is not a breakage

New in B, and the substance of [ADR-0026](../../adr/0026-statement-detail-is-an-enhancement.md).
A login that serves balances and transactions but will not serve statement detail is recorded
as a fact about the affected **cards**, not as a state of the **connection**, and the remedy is
offered where the consequence appears. This departs from
[0021](../../adr/0021-fault-isolating-sync-pass.md)'s reuse of needs-reconnect for a
user-actionable provider failure; the reasoning is in 0026.

## What B decides differently

**This cycle's unbilled spend leaves the timeline.** Chunk A reserved a card's whole balance;
B reserves only what was billed. The difference is real debt, but it has no due date inside the
horizon, and placing it would mean projecting the billing cycle that will bill it — which this
model does not do. **This is the one place the model holds back less than its predecessor**, and
the safety margin is the only headroom for a card whose next statement issues early in the
window. Recorded in [0026](../../adr/0026-statement-detail-is-an-enhancement.md) rather than
left as a surprise in the arithmetic.

## Settled by existing invariants, not re-decided

- **Nothing new blocks.** Every blocking reason is a missing *dollar value*; a missing date never
  blocks. So statement dates degrade, and a missing statement balance falls back to the current
  balance — which is already the no-statement path.
- **No second staleness rule.** Statement detail carries the balance's stamp, so a stale
  statement is already caught by the stale-card-balance reason.
- **The amount is capped at the current balance**, so a statement already paid releases without
  the model needing to observe the payment. Specified in chunk A; unchanged.

## Surfaces

- **`/sweep`** — unchanged in structure. A card's timeline row now carries a real date and a
  billed amount instead of landing at the run instant, so the derivation reads as the reason the
  figure moved.
- **The card's own row** — the payment schedule setting, and the statement-unavailable fact with
  its remedy, beside the account they describe rather than in the sweep.

## Testing

Chunk A shipped only the no-statement path: a card carries a label and a balance, and its two
tests cover the whole balance landing at the run instant and a card owing nothing contributing
no row. The payment-date resolution and the billed-amount cap listed in chunk A's testing notes
were **planned there, not built** — so the card arithmetic is new work in B, tested here:

- **Payment date** — both schedule modes; a resolved date already past landing at `now`; a
  resolved date beyond the horizon contributing nothing; either input unknown falling back to
  `now`.
- **Billed amount** — capped at the current balance, so a statement already paid releases
  without the model observing the payment; and unbilled spend (current above statement) staying
  off the timeline, which is the reserve reduction
  [0026](../../adr/0026-statement-detail-is-an-enhancement.md) accepts.
- **Still no statement** — chunk A's two cases keep passing unchanged.

And the path in, which is also new:

- **Provider client** — the credit array read; an Item with no supported credit account as an
  empty result rather than an error; loan and APR fields left undecoded.
- **Sync** — statement detail refreshed alongside balances; a failing statement read isolated to
  its connection, leaving stored detail intact and other connections refreshed; a cash-only
  connection making no such call.
- **Capability gap** — a login serving balances but not statements producing the per-card fact
  and a still-numeric, more conservative sweep, with the connection left active.
- **Payment schedule** — the setting surviving a sync that refreshes everything around it.
- **e2e** — a card whose statement falls after the next paycheck lowering the number against the
  same card with no statement detail; the statement-unavailable fact appearing on the card's row
  without a needs-reconnect badge.

## Deferred

- Any bulk re-consent campaign or consent-specific link mode — see `scope.md`; nothing has yet
  demonstrated either is needed.
- Occurrence matching, which is chunk C and unchanged by this work.

## What chunk B shipped

Recorded at the end of Implement, as this folder's last edit before it freezes. Where the
built thing differs from the design above, this is what is true.

- **A second provider-agnostic sentinel was needed.** The design said a login refusing statement
  detail becomes a per-card fact, but said nothing about *detecting* it. Detection is provider
  classification, so it follows the established shape: a sentinel on the seam, a registry in the
  client mapping native codes onto it, classified by what the user can act on
  ([0021](../../adr/0021-fault-isolating-sync-pass.md)). It is deliberately separate from the
  re-auth sentinel, because consumers must react differently — that difference is the whole
  point of [0026](../../adr/0026-statement-detail-is-an-enhancement.md).
- **The codes mapped onto it are unverified.** They are a reasoned guess at which provider codes
  mean "this login will not serve the product", made without the live confirmation the chunk-A
  spec asked for and this chunk's scope deliberately skipped. A wrong code here degrades safely
  — the read falls through to an ordinary sync error, and the card keeps its worst-case reading
  — but the per-card note would not appear when it should. **Confirm against a real login before
  trusting the note's absence as evidence that consent is fine.** The pre-merge audit narrowed
  the set: the code meaning "no supported credit account" was removed, because the seam documents
  that as an ordinary empty result rather than a failure; the code meaning the product is not
  enabled for the *client* was removed, because that is one operator-facing misconfiguration
  affecting every login and is not fixed by consent; and the OAuth re-consent code this work
  most expects to meet was added.
- **A product-criteria test was rewritten, then widened.** PC3 asserted the provider client
  contained no `/liabilities` string at all — a guard written when that was the non-goal. Money
  movement stays forbidden; the liabilities *detail* that is still out of scope is what the test
  now names. The pre-merge audit found the first rewrite too fine-grained — it named two literal
  field tags where the response carries far more loan detail than that — so the guard now
  inspects the **declared json tags** for the whole category (APR, interest, student, mortgage,
  origination, minimum payment) rather than scanning raw text. Reading tags rather than text also
  stops a doc-comment that *names* what is deliberately not decoded from tripping it. Verified by
  fault injection: decoding an `aprs` array as a nested struct — the case that defeated the
  narrow version — now fails the test.
- **Reconnect does not refresh statements**, though the design did not say either way. A failed
  statement read there would block clearing the needs-reconnect badge on a login that has just
  proved it works.
- **The payment-schedule offset is cleared when the mode does not read it**, so a stored row never
  implies a rule it does not follow.
- **The sweep's card arithmetic was built here, not in chunk A.** Chunk A's testing notes listed
  the payment-date resolution and the billed cap as covered; they were planned, not shipped. Both
  are implemented and tested in this chunk.
- **The audit found two correctness bugs, both fixed here.** A statement billing a negative figure
  (a card sitting in credit at statement time) was placed as an outflow with a negative amount,
  which the evaluator read as a *reduction* — an inflow the model invented, in a model where every
  degradation must err conservative. And a statement the bank stopped reporting was never cleared:
  because it shares the balance's staleness stamp, which every pass refreshes, no rule would ever
  have caught it, and the stale figure caps the card's contribution *below* its balance.
- **The remedy was missing and is now built.** ADR-0026 and `scope.md` both put the remedy beside
  the note; the first implementation shipped only the note. A re-authorize control now sits with
  it, driving the same update-mode relink as reconnect but named and worded apart from it —
  calling it "Reconnect" would tell the user something false about a login that is working.
