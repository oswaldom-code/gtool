@ignore
Feature: Consume messages from a subscription in the given order
  # Args: projectId, topicId, subscriptionId, orderedMessages, [ignoreMessageFields, orderingKey, maxRequestedMessages]
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Consume messages from a subscription in the given order
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('ConsumeOrderedMessages', __arg, false)
    * match result == 'OK'
