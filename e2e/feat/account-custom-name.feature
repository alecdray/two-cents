Feature: Rename an account with a custom name

  From the accounts overview, the user gives an account a name of their own. The
  custom name overrides the bank-reported name everywhere the account is shown —
  the overview rows and the transaction editor's account picker — while the mask
  still disambiguates same-named accounts. Clearing the name reverts the account
  to the bank's name. Renaming applies in place, swapping the overview region.

  Scenario: Renaming an account shows the custom name on the overview
    Given a populated overview with a cash account
    When the user renames that account
    Then the row shows the custom name in place of the bank name

  Scenario: Clearing a custom name reverts to the bank name
    Given a populated overview with a renamed cash account
    When the user clears the account's custom name
    Then the row shows the bank name again

  Scenario: A renamed account shows its custom name in the transaction editor
    Given a transfer and a renamed savings account
    When the user opens the transfer's destination account picker
    Then the picker offers the account by its custom name
