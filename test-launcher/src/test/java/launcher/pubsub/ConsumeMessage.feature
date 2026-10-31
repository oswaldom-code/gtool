@ignore
Feature: Consume a message matching expectedMessage from a subscription
  # Args: projectId, topicId, subscriptionId, expectedMessage, [ignoreMessageFields, orderingKey, attributes, maxRequestedMessages, timeoutConsumeMessageSeconds]
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Consume a message matching expectedMessage from a subscription
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('ConsumeMessage', __arg, false)
    * match result == 'OK'
