@smoke
Feature: Customers stored in MongoDB from data/customers.json

  Background:
    * def Mongo = Java.type('launcher.mongo.MongoClient')
    * def mongo = new Mongo({ connectionString: 'mongodb://localhost:27017', database: 'test' })

  Scenario: the app inserts every customer from the data file
    * json ada = mongo.findOne('customers', '{"code":"C1"}')
    * match ada == { _id: '#ignore', code: 'C1', name: 'Ada Lovelace', orders: 3 }
    * json alan = mongo.findOne('customers', '{"code":"C2"}')
    * match alan == { _id: '#ignore', code: 'C2', name: 'Alan Turing', orders: 5 }
