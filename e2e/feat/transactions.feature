Feature: Transactions

  The transactions page at `/transactions` shows a flat, newest-first list of the
  activity pulled from every connected bank. Connecting a bank backfills its
  transactions, so they are readable immediately by navigating from the overview.
  Each row shows its account name and a display-signed amount — spending negative,
  income positive — with a marker on transactions the bank has not yet posted.
  Re-syncing the same bank state never duplicates rows. With no bank connected the
  page prompts the user to connect one; with a bank connected but nothing pulled
  yet it offers to sync.

  Scenario: A connected bank's transactions appear with account names, signed amounts, and a pending marker
    Given a bank linked from the overview
    When the transactions page is opened from the navbar
    Then the bank's transactions are listed with their account names
    And spending shows negative, income shows positive, and the unposted charge is marked pending

  Scenario: Re-syncing a connected bank does not duplicate its transactions
    Given a bank linked from the overview with its transactions listed
    When the list is synced again
    Then the same transactions remain with no duplicates

  Scenario: The page prompts to connect a bank when none is connected
    Given no bank is connected
    When the transactions page is opened
    Then it shows the connect-a-bank prompt and neither the list nor the sync control

  Scenario: A connected bank with nothing synced offers to sync
    Given a bank is connected but no transactions have been synced
    When the transactions page is opened
    Then it shows the nothing-synced prompt with the sync control and no list
