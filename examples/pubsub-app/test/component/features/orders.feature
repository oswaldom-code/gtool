@smoke
Feature: Orders published to Pub/Sub from data/orders.json

  Background:
    * def pubsub = { projectId: 'test-project', topicId: 'orders', subscriptionId: 'orders-sub' }

  Scenario: the app publishes every order from the data file
    * call read('classpath:launcher/pubsub/ConsumeMessage.feature') karate.merge(pubsub, { expectedMessage: '{"id":"1","item":"book","quantity":2}' })
    * call read('classpath:launcher/pubsub/ConsumeMessage.feature') karate.merge(pubsub, { expectedMessage: '{"id":"2","item":"pen","quantity":10}' })
