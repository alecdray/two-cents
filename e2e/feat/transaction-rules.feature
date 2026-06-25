Feature: Rule-aware transaction editor

  The transaction-editing modal surfaces the Rules governing a transaction ([ADR-0016]).
  When one or more Rules match the row's merchant it lists them, governing Rule first and
  marked Applied, each opening the shared rule editor modal in edit mode; when none match
  it offers a Create-rule control prefilled from the transaction. The transaction's own
  edit URL rides along as the return handle, so saving or dismissing the rule modal
  re-mounts the transaction editor refreshed rather than landing on the bare page.

  Scenario: Editing a transaction lists the governing rule and opening it returns on save
    Given a transaction whose merchant matches a seeded rule
    When I open its editor, open the governing rule, and save the rule
    Then the rule modal opens prefilled and saving returns to the transaction editor still listing the rule

  Scenario: A transaction with no matching rule offers Create rule, which returns on save
    Given a transaction no rule matches
    When I open its editor, open the Create-rule modal prefilled, and save a new rule
    Then the new rule is created and I return to the transaction editor now listing it

  Scenario: Dismissing the rule editor returns to the transaction editor
    Given a transaction whose merchant matches a seeded rule
    When I open its editor, open the governing rule, and dismiss the rule modal
    Then I return to the transaction editor rather than the bare page
