Feature: Login

  The whole application sits behind a single local login. An unauthenticated
  visitor can reach nothing but the login screen; the correct password lets them
  in, a wrong one is refused, and signing out drops them back to the gate.

  Scenario: An unauthenticated visitor is sent to the login screen
    Given I am not signed in
    When I open a page in the app
    Then I am redirected to the login screen

  Scenario: The correct password signs me in
    Given I am not signed in
    When I enter the correct password
    Then I land in the app

  Scenario: A wrong password is refused
    Given I am not signed in
    When I enter an incorrect password
    Then I stay on the login screen with an error

  Scenario: Signing out returns to the login screen and re-locks the app
    Given I am signed in
    When I sign out
    Then I am back on the login screen
    And reopening a page in the app redirects me to the login screen
