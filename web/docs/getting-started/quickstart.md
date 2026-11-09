# Quickstart

This walkthrough uses the `postgres-app` example from the repository: a small Go
HTTP service backed by PostgreSQL, with Karate features and a seed schema.

## 1. Build the images

gtool runs the application under test as a container. Images that only exist
locally are used as-is; gtool only pulls when an image is missing. The Karate
launcher image is not published, so build it once as well:

```sh
git clone https://github.com/oswaldom-code/gtool.git
cd gtool
make test-launcher-image            # gtool/test-launcher-back:latest
cd examples/postgres-app
docker build -t postgres-app:latest .
```

## 2. Read the configuration

`component-config.yml` declares the app, the tests and the mocks:

```yaml
version: v1

app-technology: golang
app-config:
  docker-image: postgres-app:latest
  port: 8080
  host-port: auto                   # free host port per run
  environment:
    DB_HOST: host.docker.internal   # the mock as seen from the app container
    DB_PORT: "${mocks.postgresql.port}"   # this run's mock port
    DB_USER: postgres
    DB_PASSWORD: postgres
    DB_NAME: postgres

test-launcher: test-launcher-back
test-config:
  features-path: ./test/component/features
  reports-path: ./test/component/reports
  tags: smoke

third-party:
  mocks:
    - postgresql
  mock-config:
    postgresql:
      port: auto
      user: postgres
      password: postgres
      database: postgres
      scripts-path: ./test/component/mocks-data/postgresql
```

## 3. Validate and run

```sh
gtool config validate --config component-config.yml
gtool test --config component-config.yml
```

gtool starts PostgreSQL and runs the SQL scripts in `scripts-path`, starts the app
container, runs the Karate features tagged `smoke` and writes the HTML report to
`test/component/reports`. App and mock are removed at the end, whatever the result.

## 4. Work phase by phase

While writing features it is faster to keep the environment up. Separate
commands cannot share ports allocated with `auto`, so set explicit ports first
(for example `host-port: 18080` and `port: 25432`; the `${mocks.postgresql.port}`
placeholder follows them) or stay with `gtool test`:

```sh
gtool services up          # mocks only
gtool app start            # the app from app-config
gtool test karate          # features against what is running
gtool app stop && gtool services down
```

`gtool test karate` reads `test-config` (features, reports, tags) and the ports of
the same file, and accepts `--tags`, `--features` and `--reports` overrides.

## 5. Stress it

The same app ships an OpenAPI contract. From the repository root:

```sh
gtool stress plan --config configs/stress-example.yml
gtool stress --config configs/stress-example.yml
```

See [Stress testing](../guides/stress.md) for what the report tells you.
