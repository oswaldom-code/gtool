@ignore
Feature: Publish a message to a Pub/Sub topic
  # Args: projectId, topicId, message, [ceType, ceSource, orderingKey, attributes]
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Publish a message to a Pub/Sub topic
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('PublishMessage', __arg, false)
    * match result == 'OK'
