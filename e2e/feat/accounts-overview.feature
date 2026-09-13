Feature: Accounts Overview

  The accounts overview page at `/accounts`. It headlines free cash — spendable
  cash with earmarked savings set aside — alongside the supporting net cash,
  total savings, total cash, and total credit debt figures, and the accounts
  grouped into cash, credit, and other buckets. Accounts in the other bucket and
  accounts whose balance the bank has not reported are shown but excluded from
  the position. An account can be hidden — dropped from the totals and the
  pickers into a separate Hidden section — without removing its bank, and
  unhidden again. When no accounts are linked, a friendly empty state stands in
  for zeroed-out totals.

  Scenario: Seeded overview
    Given a reset DB seeded with mixed cash, credit, and other accounts
    When the overview page at /accounts is loaded
    Then the free cash, net cash, total savings, total cash, and total credit debt match the seeded figures
    And the cash and credit groups render their account rows
    And the other section is shown and labelled as excluded from net cash
    And the unknown-balance account shows an em dash rather than $0
    And the credit account on the needs-reconnect connection shows the reconnect badge

  Scenario: Hide and unhide an account
    Given a reset DB seeded with mixed cash, credit, and other accounts
    When the overview page at /accounts is loaded
    And a cash account is hidden
    Then it moves into the Hidden section and the totals drop its balance
    When the hidden account is unhidden
    Then it returns to the cash group and the totals include its balance again

  Scenario: Empty state
    Given a reset DB with no accounts
    When the overview page at /accounts is loaded
    Then the empty state is shown and no totals chrome is rendered

  Scenario: A card shows what its statement takes and when
    Given a credit account whose bank reported a statement
    When the overview page at /accounts is loaded
    Then the card shows the billed amount and its payment date

  Scenario: Choosing when a card is paid
    Given a credit account paid on its due date
    When the payment schedule is switched to a fixed number of days after the statement
    Then the offset is kept and the card re-dates its payment

  Scenario: A bank that will not share statement detail says so on the card
    Given a credit account whose login serves no statement detail
    When the overview page at /accounts is loaded
    Then the card explains the gap, offers to ask the bank again, and is not marked broken
