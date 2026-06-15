Feature: Transaction categorization

  Every synced transaction lands with a classification (Income / Spending /
  Transfer / needs-review) and, when Spending, a Category — derived automatically
  from the bank category, the user's Rules, and the amount direction. The user can
  correct any transaction's categorization from the transactions page, and that
  manual choice is sticky: it survives a later sync. Custom Categories can be
  created and archived alongside the built-ins, and creating a Rule re-categorizes
  the matching transactions immediately, reporting how many changed.

  Scenario: Synced transactions are auto-categorized with chips including a transfer and a needs-review row
    Given a bank linked from the overview
    When the transactions page is opened from the navbar
    Then each transaction shows a classification chip
    And the transfer-signal transaction is classified Transfer
    And the unclassifiable inflow is flagged needs-review

  Scenario: A manual re-categorization survives a later sync
    Given a bank linked from the overview with its transactions listed
    When a spending transaction is re-categorized as Transfer
    Then its chip changes to Transfer
    And the change survives a Sync-now

  Scenario: A custom category can be created and archived
    Given the categories page is open
    When a custom category is created
    Then it appears among the active categories
    And archiving it moves it to the archived categories

  Scenario: Creating a rule re-categorizes a matching transaction and reports the count
    Given a bank linked from the overview with a needs-review transaction
    When a rule matching that transaction's merchant is created on the rules page
    Then the rules page reports one transaction re-categorized
    And the matching transaction is re-categorized on the transactions page
