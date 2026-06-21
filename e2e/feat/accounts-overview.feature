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
