@ignore
Feature: Delete an object from a GCS bucket
  # Args: bucket, filePath. Returns true as result if the object was deleted. Server: localhost:9086.

  Scenario: Delete an object from a GCS bucket
    * def DeleteObject = Java.type('launcher.gcs.operations.DeleteObject')
    * def result = new DeleteObject().execute(bucket, filePath)
