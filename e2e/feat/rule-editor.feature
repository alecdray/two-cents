Feature: Rule editor modal

  Creating and editing a Rule on the Rules page happens in one shared editor modal,
  opened from the New rule button and each row's Edit button. A save closes the modal
  and refreshes the read-only list with how many transactions were re-categorized;
  delete removes a rule from its row or from inside the edit modal; a recoverable
  validation error keeps the modal open.

  Scenario: Create a rule through the editor modal
    Given the Rules page with no rules and an uncategorized Starbucks transaction
    When I open the New rule modal, fill in a Starbucks spending rule, and save
    Then the modal closes and the list shows the new rule with a re-categorized message

  Scenario: Edit a rule through the editor modal
    Given the Rules page with an existing rule
    When I open that rule's Edit modal, change its merchant text, and save
    Then the modal closes and the list shows the rule's new merchant text

  Scenario: Delete a rule
    Given the Rules page with an existing rule
    When I delete that rule
    Then the row is removed and the empty state is shown

  Scenario: Delete a rule from the edit modal
    Given the Rules page with an existing rule
    When I open that rule's Edit modal and delete it from there
    Then the modal closes and the empty state is shown

  Scenario: A rule with a blank merchant keeps the editor open with an error
    Given the Rules page with no rules
    When I open the New rule modal and save without entering merchant text
    Then the modal stays open showing an inline error
