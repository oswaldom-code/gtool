# Application under test

gtool runs the service under test either as a Docker container (the default
pipeline) or as native processes (stable mode).

## As a container

Set `app-config.docker-image`; the pipeline starts the app after the mocks:

```yaml
app-config:
  docker-image: users-api:latest
  port: 8080
  host-port: auto
  health-path: /health
  environment:
    DB_HOST: host.docker.internal
    DB_PORT: "${mocks.postgresql.port}"
    LOG_LEVEL: debug
```

- The image is used from the local Docker cache when present and pulled only when
  missing, so images you just built with `docker build` work as-is.
- `port` is published on `host-port` when set (`auto` or a number), otherwise
  on the same host port. `auto` only works inside `gtool test` / `gtool stress`.
- Before running tests, the orchestrator polls
  `http://localhost:<host-port><health-path>` (default `/`; any HTTP status
  counts as ready) for up to `orchestration.startup-timeout`.
- `host.docker.internal` resolves to the host gateway, so the app reaches the
  mocks on their host ports.
- If the container fails to start, it is removed; nothing is left behind. An
  exited app container from a previous run is replaced on start.

Without `docker-image` the app phase is skipped, which is useful when the app runs
elsewhere.

## Managing the app

```sh
gtool app start                                   # from app-config
gtool app start --docker-image users-api:dev --port 9090 --env LOG_LEVEL=debug
gtool app status                                  # alias: gtool app ps
gtool app logs --tail 200                         # --tail defaults to 100
gtool app restart                                 # --native restarts the native binaries
gtool app stop
```

Flags override the values from the configuration file; `--port` replaces only the
container port (`host-port` has no flag). With `GTOOL_RUN_ID` set these commands act
on that run's app; without it, only on containers of no run; add `--all-runs` to
reach every run.

## As native processes

`gtool app start --native` launches the binaries declared in `build-config.yml` as
detached processes, like the legacy `component r`:

```yaml
build:
  binaries:
    - name: api
      path: cmd/server/main.go
```

- Each binary is looked up in `$GOPATH/bin` as `<project-dir>-<name>`; build it
  first.
- Binaries get `--port 8080`, `8081`, … and `CUSTOM_SERVER_ADDRESS` on `7080`,
  `7081`, …
- `PUBSUB_EMULATOR_HOST` and `STORAGE_EMULATOR_HOST` point to the stable-mode
  mocks (`127.0.0.1:9085` and `127.0.0.1:9086`).
- gtool waits up to 30 s for port 8080 to accept connections.

`gtool app restart --native` restarts them and `gtool app stop --native` stops
them: gtool records the PID of each binary it started and stops its whole process
group, falling back to a command-line match for instances it did not start.
`--build-config` (default `build-config.yml`) points at another file.
