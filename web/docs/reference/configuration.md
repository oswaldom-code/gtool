# Configuration

gtool reads `./component-config.yml` by default; pass another file with
`--config`. It also looks for `component-config.yaml`, `gtool-config.yml` and
`gtool-config.yaml`. `gtool test` fails when no config file is found.

```sh
gtool config validate     # strict validation
gtool config show --format yml|yaml|json
gtool generate config     # writes a commented sample
```

## Full layout

```yaml
version: v1                         # only v1 (the default when omitted)

app-technology: golang              # required: golang | generic
app-config:
  docker-image: users-api:latest    # run the app as a container
  port: 8080                        # 0-65535, inside the container
  host-port: auto                   # auto | number; omitted = same as port (requires port)
  health-path: /health              # polled before tests, any status = ready (default /)
  environment:
    DB_HOST: host.docker.internal
    DB_PORT: "${mocks.postgresql.port}"   # this run's mock port

test-launcher: test-launcher-back   # required; only test-launcher-back
test-config:
  features-path: ./test/component/features   # required by gtool test; must exist
  reports-path: ./test/component/reports
  tags: "@smoke"

third-party:
  mocks: [postgresql, kafka]        # see Mock services
  mock-config:
    postgresql:
      port: auto                    # any port key (port, *-port) accepts auto
      user: postgres
      password: postgres
      scripts-path: ./test/component/mocks-data/postgresql

orchestration:
  parallel-mocks: true
  startup-timeout: 180s             # must be positive
  health-check-interval: 3s         # must be positive
  health-check-retries: 60          # must be positive
  cleanup-on-failure: true

observability:
  log-level: info                   # debug | info | warn | error

stress-config: {}                   # see Stress testing
```

The values shown under `orchestration` are the defaults:

- `parallel-mocks` starts the mocks concurrently instead of one by one.
- `startup-timeout` bounds the start of the phases, including the wait for the
  app's `health-path` before tests run.
- `health-check-interval` and `health-check-retries` drive the readiness polling
  of each mock: a mock that is not ready after `retries` checks fails the launch.
- `cleanup-on-failure` stops the mocks already started when another one fails to
  start. With `false`, a mock that started but never became ready is left
  running for inspection (`gtool s logs`); cancelled runs always clean up, and
  `gtool test` still tears everything down at the end.

Without `docker-image`, `gtool test` warns and skips the app phase, so the tests
run against an app you started yourself.

Canonical mock credential keys are `user` and `password`; the couchbase alias
`username` is still accepted.

!!! note "Removed options and values"
    `orchestration.preserve-logs`, `test-config.parallel`,
    `observability.structured-logs`, `metrics-enabled` and `report-format` no
    longer exist. Old files still load, because unknown fields are ignored at every
    level except `mock-config` keys. The values `minio` (mock; use `floci`),
    `nodejs` (app technology) and `test-launcher-front` (launcher) now fail
    validation.

## Environment expansion

`${VAR}` and `$VAR` references in values are expanded from the environment
after parsing:

- unset variables are left as written;
- a variable that is set but empty expands to `""`;
- `$$` is a literal `$`.

## Dynamic ports and concurrent runs

- `port: auto` on any mock port key (`port`, `*-port`) and `host-port: auto` on
  the app pick free host ports per run, so several `gtool test` runs share one
  host (e.g. a CI agent).
- `${mocks.<service>.<port-key>}`, `${app.port}` and `${app.host-port}` in
  `app-config.environment` are replaced with this run's values; mocks that do
  not set a port expose their plugin default. An unknown placeholder is an
  error.
- Every `gtool test` / `gtool stress` run labels and names its containers with
  a run ID (`GTOOL_RUN_ID`, or a generated one), so one run's teardown never
  touches another. With `GTOOL_RUN_ID` set, `services` and `app` commands act on
  that run's containers; without it, only on containers of no run; `--all-runs`
  widens them to every run.

!!! warning "`auto` needs a single invocation"
    `auto` only works in single-invocation pipelines (`gtool test`,
    `gtool stress`). Separate `services up` / `app start` commands cannot share
    allocated ports and reject it; use explicit ports there.

## Validation

`gtool config validate` lists the problems it finds, one per section (version,
app, launcher, mocks, ...); `mock-config` and `stress-config` list every problem
in them:

- `app-technology` and `test-launcher` are required, and they and `version` must
  hold a supported value.
- `third-party.mocks` entries must be one of: `mountebank`, `couchbase`,
  `postgresql`, `kafka`, `pubsub`, `gcs`, `redis`, `mongodb`, `mysql`, `floci`.
- `test-config.features-path` must exist when set; `gtool test` requires it
  (no features is an error).
- `app-config.health-path` must start with `/`; `host-port` requires `port`.
- Unknown `mock-config` keys, invalid ports and two port keys of one mock on
  the same host port are errors.
- `stress-config`, when present, is validated in full — see
  [Stress testing](../guides/stress.md).

## Environment overrides

| Variable | Overrides |
|---|---|
| `GTOOL_LOG_LEVEL` | `observability.log-level` |
| `GTOOL_RUN_ID` | the run ID that scopes this run's containers |
| `GTOOL_PARALLEL_MOCKS` | `orchestration.parallel-mocks` (`true`/`false`) |
| `GTOOL_DOCKER_IMAGE` | `app-config.docker-image` |

The log level resolves as `--log-level` > `--verbose` > `GTOOL_LOG_LEVEL` >
`observability.log-level`.

## build-config.yml

Used by `gtool unit`, `gtool app start --native` and `gtool test --stable`:

```yaml
version: v5
build:
  binaries:
    - name: api
      path: cmd/server/main.go
mocks:
  - source: internal/service/foo_interface.go
    filename: foo_interface.go
```
