@ignore
Feature: Find a message matching expectedMessage, propagating errors
  # Args: projectId, topicId, subscriptionId, expectedMessage, [ignoreMessageFields, orderingKey, attributes, maxRequestedMessages]
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Find a message matching expectedMessage, propagating errors
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('FindMessageDebug', __arg, true)
    * match result == '#string'
    * match result != 'KO'
