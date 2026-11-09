---
name: gtool
description: Set up and run component tests for a microservice with gtool, a Go CLI that starts Docker mocks (PostgreSQL, MySQL, MongoDB, Redis, Kafka, Pub/Sub, Couchbase, GCS, Floci S3/SQS, Mountebank), launches the app as a container, runs Karate API tests and tears everything down. Use when the user wants to add component, integration or API tests to a service, mentions gtool, component-config.yml, Karate features, mocking third-party dependencies in Docker, stress or load testing an HTTP service from its OpenAPI contract, or running these pipelines in CI (GitHub Actions, Jenkins).
---

# gtool

gtool runs a component-test pipeline with one command: **mocks → app → Karate tests → cleanup**.
Teardown is guaranteed, including on failure and Ctrl-C.

## 1. Check prerequisites

```bash
docker info >/dev/null && echo docker-ok      # Docker daemon is required
go version                                    # Go >= 1.26 to install with go install
gtool version 2>/dev/null || echo "gtool missing"
docker image inspect gtool/test-launcher-back:latest >/dev/null 2>&1 || echo "launcher image missing"
```

## 2. Install

```bash
go install github.com/oswaldom-code/gtool/cmd/gtool@latest
# make sure $(go env GOPATH)/bin is on PATH
```

The Karate launcher image is not published to a registry; build it once per machine
(straight from the repository, no clone needed):

```bash
docker build -t gtool/test-launcher-back:latest \
  https://github.com/oswaldom-code/gtool.git#v1.0.0:test-launcher
```

## 3. Wire gtool into the user's service

Work from the service's repository root. Inspect it first: which port the app listens on,
its health endpoint, which external dependencies it uses and through which env vars it
reads their host/port/credentials.

1. **Dockerfile**: the app runs as a container. If the repo has none, add one, then
   `docker build -t <service>:latest .` (gtool uses local images; it pulls only missing ones).
2. **Config**: `gtool generate config` writes a commented `component-config.yml`. Edit it:
   - `app-config.docker-image`, `port`, `health-path` (polled before tests; default `/`).
   - `host-port: auto` and `port: auto` on every mock, so runs never collide.
   - One entry in `third-party.mocks` per dependency, configured in `mock-config`.
   - App env vars point at mocks via `host.docker.internal` and placeholders:
     `DB_HOST: host.docker.internal`, `DB_PORT: "${mocks.postgresql.port}"`.
   Full schema, mock catalogue and defaults: [references/configuration.md](references/configuration.md).
3. **Seed data** under `test/component/mocks-data/<service>/` (SQL scripts, imposters, ...).
4. **Features** under `test/component/features/*.feature`, always starting with
   `* url appUrl`. Helpers for asserting on mocks: [references/karate.md](references/karate.md).
5. `gtool config validate` — fix every error before running.
6. `gtool test` — exit code 0 means every scenario passed. Report:
   `<reports-path>/karate-summary.html`.

Add `test/component/reports/` to `.gitignore`.

## 4. Commands

```bash
gtool test                      # full pipeline; --tags "@smoke" overrides test-config.tags
gtool test karate               # Karate only, mocks and app already running
gtool services up|down|status   # alias: gtool s; logs <svc> --tail 200 -f
gtool app start|stop|status|logs|restart
gtool stress plan               # resolved load targets, no traffic
gtool stress                    # mocks → app → load ramp → JSON/HTML report
gtool stress --target-url URL   # load a deployed environment instead
gtool unit                      # mockgen from build-config.yml + Ginkgo with coverage
gtool config show --format yaml
```

Global flags: `--config <file>` (default `./component-config.yml`), `--log-level debug`,
`-v`. Iterate on features fast with the split flow (fixed ports, no `auto`):

```bash
gtool s up && gtool app start && gtool test karate --tags "@wip"
gtool app stop && gtool s down
```

## 5. CI

Each `gtool test` / `gtool stress` gets a run ID (`GTOOL_RUN_ID` or generated) that labels
and names its containers, so concurrent builds on one agent are safe with `auto` ports.
Output is plain text when not on a TTY, or with `CI` / `NO_COLOR` set.

GitHub Actions:

```yaml
- uses: actions/setup-go@v5
  with: { go-version: "1.26" }
- run: go install github.com/oswaldom-code/gtool/cmd/gtool@latest
- run: docker build -t gtool/test-launcher-back:latest https://github.com/oswaldom-code/gtool.git#v1.0.0:test-launcher
- run: docker build -t my-service:latest .
- run: gtool test
  env: { GTOOL_RUN_ID: "gh-${{ github.run_id }}" }
- uses: actions/upload-artifact@v4
  if: always()
  with: { name: karate-report, path: test/component/reports }
```

A Jenkinsfile with concurrent builds: `examples/postgres-app/Jenkinsfile` in the gtool repo.

## 6. Troubleshooting

| Symptom | Fix |
|---|---|
| App cannot reach a mock | Use `host.docker.internal:<port>`, never `localhost`. Linux with ufw: `sudo ufw allow in on docker0` |
| Readiness timeout | Wrong `port` or `health-path`; inspect `gtool app logs` during the run or raise `orchestration.startup-timeout` |
| `features-path` error / zero scenarios | Path must exist and contain `.feature` files matching the tags; zero scenarios counts as a failure |
| `auto` rejected | `auto` only works in `gtool test` / `gtool stress`; split commands need fixed ports |
| Unknown key in `mock-config` | Validation is strict; check the catalogue in references/configuration.md |
| Leftover containers | `gtool s down --all-runs && gtool app stop --all-runs` |
| Launcher image not found | Build it (step 2) |

Working examples, one per mock: `examples/` in https://github.com/oswaldom-code/gtool.
Documentation: https://oswaldom-code.github.io/gtool/
