Feature: Sync-now feedback

  The "Sync now" control gives its own action-result feedback (ADR-0015),
  distinct from the app-wide request-progress bar. While its request is in
  flight the control is disabled and reads as working, so a second activation is
  ignored; once the sync settles the control returns to its normal interactive
  state. After a successful sync a brief confirmation appears in the same inline
  slot the sync error uses and then clears itself, without reloading or
  displacing the transaction list.

  Scenario: The Sync-now control is disabled and working while a sync is in flight
    Given a connected bank's transactions are listed
    When a sync is started and its request is held in flight
    Then the Sync-now control is disabled until the sync settles, then interactive again

  Scenario: A successful sync shows a transient confirmation that then clears itself
    Given a connected bank's transactions are listed
    When the bank is synced
    Then a confirmation appears beside the control and auto-clears, leaving the list in place
