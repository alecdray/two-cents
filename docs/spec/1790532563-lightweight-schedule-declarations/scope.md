# Lightweight schedule declarations

The sweep schedule today asks for more than it needs to, and shows more than a
quick check wants. This chunk lowers the cost of declaring a scheduled item and
the cost of reading the schedule, without touching how occurrence matching
actually decides a match ([ADR-0027](../../adr/0027-occurrence-matching-reconciles-the-schedule.md)).

**In scope:**
- Create a scheduled item directly from a transaction — prefilled and matched
  in the same action, so the schedule is accurate from the item's first
  occurrence rather than its second.
- A **monthly** scheduled item may declare no date at all, for a bill with no
  fixed pay date; its occurrence stays open for the whole month until matched.
  Biweekly stays dated-only, so its occurrence projection stays a pure
  function of the item with no dependency on match history.
- Any scheduled item's amount may resolve live from its own matched
  transaction history (trailing three months) instead of staying a typed
  figure — computed on every read, never cached, and never silently
  overwriting a figure the user set by hand.
- The schedule's `/sweep` surface moves from always-expanded inline forms to a
  compact list (built for a quick read) with a modal editor (built for the
  occasional edit) — reusing the existing shared modal shell and the
  cross-modal-return pattern already established for the Rule editor
  ([ADR-0011](../../adr/0011-reusable-transaction-editing-modal.md),
  [ADR-0016](../../adr/0016-rule-editor-modal-and-cross-modal-return.md)).
- The transaction editor gains a "add to schedule" action, offered only on a
  checking-account transaction, and a badge naming the item a transaction is
  matched to.

**Out of scope:**
- Any change to how automatic resolution decides a match — the learned
  merchant, the amount tests, the ambiguity rule are untouched
  ([ADR-0027](../../adr/0027-occurrence-matching-reconciles-the-schedule.md)).
- A declared merchant/amount-range/arbitrary-cadence matching-criteria system —
  considered and rejected in favor of the above, since the learned-merchant
  mechanism already does this job once an item has one real match.
- Cross-item collision detection — not needed here, since matching criteria
  themselves are unchanged.

See [ADR-0031](../../adr/0031-scheduled-items-bootstrap-and-resolve-from-matches.md)
for the decision record, and the updated `schedule`, `sweep`, and `transactions`
module docs for current-state behaviour.
