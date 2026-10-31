@smoke
Feature: Report uploaded to S3 from data/report.json

  Background:
    * def S3 = Java.type('launcher.s3.S3Client')
    * def s3 = new S3({ endpoint: 'http://localhost:4566' })

  Scenario: the app uploads the report to the bucket
    * json report = s3.getObjectAsString('reports', 'orders/report.json')
    * match report == { date: '2026-10-31', orders: 2, total: 42.5 }
