# Mock services

gtool ships 10 mock services. Each one runs as a Docker container, publishes its
port on the host and is seeded from your configuration before the app starts.

```yaml
third-party:
  mocks: [postgresql, kafka]
  mock-config:
    postgresql:
      scripts-path: ./test/component/mocks-data/postgresql
    kafka:
      topics: [orders]
```

Every service accepts `image` (override the default image), `container-name` and,
except Couchbase, `port`. Values not set fall back to the defaults below. Any port
key (`port`, `*-port`) accepts `auto` to pick a free host port per run (only in
`gtool test` / `gtool stress`), except Couchbase `data-port`, which must stay 11210.
App environment values reach these ports through `${mocks.<service>.<port-key>}`
placeholders ([Dynamic ports](../reference/configuration.md#dynamic-ports-and-concurrent-runs)). Unknown keys are validation errors, and a seeding
failure fails the launch instead of leaving a half-seeded mock.

## Catalogue

| Service | Default image | Port | Seeding and options |
|---|---|---|---|
| `postgresql` | `postgres:16-alpine` | 5432 | `user`, `password`, `database`; runs the SQL files in `scripts-path` |
| `mysql` | `mysql:8.4` | 3306 | `root-password`, `user`, `password`, `database`; rejects `user: root` and `user` without `password` |
| `mongodb` | `mongo:7` | 27017 | `database` |
| `redis` | `redis:7-alpine` | 6379 | `password` |
| `kafka` | `apache/kafka:3.7.0` | 9092, 9094 | single-node KRaft; creates `topics` with `partitions`; `docker-port` (default 9094) is the listener app containers use via `host.docker.internal`; set it to `auto` too for concurrent runs |
| `pubsub` | `gcr.io/google.com/cloudsdktool/cloud-sdk:emulators` | 8085 | `project-id`, `topics`, `subscriptions` |
| `couchbase` | `couchbase:community-7.6.2` | 8091, 8093, 11210 | `user` (alias `username`), `password`, `bucket`, `services`, `bucket-ram-mb`, `cluster-ram-mb`; ports via `admin-port` and `data-port` (must stay 11210); query port 8093 is published |
| `gcs` | `fsouza/fake-gcs-server:1.49.0` | 4443 | `project-id`, `buckets` |
| `floci` | `floci/floci:2.3.0` | 4566 | AWS emulator: `region`, S3 `buckets`, SQS `queues` |
| `mountebank` | `bbyars/mountebank:2.9.1` | 2525 | loads the imposter JSON files in `imposters-path` (single imposters or `{"imposters": [...]}`); imposter ports are published on the same host port, not `auto` |

## Examples

=== "Pub/Sub"

    ```yaml
    pubsub:
      project-id: test-project
      topics:
        - orders
      subscriptions:
        - name: orders-sub
          topic: orders
    ```

=== "Floci (S3 + SQS)"

    ```yaml
    floci:
      region: us-east-1
      buckets: [reports]
      queues: [events]
    ```

=== "Mountebank"

    ```yaml
    mountebank:
      port: "2525"
      imposters-path: ./test/component/mocks-data/mountebank
    ```

!!! note "Pub/Sub in stable mode"
    `services up --stable` and `test --stable` read a different Pub/Sub layout:
    `topics` is a list of `{topic-id, subscription-ids}` entries. The layout above
    is the one the default pipeline uses.

    ```yaml
    pubsub:
      project-id: test-project
      topics:
        - topic-id: orders
          subscription-ids: [orders-sub]
    ```

## Managing services

```sh
gtool services up                   # every service in the config
gtool s up postgresql kafka         # only these
gtool s status                      # alias: gtool s ps
gtool s logs kafka --tail 200 -f    # --tail defaults to 100; -f follows one service until Ctrl-C
gtool s logs --all                  # every service (a service name or --all is required)
gtool s down                        # every service; or name some; --all-runs for every run
gtool s down --stable               # the fixed-contract containers of services up --stable
```

With `GTOOL_RUN_ID` set these commands act on that run's containers; without it,
only on containers of no run.

## Test coverage

Every mock plugin has Docker integration tests
(`go test -tags=integration ./internal/plugin/services/...`).
