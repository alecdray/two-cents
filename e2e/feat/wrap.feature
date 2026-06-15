Feature: Month wrap

  The Wraps section lists past and present months, each linking to that month's
  wrap — a retrospective scorecard of actuals only, never compared against a
  budget. A month's wrap shows its net income, the savings contributed, and the
  spend broken down by Category, with a settling badge while any transaction is
  still pending and a partial badge when the month sits at the backfill edge.

  Scenario: The wraps list shows the current month and links to its wrap
    Given a bank linked from the accounts overview with its transactions listed
    When the wraps list is opened
    Then the current month is listed
    And opening its wrap shows the net income, savings contributed, and spend by category
    And the wrap is marked settling because a transaction is still pending
    And the wrap is marked partial because the month is the connect month
