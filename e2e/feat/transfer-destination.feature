Feature: Transfer destination

  Every outflow Transfer resolves a destination and a subtype on sync: a transfer
  into an account that counts as savings is a Savings contribution; a transfer
  with no matching leg is left destination-unknown for the user to mark. From the
  transactions page the user can see each transfer's resolved state as a chip and
  mark or correct an unknown one — and that manual choice is sticky: it survives a
  later sync. Non-transfer rows offer no transfer-destination control.

  Scenario: An auto-paired savings transfer shows a savings contribution chip
    Given a bank linked from the overview with its transactions listed
    Then the savings transfer row shows a savings contribution to the savings account

  Scenario: Marking an unknown transfer's destination sticks across a sync
    Given a bank linked from the overview with an unpaired outflow transfer
    When the unknown transfer is marked as a savings contribution
    Then its chip changes to a savings contribution
    And the marked destination survives a Sync-now

  Scenario: A non-transfer row offers no transfer-destination control
    Given a bank linked from the overview with its transactions listed
    Then a spending row exposes no transfer-destination control
