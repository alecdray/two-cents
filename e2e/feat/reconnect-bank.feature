Feature: Reconnect a bank

  A linked bank whose login has expired is flagged needs-reconnect on the
  overview, showing a badge and a reconnect control on its accounts.
  Reconnecting re-authenticates the login and refreshes the bank's accounts; on
  success the badge clears in place, leaving the accounts intact.

  Scenario: Reconnecting a needs-reconnect bank clears the badge
    Given a linked bank flagged needs-reconnect
    When the user reconnects the bank
    Then the needs-reconnect badge clears and the accounts remain
