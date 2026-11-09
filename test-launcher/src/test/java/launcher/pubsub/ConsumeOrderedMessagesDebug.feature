@ignore
Feature: Consume ordered messages, propagating errors
  # Args: projectId, topicId, subscriptionId, orderedMessages, [ignoreMessageFields, orderingKey, maxRequestedMessages]
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Consume ordered messages, propagating errors
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('ConsumeOrderedMessagesDebug', __arg, true)
    * match result == 'OK'
