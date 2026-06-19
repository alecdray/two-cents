Feature: Override an account's kind and counts-as-savings

  From the accounts overview, the user corrects how an account is bucketed: its
  spending kind (cash / credit / other) and, on non-credit accounts, whether it
  counts as savings. Each override applies on change and swaps the overview in
  place — a kind change re-buckets the row and recomputes net cash. The
  counts-as-savings toggle is offered on cash and other rows but never on credit,
  where a transfer in is a card payment, not saving.

  Scenario: Overriding an account's kind re-buckets it and updates net cash
    Given a populated overview with a cash account and a credit card
    When the user changes the cash account's kind to other
    Then the account moves to the other group and net cash drops by its balance

  Scenario: Turning on counts-as-savings reflects on the row immediately
    Given a populated overview with a cash account that is not counted as savings
    When the user turns on counts-as-savings for that account
    Then the account's counts-as-savings toggle shows on

  Scenario: Overriding a savings account to credit drops its savings toggle
    Given a populated overview with a savings account counted as savings
    When the user changes that account's kind to credit
    Then the account moves to the credit group and its savings toggle is gone
