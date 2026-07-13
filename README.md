# GTOOL - Component Testing Orchestrator

> 🌐 **Language:** English · [Español](docs/readme/README.es.md)

<div align="center">

[![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go)](https://go.dev/)
![Tests](https://img.shields.io/badge/tests-185%20passing-success)
![Pipeline](https://img.shields.io/badge/pipeline-functional-success)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

**A Go CLI to orchestrate microservice component tests: spin up mocks, launch the app, run the tests and clean everything up — with a single command.**

</div>

---

## 🎯 What GTOOL does

GTOOL is a CLI orchestrator for **component testing** of microservices. It stands up a service's external dependencies as mocks, launches the service, runs its test suite against that isolated environment, and tears everything down — reproducibly, with a single command. It replaces hand-managed Docker setups and ad-hoc shell scripts with one typed Go binary featuring structured logging, strict config validation and a plugin architecture.

```mermaid
graph LR
    A[🔧 Mocks] --> B[🚀 App]
    B --> C[🧪 Karate Tests]
    C --> D[📊 HTML Report]
    D --> E[🧹 Cleanup]

    style A fill:#4fc3f7
    style B fill:#66bb6a
    style C fill:#ffa726
    style D fill:#ab47bc
    style E fill:#ef5350
```

**A single `gtool test` runs the full pipeline:**

1. **Mocks** — starts the third-party dependencies in Docker. 10 built-in services: PostgreSQL, MySQL, MongoDB, Redis, Kafka, Pub/Sub, Couchbase, GCS, MinIO and Mountebank.
2. **App** — launches the service under test, as a Docker image or as native binaries.
3. **Tests** — runs the Karate (backend API) suite against the app and its mocks.
4. **Report** — generates the Karate HTML report and can open it in the browser.
5. **Cleanup** — always tears down app and mocks, even on failure or Ctrl-C.

Beyond the pipeline, GTOOL also runs **unit tests** (`gtool unit` — mock generation + Ginkgo) and can drive each phase on its own (`gtool services`, `gtool app`, `gtool test karate`). New mock services plug in through the `ServicePlugin` interface.

---

## 🚀 Installation

```bash
git clone <repo-url> && cd gtool

make build              # builds ./bin/gtool
./bin/gtool version     # verify

make install            # copies the binary to $GOPATH/bin
```

> ⚠️ **`make install` copies to `$GOPATH/bin` (`~/go/bin`).** If your terminal can't find `gtool` after installing, that directory is not on your `PATH`. Add it:
> ```bash
> echo 'export PATH="$PATH:$GOPATH/bin"' >> ~/.zshrc   # or ~/.bashrc
> source ~/.zshrc && rehash
> ```

**Requirements:** Go 1.24+, Docker, Make.

---

## ⚡ Quick Start

```bash
# 1. Validate the repo configuration
gtool config validate --config component-config.yml

# 2. Start the mocks only
gtool services up
gtool services status
gtool services down

# 3. Full pipeline (mocks → app → tests → cleanup)
gtool test
```

GTOOL looks for `./component-config.yml` by default. Use `--config <file>` for another one.

---

## 📖 Commands

Every command accepts `--config <file>`, `--log-level debug|info|warn|error` and `--verbose`.

### `gtool config` — configuration
```bash
gtool config validate --config component-config.yml   # validates the schema
gtool config show --format yaml                        # prints the resolved config
gtool config show --format json
```

### `gtool services` (alias `s`) — third-party mocks
```bash
gtool services up                     # starts every mock in the config
gtool s up postgresql kafka           # starts specific services
gtool s status                        # services status
gtool s logs postgresql               # logs of a service
gtool s down                          # stops all
```

### `gtool app` — application under test
```bash
gtool app start --docker-image myapp:latest --port 8080
gtool app status
gtool app logs --tail 100
gtool app stop
```

### `gtool unit` (alias `u`) — unit tests
Generates the mocks from `build-config.yml` (mockgen) and runs the suite with Ginkgo, leaving coverage and the JUnit report in `./coverage`.
```bash
gtool unit
gtool unit --skip-mocks               # runs the tests only
gtool unit --build-config build-config.yml
```

### `gtool test` — component pipeline
```bash
gtool test                            # gtool-native pipeline (public images)
gtool test karate                     # Karate only (mocks and app already up)
gtool test karate --tags "@smoke" --no-open
```

### `gtool generate` / `gtool version`
```bash
gtool generate config                 # generates a sample component-config.yml
gtool version
```

---

## 🧩 Configuration

GTOOL uses two files (`.yml` extension preferred; `.yaml` supported):

### `component-config.yml` — component pipeline
```yaml
version: v1
app-technology: golang            # golang | nodejs | generic
test-launcher: test-launcher-back
third-party:
  mocks: [postgresql, pubsub, mountebank]
  mock-config:
    pubsub:
      project-id: my-project
      topics:
        - topic-id: my-topic
          subscription-ids: [my-sub]
```

### `build-config.yml` — Go binaries and mocks (for `gtool unit`)
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

---

## 🏗️ Architecture

```mermaid
graph TB
    User[👤 User] --> CLI[CLI - Cobra]
    CLI --> Orch[Orchestrator]
    Orch --> Mock[Mock Manager]
    Orch --> App[App Launcher]
    Orch --> Test[Test Runner]
    Mock --> Plugins[Service Plugins]
    Plugins --> Docker[Docker Client]
    App --> Docker
    Test --> Docker

    style CLI fill:#b3e5fc
    style Orch fill:#81d4fa
    style Mock fill:#4fc3f7
    style App fill:#4fc3f7
    style Test fill:#4fc3f7
```

- **Plugins** (`internal/plugin/`): `ServicePlugin` (mocks), `AppLauncher`, `TestExecutor`, registered in a thread-safe `PluginRegistry`.
- **Typed errors** (`pkg/errors`) and **structured logging** with Zap (`pkg/logger`).

---

## 🛠️ Development

```bash
make build          # builds ./bin/gtool
make test           # tests with -race
make test-coverage  # HTML coverage report
make lint           # golangci-lint
make fmt            # gofmt + goimports
make clean          # cleans artifacts
make help           # lists every target
```

**Standards:** minimum 80% coverage for new code (95%+ for config/orchestration), table-driven tests, errors from `pkg/errors`, conventional commits. See [CLAUDE.md](CLAUDE.md).

Integration tests (require Docker) per plugin:
```bash
go test -tags=integration ./internal/plugin/services/...
```

---

## ⚠️ Known limitations

- **App→mock networking on Linux (native `gtool test`).** The default pipeline runs the app as a bridge-network container while the mocks publish their ports on the host, so a containerized app cannot reach a mock at `localhost`. It needs a host gateway (`host.docker.internal`, automatic on Docker Desktop, manual on Linux) or a shared Docker network. The `--stable` / `--native` path avoids this by running the app as native processes that reach the mocks over `127.0.0.1` (mountebank runs on the host network).
- **Newer mock plugins are lightly tested.** `redis`, `mongodb`, `mysql` and `minio` currently have unit tests only — no Docker integration tests yet. Treat them as experimental.
- **Karate result is exit-code based.** Pass/fail comes from the launcher's exit code; there is no per-scenario parsing of the `karate-reports` output.

---

## 📦 Status

Functional end-to-end pipeline: 10 mock plugins, app launcher (Docker and native), Karate runner and orchestration with guaranteed teardown. 185 tests passing.

| Phase | Status |
|------|--------|
| 1. Foundation (CLI, config, plugins, errors, logging) | ✅ |
| 2. Mocks (6 plugins) | ✅ |
| 3. App Launcher (Docker + native) | ✅ |
| 4. Test Executor (Karate) | ✅ |
| 5. Orchestration (pipeline + teardown) | ✅ |
| 6–7. Advanced features, docs/release | 🔄 |

---
