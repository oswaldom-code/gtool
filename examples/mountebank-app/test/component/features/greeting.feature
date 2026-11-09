@smoke
Feature: Greeting backed by an external API mocked with Mountebank

  Scenario: the app returns the greeting from the external API
    Given url appUrl
    And path '/greeting'
    When method get
    Then status 200
    And match response == { greeting: 'hello from mountebank' }
