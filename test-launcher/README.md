# gtool test-launcher

Karate-based backend test launcher image used by `gtool` to run component tests
against an already-running app and its mocks. This is the open, in-repo
replacement for the legacy private `test-launcher-back` image — a "swiss-army"
launcher with helpers for the datastores, messaging systems, object storage and
test utilities a component test typically needs.

## What it is

- Base: `maven:3.9.9-eclipse-temurin-21` (public).
- Engine: [Karate](https://github.com/karatelabs/karate) `karate-junit5` (`io.karatelabs`, **1.5.2**). 1.5.x keeps the `com.intuit.karate.*` package namespace.
- Built as `gtool/test-launcher-back:latest` (the default image used by `gtool`).

## Helpers

### Globals (wired in `karate-config.js`, ready to use in every feature)

| Variable | Class | Purpose |
|---|---|---|
| `appUrl` | — | base URL of the app under test for this run (`GTOOL_APP_URL`, default `http://localhost:8080`); use `* url appUrl` |
| `ps` | `launcher.postgres.PostgresClient` | read / seed / assert against PostgreSQL (this run's port, user, password and database from `GTOOL_POSTGRESQL_*`, defaults `postgres`) |
| `du` | `utils.DateUtils` | timezone-aware date comparisons |
| `pdfu` | `utils.PdfUtils` | page-by-page PDF diff (writes a highlighted diff image) |
| `sleep(seconds)` | — | thread sleep |

### Available via `Java.type(...)`

Instantiate inside a feature, e.g.:

```gherkin
* def Redis = Java.type('launcher.redis.RedisClient')
* def redis = new Redis({ host: 'localhost', port: '6379' })
* match redis.get('some-key') == 'expected'
```

Each helper targets `localhost` (the container runs with `--network host`).
gtool exports this run's ports as `GTOOL_<SERVICE>_PORT` (and connection
settings as `GTOOL_<SERVICE>_USER/PASSWORD/DATABASE`); `launcher.util.RunPorts`
reads them and falls back to the plugin defaults, so helpers follow
`port: auto`.

Message matching (Pub/Sub consume/find, Kafka consume) is lenient and shared by
every helper: a `null` expected value matches anything, objects must have the
same fields, arrays match regardless of order (duplicates count), and
`ignoreMessageFields` are removed before comparing. Messages that do not match
are left for later steps (Pub/Sub nacks them; Kafka resumes after the last
matched offset).

| Class | Domain | Default endpoint |
|---|---|---|
| `launcher.postgres.PostgresClient` | PostgreSQL (rows/fields/exists/update/delete/scripts/param queries) | `GTOOL_POSTGRESQL_PORT` (default 5432) |
| `launcher.mysql.MySqlClient` | MySQL/MariaDB (same API shape as PostgresClient) | from `url` |
| `launcher.couchbase.CouchbaseClient` | Couchbase (get field/document, exists, N1QL); `close()` | from `connectionString` |
| `launcher.redis.RedisClient` | Redis (set/get/exists/del/setEx) | `GTOOL_REDIS_PORT` (default 6379) |
| `launcher.mongo.MongoClient` | MongoDB (insert/find/findOne/exists/delete, JSON filters); `close()` | `connectionString`, default `GTOOL_MONGODB_PORT` (27017) |
| `launcher.kafka.KafkaClient` | Kafka (`publishMessage`, `consumeMessage`) | `GTOOL_KAFKA_PORT` (default 9092) |
| `launcher.pubsub.PubSub` + `launcher.pubsub.operations.*` | Pub/Sub emulator (publish, consume, ordered, find, create topic/subscription) | `PUBSUB_EMULATOR_HOST`, else `GTOOL_PUBSUB_PORT` (8085) |
| `launcher.gcs.operations.ReadObject` / `DeleteObject` | GCS emulator (fake-gcs-server) | `GTOOL_GCS_PORT` (default 4443) |
| `launcher.amazon.sqs.operations.PublishSQSMessage` | SQS (publish) | `GTOOL_FLOCI_PORT` (default 4566) |
| `launcher.s3.S3Client` | S3 / MinIO (put/get/exists/delete); `close()` | from `endpoint` |
| `launcher.util.Jwt` | generate / verify / inspect HS256 tokens | — |
| `launcher.util.Faker` | random test data (name/email/uuid/number/expression) | — |
| `launcher.util.JsonSchema` | JSON Schema (Draft 2020-12) validation | — |

> Not included: a generic gRPC helper (needs per-service stubs) and a custom
> "await" helper (Karate already provides retry / `karate.repeat`).

## Runtime contract (consumed by gtool)

`gtool`'s Karate runners (`internal/core/test/karate.go`, `internal/core/test/stablekarate/runner.go`) expect:

| Aspect | Value |
|---|---|
| Workdir | `/app` |
| Features (bind, ro) | `/app/features` |
| Reports (bind) | `/app/target/karate-reports` (HTML: `karate-summary.html`) |
| Entry | `CMD ["bash","-c","./scripts/run.bash"]` |
| Env | `TAGS`, `PUBSUB_EMULATOR_HOST`, `GTOOL_APP_URL`, `GTOOL_<SVC>_PORT`, `GTOOL_<SVC>_{USER,PASSWORD,DATABASE}`, `MVN_ONLINE` (allow Maven downloads; offline by default), `DEBUG`. `URLS_TO_BLOCK` is accepted for the legacy contract but ignored |
| Network | host |
| User | invoking host user (`uid:gid`), so reports are owned by it |
| Pass/fail | container exit code |

`src/test/java/launcher/features` is a symlink to `/app/features`, so user
features are picked up on the `classpath:launcher/features` path that
`TestLauncher` runs in parallel.

### Build trick

`RUN mvn test` during the image build executes **zero** features (the
`/app/features` volume is empty at build time), so it only pre-caches Maven
dependencies and compiles the helpers.

## Build & run

```bash
# build the image (or: make test-launcher-image from the repo root)
docker build -t gtool/test-launcher-back:latest test-launcher/

# via gtool, against a running app + mocks
cd examples/postgres-app
gtool services up
gtool test karate            # uses gtool/test-launcher-back:latest by default
```

## Local helper unit tests

```bash
cd test-launcher && mvn test   # DateUtils, Jwt, Faker, JsonSchema, TestLauncher
```

> Datastore/messaging helpers are thin driver wrappers validated by compilation;
> end-to-end coverage requires the matching services (e.g. via `gtool services up`)
> or Testcontainers.
