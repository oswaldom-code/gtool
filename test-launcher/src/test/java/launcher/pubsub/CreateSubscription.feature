@ignore
Feature: Create a Pub/Sub subscription bound to a topic
  # Args: projectId, topicId, subscriptionId
  # Requires PUBSUB_EMULATOR_HOST; gtool sets it when the pubsub mock is configured.

  Scenario: Create a Pub/Sub subscription bound to a topic
    * def runOperation = read('classpath:launcher/pubsub/run-operation.js')
    * def result = runOperation('CreateSubscription', __arg, false)
    * match result == 'OK'
