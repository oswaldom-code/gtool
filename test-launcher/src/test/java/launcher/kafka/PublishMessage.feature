@ignore
Feature: Publish a message to a Kafka topic
  # Args: topic, message. Broker: localhost:9092.

  Scenario: Publish a message to a Kafka topic
    * def KafkaClient = Java.type('launcher.kafka.KafkaClient')
    * def result = new KafkaClient().publishMessage(topic, message)
    * match result == 'OK'
