Feature: Current-month Tracker

  The application's landing page at `/` shows this month's Tracker: how the
  month's spending, income, and savings stand against the rolling budget. Each
  budgeted Category shows what is remaining, the pace to hold the line, and a bar
  showing the share of its budget used, with an over-budget marker when its
  spending exceeds its limit; an everything-else line covers unbudgeted spend; and
  income and savings progress show movement
  toward their targets (savings reflects the auto-paired contribution). With no
  budget set, the page shows actuals only and prompts the user to create one.

  Scenario: A budget set against the month's activity shows remaining, pace, progress, and an over-budget category
    Given a bank linked from the accounts overview with its transactions listed
    And a budget whose income and savings targets and category limits are set, one limit below its spending
    When the Tracker home page is opened
    Then each budgeted category shows its remaining and pace
    And the category whose spending exceeds its limit is flagged over budget
    And the everything-else line and total remaining are shown
    And every category, everything-else, and the total row shows a budget-used bar
    And income progress and savings progress reflect the month's income and the paired savings contribution
    And the month's surplus is shown
    And the month rail shows the current month as the active chip linking to the Tracker

  Scenario: With no budget set the Tracker prompts to create one
    Given no budget is set
    When the Tracker home page is opened
    Then the needs-budget prompt is shown
    And the month's surplus and the month rail's current-month chip are shown
