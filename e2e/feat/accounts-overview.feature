Feature: Accounts Overview

  The accounts overview page at `/accounts`. It shows the net cash position derived
  from the user's linked accounts — total spendable cash minus total credit
  debt — alongside the accounts grouped into cash, credit, and other buckets.
  Accounts in the other bucket and accounts whose balance the bank has not
  reported are shown but excluded from the position. When no accounts are
  linked, a friendly empty state stands in for zeroed-out totals.

  Scenario: Seeded overview
    Given a reset DB seeded with mixed cash, credit, and other accounts
    When the overview page at /accounts is loaded
    Then the net cash, total cash, and total credit debt match the seeded cash minus credit
    And the cash and credit groups render their account rows
    And the other section is shown and labelled as excluded from net cash
    And the unknown-balance account shows an em dash rather than $0
    And the credit account on the needs-reconnect connection shows the reconnect badge

  Scenario: Empty state
    Given a reset DB with no accounts
    When the overview page at /accounts is loaded
    Then the empty state is shown and no totals chrome is rendered
