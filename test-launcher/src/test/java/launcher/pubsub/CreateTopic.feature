@ignore
Feature: Create a Pub/Sub topic
  # Args: projectId, topicId
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Create a Pub/Sub topic
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('CreateTopic', __arg, false)
    * match result == 'OK'
