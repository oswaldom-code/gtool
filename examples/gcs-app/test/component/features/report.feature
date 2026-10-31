@smoke
Feature: Report uploaded to GCS from data/report.json

  Scenario: the app uploads the report to the bucket
    * def object = call read('classpath:launcher/gcs/ReadObject.feature') { bucket: 'reports', filePath: 'orders/report.json' }
    * json report = karate.toString(object.result)
    * match report == { date: '2026-10-31', orders: 2, total: 42.5 }
