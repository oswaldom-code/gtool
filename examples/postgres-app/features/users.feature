@smoke
Feature: Users API backed by PostgreSQL

  Background:
    * url 'http://localhost:8080'

  Scenario: the service is healthy
    Given path '/health'
    When method get
    Then status 200
    And match response.status == 'ok'

  Scenario: list the seeded users
    Given path '/users'
    When method get
    Then status 200
    And match response contains { id: 1, name: 'Ada Lovelace' }
    And match response contains { id: 2, name: 'Alan Turing' }

  Scenario: create a user
    Given path '/users'
    And request { name: 'Grace Hopper' }
    When method post
    Then status 201
    And match response.name == 'Grace Hopper'
