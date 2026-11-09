@ignore
Feature: Consume a message matching expectedMessage from a Kafka topic
  # Args: topic, expectedMessage, [ignoreMessageFields, timeoutConsumeMessageSeconds]. Broker: localhost:<GTOOL_KAFKA_PORT, default 9092>.

  Scenario: Consume a message matching expectedMessage from a Kafka topic
    * def KafkaClient = Java.type('launcher.kafka.KafkaClient')
    * def result = new KafkaClient().consumeMessage(topic, expectedMessage, karate.get('ignoreMessageFields'), karate.get('timeoutConsumeMessageSeconds'))
    * match result == 'OK'
