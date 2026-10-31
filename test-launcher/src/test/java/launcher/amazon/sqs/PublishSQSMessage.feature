@ignore
Feature: Publish a message to an SQS queue
  # Args: queueName, message. Endpoint: localhost:4566.

  Scenario: Publish a message to an SQS queue
    * def PublishSQSMessage = Java.type('launcher.amazon.sqs.operations.PublishSQSMessage')
    * def SQSData = Java.type('launcher.amazon.sqs.operations.PublishSQSMessage$SQSData')
    * def data = new SQSData()
    * eval data.queueName = queueName
    * eval data.message = message
    * def result = new PublishSQSMessage().run(data)
    * match result == 'OK'
