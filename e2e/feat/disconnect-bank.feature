Feature: Disconnect a bank

  From the accounts overview, the user removes a linked bank. Because
  disconnecting is irreversible — it deletes the bank's accounts from Two Cents —
  the action requires an explicit confirmation before it fires. Cancelling the
  confirmation removes nothing; confirming severs the bank and swaps the updated
  overview in place.

  Scenario: Cancelling the confirmation keeps the bank
    Given a populated overview with a linked bank
    When the user opens the disconnect confirmation and cancels it
    Then nothing is removed and the bank's accounts remain

  Scenario: Confirming the disconnect removes the bank
    Given a populated overview with a linked bank
    When the user opens the disconnect confirmation and confirms it
    Then the bank's accounts are gone and the overview returns to the empty state
