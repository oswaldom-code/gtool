# Component pipeline

`gtool test` runs a microservice's component tests against an isolated, disposable
environment:

<div class="gt-pipeline">
  <span class="step">Mocks</span><span class="arr">→</span>
  <span class="step">App</span><span class="arr">→</span>
  <span class="step hot">Tests</span><span class="arr">→</span>
  <span class="step">Report</span><span class="arr">→</span>
  <span class="step end">Cleanup</span>
</div>

1. **Mocks** — every service in `third-party.mocks` is started and seeded
   ([Mock services](mocks.md)).
2. **App** — when `app-config.docker-image` is set, the application under test is
   started as a container ([Application under test](app.md)).
3. **Tests** — the Karate launcher runs the features in `test-config.features-path`.
4. **Report** — the Karate HTML report is written to `test-config.reports-path`.
5. **Cleanup** — the app and the mocks are stopped in reverse order.

Cleanup always runs: on success, on a failed phase and on Ctrl-C or SIGTERM. It uses
a fresh context, so a cancelled run still tears everything down.

```sh
gtool test --config component-config.yml
```

The command exits non-zero when a phase fails or any test fails, so it can gate a CI
job directly.

## The Karate launcher

Tests run in the `gtool/test-launcher-back:latest` image, built from
[`test-launcher/`](https://github.com/oswaldom-code/gtool/tree/main/test-launcher)
with `make test-launcher-image` (see [Installation](../getting-started/installation.md#karate-launcher-image));
it is not published to a registry. For each run gtool:

- mounts `features-path` read-only and `reports-path` read-write into the container;
- runs it on the **host network**, so features reach the app and the mocks on
  `localhost:<port>`;
- passes `TAGS` from `test-config.tags`, and `PUBSUB_EMULATOR_HOST` when the
  `pubsub` mock is configured;
- passes this run's values: `GTOOL_APP_URL`, `GTOOL_APP_PORT`, `GTOOL_APP_HOST_PORT`,
  one `GTOOL_<SERVICE>_<PORT_KEY>` per port key of every enabled mock, defaults
  included (`GTOOL_POSTGRESQL_PORT`, `GTOOL_KAFKA_DOCKER_PORT`,
  `GTOOL_COUCHBASE_ADMIN_PORT`, ...), and `GTOOL_<SERVICE>_{USER,USERNAME,PASSWORD,DATABASE}`
  when set in `mock-config`; features use `* url appUrl` instead of a hardcoded
  port;
- runs the launcher as your host user, so the reports belong to you (a final
  `chown` is only a safety net);
- reads the real scenario counts from `karate-summary-json.txt`; a run with
  zero scenarios is a failure.

`test-config.features-path` is required: without it `gtool test` fails instead
of testing nothing.

Every `gtool test` run scopes its containers with a run ID (`GTOOL_RUN_ID`, or a
generated one), so concurrent runs on one host never tear each other down. See
[Dynamic ports and concurrent runs](../reference/configuration.md#dynamic-ports-and-concurrent-runs).

## Running only the tests

Keep the environment up while you write features, and rerun only Karate:

```sh
gtool services up
gtool app start
gtool test karate --tags "@smoke"
```

This flow spans separate invocations, so it needs explicit ports: `port: auto`
and `host-port: auto` only work in single-invocation pipelines (`gtool test`,
`gtool stress`) and are rejected here. Without `GTOOL_RUN_ID`, `services` and
`app` act only on containers of no run; `--all-runs` widens them to every run.

`gtool test karate` uses the same launcher and environment as `gtool test`: it reads
`test-config` and the ports of the configuration file (a file is required) and
prints the path of the HTML report. It accepts:

| Flag | Purpose |
|---|---|
| `--tags` | Karate tags filter (overrides `test-config.tags`) |
| `--features` | features path (overrides `test-config.features-path`) |
| `--reports` | reports path (overrides `test-config.reports-path`) |
| `--image` | override the launcher image |
| `--stable` | legacy `component e` runner, see below |
| `--urls-to-block` | with `--stable`; accepted for compatibility, ignored by the gtool launcher |
| `--no-open` | with `--stable`; do not open the HTML report |

## Stable mode

`gtool test --stable` reproduces the legacy `component t` flow with a fixed
contract: the `postgresql`, `pubsub` and `mountebank` mocks on official public
images seeded from `test/component/mocks-data`, the app binaries declared in
`build-config.yml` launched as native processes, then Karate.

```sh
gtool test --stable --tags "@smoke" --build-config build-config.yml
```

The same building blocks are available on their own: `gtool services up --stable`,
`gtool app start --native` and `gtool test karate --stable`. The latter always uses
`test/component/features` and `test/component/reports` (cleared before the run),
points Pub/Sub helpers at the stable emulator and opens the HTML report on an
interactive terminal (not with `CI` or `NO_COLOR`).

## Networking

The app container runs on the Docker bridge network while the mocks publish their
ports on the host. gtool maps `host.docker.internal` to the host gateway on the app
container, so point the app at `host.docker.internal:<mock port>` on Linux and
Docker Desktop alike.

!!! warning "Linux firewalls"
    If the host firewall drops traffic from the Docker bridge (ufw with its default
    policy, for example), the app cannot reach the mocks. Allow it with
    `sudo ufw allow in on docker0`.
