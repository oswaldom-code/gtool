@ignore
Feature: Find a message matching expectedMessage and return it as result
  # Args: projectId, topicId, subscriptionId, expectedMessage, [ignoreMessageFields, orderingKey, attributes, maxRequestedMessages]
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Find a message matching expectedMessage and return it as result
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('FindMessage', __arg, false)
    * match result == '#string'
    * match result != 'KO'
