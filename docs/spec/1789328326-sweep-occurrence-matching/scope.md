# Scope — occurrence matching (chunk C)

Chunk C of the cash-flow timeline sweep. The model, its rationale and the slice boundary are
[ADR-0024](../../adr/0024-cash-flow-timeline-sweep.md) and the chunk-A record
([`../1789161803-sweep-reserve-foundation/`](../1789161803-sweep-reserve-foundation/)) — this
folder does not restate them.

Chunks A and B ship a timeline whose occurrence window **looks only forward**: an occurrence
that already fell due is left off entirely, because nothing can tell a paid one from an unpaid
one and placing both last month's and this month's would reserve every declared bill twice for
the whole month. C supplies what was missing — a record binding an occurrence to the real
transaction that satisfied it — so a settled occurrence drops out before it reaches the
timeline, and the window can finally reach back over the occurrences that did not.

The number moves in one direction as a result: **up**, and only in the unmatched-past case. A
bill that fell due and was paid looks exactly as it does today (its occurrence is matched and
placed nowhere); a bill that fell due and was *not* paid is now held back instead of silently
dropped. This is the opposite of chunk B's one concession, and it restores the model's rule
that every unknown makes the answer more conservative.

## Boundary

**In:** the occurrence match record and its storage; manual association, correction and
clearing from `/sweep`; best-effort automatic resolution as a step in the existing sync pass,
reached through an injected seam; the one-cadence lookback the match record makes safe; and
the timeline event's record of what it was originally due on, so a row placed at the run
instant explains why.

**In, departing from the recorded chunk boundary:** the card's arithmetic. Cards carry no
occurrence match — the bank reports their payments directly, so a match record would re-derive a
fact already in hand — but the grilling pass found the `min(statement balance, current balance)`
cap to be a proxy that both leaks unbilled spend and barely releases a paid statement on a card in
active use. It is replaced by the unpaid statement ([ADR-0028](../../adr/0028-a-card-reserves-its-unpaid-statement.md)).

[`chunks.md`](../1789161803-sweep-reserve-foundation/chunks.md) says "cards meanwhile: unchanged"
for chunk C, and that folder is frozen, so the departure is recorded here instead: the boundary
was drawn on the assumption that cards were *served* by the cap, which they are not. The two
changes stay separate decisions in separate ADRs even though they ship on one branch.

**Out, deferred deliberately:** declaration drift (comparing the declared amount against what
actually landed, so a rent increase surfaces instead of rotting). ADR-0024 already names it "a
consequence worth noting, not building yet"; the match record is its prerequisite, not its
delivery.

## Decisions taken

Recorded in [ADR-0027](../../adr/0027-occurrence-matching-reconciles-the-schedule.md); the
reasoning is there, not here.

- The occurrence window **reaches back exactly one cadence interval**, reversing the
  forward-only rule ADR-0024 recorded. One interval, and not more, is what keeps an occurrence
  that will never be matched a finite, visible error rather than an unbounded one.
- **Automatic resolution is tuned for precision, not coverage.** A missed match over-reserves
  and a false match under-reserves, and under-reserving is the one direction this model is not
  allowed to go — so an ambiguous candidate set declines to match rather than guessing.
- **Clearing a match is a stored decision, not the absence of one**, or the next sync would
  re-make the match the user just rejected.
- A match whose transaction no longer exists is **dropped, manual or not**, and its occurrence
  returns to the timeline — so every way of not holding a good match reserves more.
- `sweep` learns *which occurrences are settled*, never *what settled them*: it stays free of
  the ledger, and `TestSweepReadsNeitherBudgetNorLedger` stands unchanged.
- **There is no "settled, but nothing to point at".** The schedule is checking activity by
  definition, so a genuinely settled occurrence always has a row to point at; a missing one means
  the declaration is wrong or the bill went unpaid, and neither should be silenced. Manual
  association is therefore restricted to **checking** transactions, which makes the invariant
  enforced rather than merely asserted.
- **A learned merchant is evidence only while it is distinctive.** A descriptor two declared items
  have both learned identifies neither.
- **The checking *identification* moves to `accounts`**, which owns the flag it turns on; `sweep`
  keeps the three needs-attention reasons, which are its own. Matching being checking-only is what
  forces the question, and a second copy of the rule in the composition root is what it avoids.

Recorded in [ADR-0028](../../adr/0028-a-card-reserves-its-unpaid-statement.md):

- A card reserves its **unpaid statement**, bounded by its current balance. The balance stops
  being a payment record and becomes only a ceiling on what the card can claim; for a card with no
  statement at all it remains the whole obligation, under the missing-date rule.

## Carried assumption, unconfirmed

The card fix reads two billing-cycle facts — the last payment's amount and date — that no live
login has been shown to populate, because the deploy that chunk B also waited on still has not
happened. This is not the same gamble chunk B took: the arithmetic degrades to reserving the full
statement whenever the payment facts are absent, which is the behaviour the card would have had
anyway, so an issuer that reports nothing loses the improvement rather than getting a wrong number.
Confirming them against a live login is still owed, and their absence must not be read as evidence
the issuers do not support them.

## Incidental correction

Chunk B reconciled the domain glossary and `sweep`'s Boundaries section, but **not** the two
places that describe the card's path onto the timeline: the [domain derivation
card](../../domain/README.md) and `sweep`'s `README.md` §What reaches the timeline. Both still
said every card contributes its whole balance at the run instant, which B superseded. This
branch rewrites both blocks anyway for the occurrence rules, so it brings the card lines up to
what B shipped in the same edit — and the correction is true of `main` today, so it lands
before any of chunk C's own behaviour does.
