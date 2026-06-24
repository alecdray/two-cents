Feature: Request progress indicator

  A thin, indeterminate progress bar pinned to the top edge of the viewport
  signals that the app is waiting on the server. It is app-wide chrome mounted
  once in the shared layout and driven entirely by the HTMX request lifecycle:
  it appears while at least one request is in flight and hides once the
  outstanding requests settle — whether they succeed, error, or are aborted.
  With several requests overlapping it stays up until the last one finishes.
  Exactly one bar exists on every page, and it survives the body morph of a
  boosted navigation without duplicating or sticking. It overlays the top edge
  above the page content without displacing it or colliding with the fixed
  bottom navigation.

  Scenario: No progress bar is shown while the app is idle
    Given the app has loaded and settled
    Then no progress bar is visible

  Scenario: Starting a request reveals the progress bar at the top of the page
    Given the app has loaded
    When a navigation request is in flight
    Then the progress bar becomes visible

  Scenario: The progress bar hides after a request succeeds
    Given a navigation request is in flight and the bar is visible
    When the request completes successfully
    Then the progress bar is hidden again

  Scenario: The progress bar hides after a request gets an error response
    Given a request is in flight and the bar is visible
    When the server answers it with an error status
    Then the progress bar is hidden again

  Scenario: The progress bar hides after a request is aborted
    Given a navigation request is in flight and the bar is visible
    When the request is aborted
    Then the progress bar is hidden again

  Scenario: The progress bar stays visible until the last of several overlapping requests settles
    Given two requests are in flight at once
    When the shorter one settles
    Then the progress bar stays visible until the longer one settles too

  Scenario: Every page shows exactly one progress bar that survives boosted navigation
    Given the app has loaded
    When the user navigates between pages
    Then exactly one progress bar exists throughout and its driver still toggles it

  Scenario: The progress bar overlays the top edge without colliding with the bottom navigation
    Given a navigation request is in flight and the bar is visible
    Then the bar is a thin strip pinned to the top edge, clear of the bottom navigation
