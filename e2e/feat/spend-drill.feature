Feature: Spend drill-down

  From a spend-by-Category figure — on a month wrap or on the current-month
  Tracker — the user drills into the transactions that make it up, to verify and
  investigate. The drilled list reconciles to the figure it was reached from. A
  row can be re-categorized in place; doing so re-renders the list and net total,
  so a row the edit moves out of the bucket drops and the total stays honest. The
  Tracker's "everything else" residual drills into the unbudgeted-plus-
  uncategorized spend it represents.

  Scenario: Drilling a wrap category lists the transactions making up its total
    Given a bank linked from the accounts overview with its transactions listed
    When a Category figure on the current month's wrap is opened
    Then the drilled list shows that Category's transactions and a net total equal to the figure

  Scenario: Re-categorizing a drilled transaction out of the bucket updates the list and net total
    Given a bank linked from the accounts overview with its transactions listed
    When the only transaction in a wrap Category's drill is re-categorized as Income
    Then it leaves the bucket, the list empties, and the net total drops to zero

  Scenario: Drilling the Tracker's everything else lists the residual spend
    Given a budget that leaves one spent Category unbudgeted
    When the Tracker's everything-else line is opened
    Then the drilled list shows the unbudgeted spend and a net total equal to the everything-else figure
