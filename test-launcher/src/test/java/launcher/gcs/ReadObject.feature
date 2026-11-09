@ignore
Feature: Read an object from a GCS bucket
  # Args: bucket, filePath. Returns the object bytes as result (null if missing). Server: localhost:<GTOOL_GCS_PORT, default 4443>.

  Scenario: Read an object from a GCS bucket
    * def ReadObject = Java.type('launcher.gcs.operations.ReadObject')
    * def result = new ReadObject().execute(bucket, filePath)
