Feature: On-demand sweep snapshots

  The cash sweep can be run whenever the user wants one, not only on the monthly
  tick, and every run is kept: each is a record of what was advised at a given
  instant, so the page opens on the newest and steps back through the rest. A run
  refuses to produce a number when the checking balance it would rest on has gone
  too long without a refresh — advice on a balance we cannot vouch for looks
  exactly as authoritative as advice on a fresh one.

  Scenario: Running the sweep on demand produces a snapshot
    Given no sweep has ever been run
    When the sweep is run from the page
    Then the page shows the recommendation it just computed

  Scenario: A second run adds to the history instead of replacing the first
    Given a sweep was run against an earlier checking balance
    When the sweep is run again after the balance changes
    Then the page shows the newer snapshot and can step back to the earlier one

  Scenario: A stale checking balance blocks the recommendation
    Given a checking account whose balance stopped refreshing
    When the sweep is run from the page
    Then the page reports that the checking balance is too old to advise on

  Scenario: Overspending on a card is held back from the sweep
    Given a card balance that exceeds the month's budget
    When the sweep is run from the page
    Then the recommendation reserves the debt the budget does not cover

  Scenario: A card balance that stopped refreshing blocks the recommendation
    Given a card whose balance stopped refreshing
    When the sweep is run from the page
    Then the page reports that the card balance is too old to reserve against
