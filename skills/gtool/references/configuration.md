# gtool configuration reference

`component-config.yml` at the service root (`--config` for another path). Validation is
strict: unknown keys, invalid ports and two port keys of one mock on the same host port
are errors. `${VAR}` / `$VAR` are expanded from the environment (unset ones stay as
written, `$$` is a literal `$`).

## Full component pipeline

```yaml
version: v1
app-technology: golang            # golang | generic (informational for the Docker app)

app-config:
  docker-image: my-service:latest # build it before gtool test
  port: 8080                      # port inside the container
  host-port: auto                 # free host port per run; omit = same as port
  health-path: /health            # polled until it answers; default /
  environment:
    LOG_LEVEL: debug
    DB_HOST: host.docker.internal
    DB_PORT: "${mocks.postgresql.port}"
    KAFKA_BROKERS: "host.docker.internal:${mocks.kafka.docker-port}"
    SELF_URL: "http://localhost:${app.port}"

test-launcher: test-launcher-back
test-config:
  features-path: ./test/component/features   # required, must contain .feature files
  reports-path: ./test/component/reports
  tags: "@smoke"                              # Karate tags; --tags overrides

third-party:
  mocks: [postgresql, kafka]
  mock-config:
    postgresql:
      port: auto
      user: postgres
      password: postgres
      database: postgres
      scripts-path: ./test/component/mocks-data/postgresql
    kafka:
      port: auto
      docker-port: auto
      topics: [orders]
      partitions: 1

orchestration:
  parallel-mocks: true
  startup-timeout: 180s
  health-check-interval: 3s
  health-check-retries: 60
  cleanup-on-failure: true

observability:
  log-level: info                 # --log-level and GTOOL_LOG_LEVEL win
```

## Placeholders (only in `app-config.environment`)

| Placeholder | Value |
|---|---|
| `${mocks.<service>.<port-key>}` | this run's host port of that mock key (`port`, `docker-port`, `admin-port`, ...); default port if not set |
| `${app.port}` | container port |
| `${app.host-port}` | published host port |

The app runs on a bridge network while mocks publish on the host: always use
`host.docker.internal`, which gtool maps to the host gateway on Linux and Docker Desktop.

## Mock catalogue

Every service accepts `image`, `container-name` and (except Couchbase) `port`. Any port key
accepts `auto` (only inside `gtool test` / `gtool stress`). A seeding failure fails the launch.

| Service | Default image | Ports | Options and seeding |
|---|---|---|---|
| `postgresql` | `postgres:16-alpine` | 5432 | `user`, `password`, `database`; runs the `.sql` files in `scripts-path` in order, each in one transaction |
| `mysql` | `mysql:8.4` | 3306 | `root-password`, `user`, `password`, `database`; `user: root` and `user` without `password` are rejected |
| `mongodb` | `mongo:7` | 27017 | `database` |
| `redis` | `redis:7-alpine` | 6379 | `password` |
| `kafka` | `apache/kafka:3.7.0` | 9092, 9094 | single-node KRaft; `topics`, `partitions`; `docker-port` (9094) is the listener app containers use via `host.docker.internal`, `port` is for host clients and Karate |
| `pubsub` | `gcr.io/google.com/cloudsdktool/cloud-sdk:emulators` | 8085 | `project-id`, `topics`, `subscriptions` (`name`, `topic`); app uses `PUBSUB_EMULATOR_HOST=host.docker.internal:${mocks.pubsub.port}` |
| `couchbase` | `couchbase:community-7.6.2` | 8091, 8093, 11210 | `user` (alias `username`), `password`, `bucket`, `services`; `admin-port`, `data-port` (must stay 11210) |
| `gcs` | `fsouza/fake-gcs-server:1.49.0` | 4443 | `project-id`, `buckets` |
| `floci` | `floci/floci:2.3.0` | 4566 | AWS emulator: `region`, S3 `buckets`, SQS `queues` |
| `mountebank` | `bbyars/mountebank:2.9.1` | 2525 | loads imposter JSON files from `imposters-path` (single imposter or `{"imposters": [...]}`); imposter ports are published as-is, not `auto` |

Examples:

```yaml
pubsub:
  project-id: test-project
  topics: [orders]
  subscriptions:
    - name: orders-sub
      topic: orders

floci:
  region: us-east-1
  buckets: [reports]
  queues: [events]

mountebank:
  imposters-path: ./test/component/mocks-data/mountebank
```

Mountebank is the way to fake third-party HTTP APIs: point the app's base URL at
`http://host.docker.internal:<imposter port>`.

## Environment passed to Karate

`GTOOL_APP_URL`, `GTOOL_<SERVICE>_PORT` (and `_<KEY>` for other port keys),
`GTOOL_<SERVICE>_{USER,PASSWORD,DATABASE}`, `PUBSUB_EMULATOR_HOST` when Pub/Sub is used.

## Stress testing (`gtool stress`)

Same file plus a `stress-config` block; mocks and app are started the same way.

```yaml
stress-config:
  source:
    openapi: ./api/openapi.yml      # Swagger 2.0 or OpenAPI 3.x; hosts are ignored
    methods: [GET]                  # default GET only
    include-tags: [users]
    exclude-paths: ["/health"]
    headers: { Authorization: "Bearer ${STRESS_TOKEN}" }
    param-values: { userId: ["1", "2", "3"] }
    operation-overrides:
      createUser: { body-file: test/stress/user.json, weight: 1 }
  targets:                          # complement or replace spec operations
    - { name: health, method: GET, path: /health, weight: 1 }
  ramp: { start-rps: 50, step-rps: 50, step-duration: 10s, max-rps: 500 }
  thresholds:
    degradation-latency-factor: 2.0 # p95 over 2x baseline
    degradation-p95-ms: 200         # or absolute SLO
    break-error-rate: 0.05          # 5xx + network errors
  min-acceptable-rps: 100           # exit non-zero below this
  reports-path: ./test/stress/reports
```

Outputs `stress-report.json` (stable schema for CI gates) and `stress-report.html`.
Run `gtool stress plan` first to check the resolved targets.

## Split commands

`gtool services up [svc...]`, `gtool app start`, `gtool test karate` reuse the same file
but cannot share `auto` ports: use fixed ports there. Without `GTOOL_RUN_ID` they act only
on containers that belong to no run; `--all-runs` widens them.

## Fixed-contract mode

`--stable` (on `test`, `test karate`, `services up/down`) and `--native` (on `app`) run
fixed mocks (`postgresql`, `pubsub`, `mountebank` on fixed ports) and the binaries in
`build-config.yml` as native processes. Prefer the default pipeline for new services.
