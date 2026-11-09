# CLI

Global flags, accepted by every command:

| Flag | Purpose |
|---|---|
| `--config <file>` | configuration file (default `./component-config.yml`) |
| `--log-level debug\|info\|warn\|error` | log level (default `info`) |
| `-v, --verbose` | verbose output (debug) |
| `--version` | same output as `gtool version` |

The log level resolves as `--log-level` > `--verbose` > `GTOOL_LOG_LEVEL` >
`observability.log-level`. Errors are printed once, without the usage text.
Reports open in a browser only on an interactive terminal (not with `CI` or
`NO_COLOR`).

## gtool test

Runs the full component pipeline. See [Component pipeline](../guides/pipeline.md).

| Command / flag | Purpose |
|---|---|
| `gtool test` | mocks → app → Karate → cleanup |
| `--stable` | fixed-contract mocks + native app + Karate |
| `--tags` | override `test-config.tags` (with or without `--stable`) |
| `--build-config`, `--no-open` | options for `--stable` (require it) |
| `gtool test karate` | Karate only, against what is already running |

`gtool test karate` flags:

| Flag | Purpose |
|---|---|
| `--tags` | override `test-config.tags` |
| `--features` | override `test-config.features-path` |
| `--reports` | override `test-config.reports-path` |
| `--image` | override the launcher image |
| `--stable` | fixed-contract runner: fixed `test/component/{features,reports}`, opens the report |
| `--urls-to-block`, `--no-open` | options for `--stable` (require it) |

Without `--stable` it needs a configuration file with explicit ports (no `auto`).

## gtool stress

Load ramp with degradation and breaking points. See
[Stress testing](../guides/stress.md).

| Command / flag | Purpose |
|---|---|
| `gtool stress` | mocks → app → ramp → reports → cleanup |
| `--target-url <url>` | skip mocks and app, hit that URL |
| `gtool stress plan` | print the resolved targets without running load |

## gtool services (alias `s`)

| Command | Purpose |
|---|---|
| `up [service...]` | start every configured mock, or only those named; `--stable` for the fixed contract |
| `down [service...]` | stop every mock, or only those named; `--stable` stops the fixed-contract containers |
| `status` (alias `ps`) | show their status |
| `logs <service>` | `--tail N` (default 100); `-f/--follow` follows one service until Ctrl-C; `--all` for every service (a service or `--all` is required) |

With `GTOOL_RUN_ID` set, `services` commands act on that run; without it, only on
containers of no run; `--all-runs` widens them to every run.

## gtool app

| Command | Purpose |
|---|---|
| `start` | `--docker-image`, `--port` (container port only), `--env KEY=VALUE`; `--native` with `--build-config` (default `build-config.yml`) |
| `stop` | `--native` (with `--build-config`) stops the native binaries |
| `restart` | same flags as `start`; `--native` (with `--build-config`) restarts the native binaries |
| `status` (alias `ps`) | |
| `logs` | `--tail N` (default 100) |

`app` commands also accept `--all-runs`, with the same run-ID scoping as
`services`.

## gtool unit (alias `u`)

Mock generation + Ginkgo. Flags: `--build-config`, `--skip-mocks`. See
[Unit tests](../guides/unit.md).

## gtool config

| Command | Purpose |
|---|---|
| `validate` | strict schema validation |
| `show` | print the resolved configuration, `-f/--format yml\|yaml\|json` (default `yml`) |

## Other commands

| Command | Purpose |
|---|---|
| `gtool generate config` | write a sample config: `-o/--output` (default `component-config.yml`), `-f/--force` to overwrite; `generate` has the alias `g` |
| `gtool version` | version; `--full` adds the git commit and build date |
| `gtool completion <shell>` | shell autocompletion script |

## Exit codes

Every command exits `0` on success and `1` on failure. `gtool test` fails when a
phase fails or any test fails; `gtool stress` fails when the sustained load is
below `min-acceptable-rps`.
