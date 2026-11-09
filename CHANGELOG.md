# Changelog

## v1.0.0

First stable release. Documentation: https://oswaldom-code.github.io/gtool/

### Added

- `gtool stress`: load ramp over targets resolved from an OpenAPI contract
  (Swagger 2.0 / OpenAPI 3.x) and explicit targets, degradation and breaking
  points, JSON and HTML reports, and a CI gate on `min-acceptable-rps`.
  `gtool stress plan` prints the resolved targets without load.
- Dynamic ports and run IDs: `port: auto` on any mock port key and
  `host-port: auto` on the app; `${mocks.<svc>.<key>}`, `${app.port}` and
  `${app.host-port}` placeholders in `app-config.environment`; every
  `gtool test` / `gtool stress` run labels and names its containers
  (`GTOOL_RUN_ID`), so concurrent runs share a host.
- Karate receives `GTOOL_APP_URL`, `GTOOL_APP_PORT`, `GTOOL_APP_HOST_PORT`,
  `GTOOL_<SVC>_<PORT_KEY>` and `GTOOL_<SVC>_{USER,USERNAME,PASSWORD,DATABASE}`;
  features use `* url appUrl`.
- In-repo Karate launcher (`test-launcher/`, image
  `gtool/test-launcher-back:latest`) with helpers for PostgreSQL, MySQL,
  MongoDB, Redis, Couchbase, Kafka, Pub/Sub, GCS, S3/SQS, JWT, Faker and JSON
  Schema.
- Floci mock (AWS S3 and SQS emulator).
- `app-config.health-path`: the app is polled before tests run.
- `--all-runs` on `services` and `app`.
- Documentation site (English and Spanish), CI examples (GitHub Actions,
  Jenkins with concurrent builds) and an Agent Skill
  (`npx skills add oswaldom-code/gtool --skill gtool`).
- Release binaries for linux and darwin (amd64, arm64) and windows (amd64).

### Changed

- `gtool test karate` uses the same runner as `gtool test`: it reads
  `test-config` and the configured ports. The fixed-contract runner moved
  behind `--stable`; `--urls-to-block` and `--no-open` only apply there.
- `gtool test` fails without a config file, with an invalid config, without
  `test-config.features-path` or when zero scenarios run.
- Test counts come from the Karate scenario summary.
- Without `GTOOL_RUN_ID`, `services` and `app` only act on containers of no
  run; `--all-runs` restores the global sweep.
- Stricter validation: unknown `mock-config` keys, MySQL `user: root`, a
  remapped Couchbase `data-port`, `nodejs` and `test-launcher-front` are
  errors.
- A seeding failure fails the mock launch.
- `${VAR}` with a set but empty variable expands to an empty string.
- `--log-level`, `--verbose`, `GTOOL_LOG_LEVEL` and
  `observability.log-level` take effect, in that order of precedence.
- Terminal symbols only on an interactive terminal; plain text with a pipe,
  `CI` or `NO_COLOR`.
- Native apps (`--native`) are stopped by PID and process group.
- `make install` copies to `$(go env GOPATH)/bin`.

### Removed

- The `minio` mock (use `floci`).
- Options `orchestration.preserve-logs`, `test-config.parallel`,
  `observability.structured-logs`, `metrics-enabled` and `report-format`
  (old files still load).
- Unused `AppLauncher` / `TestExecutor` plugin interfaces and their stub
  implementations.

## v0.1.0

Initial release.
