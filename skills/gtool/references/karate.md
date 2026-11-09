# Writing Karate features for gtool

Features live in `test-config.features-path` and run inside the launcher image
`gtool/test-launcher-back:latest` (Karate 1.5, JDK 21, host network). Every helper targets
`localhost` on this run's ports, so they work with `port: auto`.

## Skeleton

```gherkin
@smoke
Feature: Orders API

  Background:
    * url appUrl

  Scenario: create an order and persist it
    Given path '/orders'
    And request { sku: 'A-1', qty: 2 }
    When method post
    Then status 201
    And match response == { id: '#number', sku: 'A-1', qty: 2 }
    * def rows = ps.getRows("select qty from orders where id = " + response.id)
    * match rows[0].qty == 2
```

Tag scenarios (`@smoke`, `@wip`) and select them with `test-config.tags` or `--tags`.
A run with zero matching scenarios fails.

## Globals (no setup)

| Name | Purpose |
|---|---|
| `appUrl` | base URL of the app for this run |
| `ps` | `launcher.postgres.PostgresClient` on this run's PostgreSQL: `getRows(query)`, `getRows(table, filter)`, `getRow`, `getField`, `elementExists`, `updateField`, `deleteFromTable`, `executeSqlScript`, `executeParamQuery` |
| `du` | `utils.DateUtils`, timezone-aware date comparisons |
| `pdfu` | `utils.PdfUtils`, page-by-page PDF diff |
| `sleep(seconds)` | thread sleep |

## Helpers via `Java.type`

```gherkin
* def Redis = Java.type('launcher.redis.RedisClient')
# host and port default to this run's Redis
* def redis = new Redis({})
* match redis.get('order:1') == 'created'
```

| Class | Use |
|---|---|
| `launcher.postgres.PostgresClient` | PostgreSQL |
| `launcher.mysql.MySqlClient` | MySQL / MariaDB, same API shape as PostgresClient |
| `launcher.mongo.MongoClient` | insert / find / findOne / exists / delete with JSON filters; `close()` |
| `launcher.redis.RedisClient` | set / get / exists / del / setEx |
| `launcher.couchbase.CouchbaseClient` | get field or document, exists, N1QL; `close()` |
| `launcher.kafka.KafkaClient` | `publishMessage`, `consumeMessage` |
| `launcher.pubsub.PubSub` + `launcher.pubsub.operations.*` | publish, consume, ordered, find, create topic / subscription |
| `launcher.gcs.operations.ReadObject` / `DeleteObject` | fake-gcs-server objects |
| `launcher.s3.S3Client` | S3 on Floci: put / get / exists / delete; `close()` |
| `launcher.amazon.sqs.operations.PublishSQSMessage` | SQS on Floci |
| `launcher.util.Jwt` | generate / verify / inspect HS256 tokens |
| `launcher.util.Faker` | random names, emails, UUIDs, numbers |
| `launcher.util.JsonSchema` | JSON Schema (Draft 2020-12) validation |

Message matching (Pub/Sub, Kafka) is lenient: `null` expected values match anything,
arrays match in any order, `ignoreMessageFields` are dropped before comparing, and
non-matching messages stay available for later steps.

The exact method signatures are in the launcher sources:
https://github.com/oswaldom-code/gtool/tree/main/test-launcher/src/test/java/launcher

## Asynchronous effects

Use Karate's retry instead of fixed sleeps:

```gherkin
* configure retry = { count: 20, interval: 500 }
Given path '/orders', id
And retry until response.status == 'PROCESSED'
When method get
```

## Faking HTTP dependencies

Add `mountebank` to the mocks and put imposters in `imposters-path`:

```json
{
  "port": 4545,
  "protocol": "http",
  "stubs": [{
    "predicates": [{ "equals": { "method": "GET", "path": "/rates/EUR" } }],
    "responses": [{ "is": { "statusCode": 200, "body": { "rate": 1.08 } } }]
  }]
}
```

Point the app at it: `RATES_URL: http://host.docker.internal:4545`.

## Reports

`<reports-path>/karate-summary.html` (open in a browser) and `karate-summary-json.txt`
(machine-readable, used by gtool to decide pass/fail).
