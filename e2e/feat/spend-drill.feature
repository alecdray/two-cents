Feature: Spend drill-down

  From a figure — a spend-by-Category row, the gross Income or Savings figure on a
  month wrap, or the income/savings/everything-else figures on the current-month
  Tracker — the user drills into the transactions that make it up, to verify and
  investigate. The drilled list reconciles to the figure it was reached from. A row
  can be re-categorized through the shared editing modal; the drill region then
  self-refreshes, so a row the edit moves out of the bucket drops and the total
  stays honest. The income and savings figures read no budget and drill for any
  month; the Tracker's "everything else" residual drills into the
  unbudgeted-plus-uncategorized spend it represents.

  Scenario: Drilling a wrap category lists the transactions making up its total
    Given a bank linked from the accounts overview with its transactions listed
    When a Category figure on the current month's wrap is opened
    Then the drilled list shows that Category's transactions and a net total equal to the figure

  Scenario: Drilling the wrap's Income figure lists the month's income
    Given a bank linked from the accounts overview with its transactions listed
    When the Income figure on the current month's wrap is opened
    Then the drilled list shows the month's income and a net total equal to gross income

  Scenario: Drilling the wrap's Savings figure lists the savings contributions
    Given a bank linked from the accounts overview with its transactions listed
    When the Savings figure on the current month's wrap is opened
    Then the drilled list shows the savings contributions and a net total equal to the figure

  Scenario: Re-categorizing a drilled transaction out of the bucket updates the list and net total
    Given a bank linked from the accounts overview with its transactions listed
    When the only transaction in a wrap Category's drill is re-categorized as Income
    Then it leaves the bucket, the list empties, and the net total drops to zero

  Scenario: Drilling the Tracker's everything else lists the residual spend
    Given a budget that leaves one spent Category unbudgeted
    When the Tracker's everything-else line is opened
    Then the drilled list shows the unbudgeted spend and a net total equal to the everything-else figure
