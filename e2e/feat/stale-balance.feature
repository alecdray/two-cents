Feature: Stale balance

  A connection can stop syncing without its login having expired, so the bank is
  never flagged needs-reconnect and its balances quietly stop updating. The
  overview marks an account whose balance has gone too long without a refresh, so
  an old figure is not mistaken for a current one. A recently refreshed account
  carries no such mark, and a bank already flagged needs-reconnect shows only the
  reconnect badge — never both explanations at once.

  Scenario: An account that stopped refreshing is marked stale
    Given a linked bank whose balance last refreshed days ago
    Then the account shows the stale-balance mark

  Scenario: A recently refreshed account is not marked stale
    Given a linked bank whose balance refreshed within the threshold
    Then the account shows no stale-balance mark

  Scenario: A needs-reconnect bank shows only the reconnect badge
    Given a stale account whose connection needs reconnecting
    Then the account shows the needs-reconnect badge and no stale-balance mark
