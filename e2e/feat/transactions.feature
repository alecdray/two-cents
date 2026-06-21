Feature: Transactions

  The transactions page at `/transactions` shows the newest-first activity pulled
  from every connected bank, grouped under month-section headers. Connecting a bank
  backfills its transactions, so they are readable immediately by navigating from the
  overview. Each row shows its account name and a display-signed amount — spending
  negative, income positive — with a marker on transactions the bank has not yet
  posted. A merchant search box narrows the list across the full history, and a
  Needs-attention view filters to the transactions the system left for the user to
  resolve (unresolved inflows, uncategorized spending, unknown-destination transfers);
  resolving one from that view drops it from the worklist. Re-syncing the same bank
  state never duplicates rows. With no bank connected the page prompts the user to
  connect one; with a bank connected but nothing pulled yet it offers to sync.

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

  Scenario: Transactions are grouped under month headers
    Given a bank linked from the overview with its transactions listed
    Then the rows sit under a month-section header for their transaction month

  Scenario: Searching by merchant filters the list to the matching transactions
    Given a bank linked from the overview with its transactions listed
    When a merchant is typed into the search box
    Then only the transactions whose merchant matches remain

  Scenario: The needs-attention view shows only transactions needing attention
    Given a bank linked from the overview with its transactions listed
    When the Needs-attention view is selected
    Then only the transactions needing attention are listed

  Scenario: Resolving a transaction in the needs-attention view drops it from the worklist
    Given the Needs-attention view listing the one transaction that needs attention
    When that transaction is re-categorized to a resolved bucket
    Then it drops from the worklist, which shows the nothing-needs-attention empty state
