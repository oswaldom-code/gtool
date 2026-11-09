@smoke
Feature: Products stored in Redis from data/products.json

  Background:
    * def Redis = Java.type('launcher.redis.RedisClient')
    * def redis = new Redis({ host: 'localhost' })

  Scenario: the app stores every product from the data file
    * json book = redis.get('product:1')
    * match book == { id: '1', name: 'book', price: 12.5 }
    * json pen = redis.get('product:2')
    * match pen == { id: '2', name: 'pen', price: 1.2 }
