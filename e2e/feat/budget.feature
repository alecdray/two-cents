Feature: Budget editor

  The user sets a single rolling monthly budget — an income target, a savings
  target, and an optional spending limit per Category. The page computes the
  "everything else" residual left after the limits and savings, and shows whether
  the plan is balanced or over-allocated. The plan persists: a reload reads back
  the saved targets and limits. An over-allocated plan (limits plus savings
  exceeding income) is surfaced but never blocked — it still saves.

  Scenario: A budget is set, shows its residual and a balanced banner, and persists across a reload
    Given the budget page is open
    When an income, a savings target, and a category limit are saved
    Then the everything-else residual reflects the plan
    And the balance banner reads balanced
    And reloading the page shows the saved values

  Scenario: An over-allocated plan still saves and shows the over-allocated banner
    Given the budget page is open
    When the limits and savings are set to exceed the income and saved
    Then the balance banner reads over-allocated
    And reloading the page shows the over-allocated plan persisted
