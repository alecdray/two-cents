Feature: On-demand sweep snapshots

  The cash sweep places everything expected to move through checking on a dated
  timeline covering the next month, and holds back the worst point that timeline
  reaches. It can be run whenever the user wants one, not only on the monthly
  tick, and every run is kept: each is a record of what was advised at a given
  instant, so the page opens on the newest and steps back through the rest. A run
  refuses to produce a number when a balance it would rest on has gone too long
  without a refresh — advice on a figure we cannot vouch for looks exactly as
  authoritative as advice on a fresh one.

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

  Scenario: A card balance that stopped refreshing blocks the recommendation
    Given a card whose balance stopped refreshing
    When the sweep is run from the page
    Then the page reports that the card balance is too old to reserve against

  Scenario: A card with no statement detail still produces a result
    Given a card reporting a balance but no statement
    When the sweep is run from the page
    Then the whole balance is held back as due immediately

  Scenario: A bill falling due before the next paycheck raises what must stay put
    Given a bill due in three days and a paycheck arriving in ten
    When the sweep is run from the page
    Then the bill must be covered from the balance already in checking

  Scenario: The same bill falling due after the paycheck lowers it again
    Given a paycheck arriving in ten days and the same bill due a week later
    When the sweep is run from the page
    Then none of the checking balance needs to stay put

  Scenario: Declaring a scheduled item from the sweep page
    Given an empty schedule
    When a bill is declared on the sweep page
    Then it appears on the schedule and reaches the next run's timeline

  Scenario: Taking a scheduled item off the timeline without deleting it
    Given a declared bill the sweep is holding money back for
    When the item is switched off
    Then it is kept on the schedule but no longer reserved for

  Scenario: A card statement falling after the paycheck lowers what must stay put
    Given a card whose statement is due after the next paycheck arrives
    When the sweep is run from the page
    Then only the billed amount is reserved, and the paycheck covers it

  Scenario: Unbilled card spending is not reserved for
    Given a card whose balance exceeds what its statement billed
    When the sweep is run from the page
    Then only the billed amount is held back, not the whole balance
