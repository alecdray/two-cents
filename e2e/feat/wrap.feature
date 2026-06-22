Feature: Month wrap

  The Wraps section lists past and present months, each linking to that month's
  wrap — a retrospective scorecard of actuals only, never compared against a
  budget. A month's wrap shows its net income, gross income, the savings
  contributed, the spend broken down by Category, and an inline list of the month's
  whole transaction set, with a settling badge while any transaction is still
  pending and a partial badge when the month sits at the backfill edge. Editing a
  transaction from that list refreshes the wrap's figures, since an edit can shift
  any of them.

  Scenario: The wraps list shows the current month and links to its wrap
    Given a bank linked from the accounts overview with its transactions listed
    When the wraps list is opened
    Then the current month is listed
    And opening its wrap shows the net income, gross income, savings contributed, and spend by category
    And the wrap lists every transaction in the month
    And the wrap is marked settling because a transaction is still pending
    And the wrap is marked partial because the month is the connect month

  Scenario: Editing a transaction from the wrap list refreshes the wrap's figures
    Given a bank linked from the accounts overview with its transactions listed
    When a needs-review inflow is re-categorized to Income from the wrap's list
    Then the wrap's gross income figure rises by that amount
