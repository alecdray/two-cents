Feature: Connect a bank

  From the accounts overview at `/accounts`, the user links a bank and sees its accounts
  appear in place. The connect control is the empty state's primary call to
  action and a persistent affordance once accounts are linked. The response swaps
  the overview region without a full-page reload, so the freshly linked accounts
  and the updated net cash position render immediately.

  Scenario: Linking a bank from the empty overview reveals its accounts
    Given a reset DB with no accounts
    When the overview page at /accounts is loaded
    Then the empty state and the connect control are shown
    When the connect control is used to link the bank
    Then the linked bank's cash and credit accounts appear in their groups
    And the net cash, total cash, and total credit debt reflect the linked accounts
