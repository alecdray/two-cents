Feature: Reconciling the schedule against what actually happened

  The sweep's timeline reaches back one cadence interval, so a declared
  occurrence that already fell due is held back until something says it was
  satisfied. Pointing it at the transaction that paid it releases the money;
  saying nothing paid it — or rejecting a match the app made — holds it back
  again. Every way of not holding a good match reserves more, never less.

  Scenario: Matching the occurrence that already fell due releases the money
    Given a declared bill whose last occurrence is unmatched
    When it is matched to the checking transaction that paid it
    Then the next run no longer holds that occurrence back

  Scenario: Rejecting a match hands the occurrence back to the timeline
    Given a bill whose last occurrence the sweep has released as settled
    When the match is rejected
    Then the next run holds the money back again
