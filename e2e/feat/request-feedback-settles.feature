Feature: Request feedback settles after real flows

  The app-wide top progress bar is driven by the aggregate HTMX request
  lifecycle: it shows while work is in flight and clears once everything settles.
  This holds across the real product flows, not just synthetic requests — after
  any genuine interaction finishes, the bar returns to hidden and is never left
  visible while the page sits idle. A flow that fans out into several requests
  keeps the bar up until the last of them settles.

  Scenario: After a manual sync, the progress bar is hidden
    Given a bank linked from the overview with its transactions listed
    When the user runs Sync now and the refreshed list settles
    Then the progress bar is hidden

  Scenario: After a boosted navigation, the progress bar is hidden
    Given the app has loaded and settled
    When the user navigates to another page and it settles
    Then the progress bar is hidden

  Scenario: After editing a transaction through the modal, the progress bar is hidden
    Given a bank linked from the overview with its transactions listed
    When the user changes a transaction in the editor and saves, fanning out a refresh
    Then the progress bar is hidden once every resulting request has settled
