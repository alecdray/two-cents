Feature: Month wrap

  A past month's wrap is a retrospective scorecard of actuals only, never compared
  against a budget: it shows net income, gross income, savings contributed,
  surplus, the spend broken down by Category, and an inline list of the month's
  whole transaction set, with a settling badge while any transaction is still
  pending and a partial badge when the month sits at the backfill edge. The current
  month has no wrap of its own — its wrap address redirects to the Tracker at `/` —
  so a real wrap lives on an earlier month, reached from the month rail. Editing a
  transaction from the wrap's list refreshes the wrap's figures, since an edit can
  shift any of them.

  Scenario: A prior month's wrap shows its figures and sits on the month rail
    Given a prior month seeded with a fully-classified transaction set
    When that month's wrap is reached from the month rail's earlier chip
    Then it shows the net income, gross income, savings contributed, surplus, and spend by category
    And the wrap lists every transaction in the month
    And the wrap is marked settling because a transaction is still pending
    And the wrap is marked partial because the month sits at the backfill edge
    And the rail's active chip is that month, alongside a current-month chip linking to the Tracker

  Scenario: Visiting the current month's wrap redirects to the Tracker
    Given a prior month seeded with a fully-classified transaction set
    When the current month's wrap address is visited
    Then the Tracker home page is shown

  Scenario: Editing a transaction from the wrap list refreshes the wrap's figures
    Given a prior month seeded with a fully-classified transaction set
    When a needs-review inflow is re-categorized to Income from the wrap's list
    Then the wrap's gross income figure rises by that amount
