@ignore
Feature: Wait for a message matching expectedMessage and return it as result
  # Args: projectId, topicId, subscriptionId, expectedMessage, [ignoreMessageFields, timeoutConsumeMessageSeconds]
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Wait for a message matching expectedMessage and return it as result
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('FindMessageAsync', __arg, false)
    * match result == '#string'
    * match result != 'KO'
