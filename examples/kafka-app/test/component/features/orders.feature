@smoke
Feature: Orders published to Kafka from data/orders.json

  Scenario: the app publishes every order from the data file
    * call read('classpath:launcher/kafka/ConsumeMessage.feature') { topic: 'orders', expectedMessage: '{"id":"1","item":"book","quantity":2}' }
    * call read('classpath:launcher/kafka/ConsumeMessage.feature') { topic: 'orders', expectedMessage: '{"id":"2","item":"pen","quantity":10}' }
