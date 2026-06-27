# postgres-app — sample application under test

A minimal Go HTTP service backed by PostgreSQL, used to demonstrate and exercise
the `gtool` pipeline (mock PostgreSQL → app → Karate tests → cleanup).

## Endpoints

| Method | Path      | Description                          |
|--------|-----------|--------------------------------------|
| GET    | `/health` | `200` when the database is reachable |
| GET    | `/users`  | List users                           |
| POST   | `/users`  | Create a user (`{"name": "..."}`)    |

Configuration is read from environment variables: `DB_HOST`, `DB_PORT`,
`DB_USER`, `DB_PASSWORD`, `DB_NAME`, `PORT`.

## Run it standalone (no gtool)

```bash
# Start PostgreSQL with the seed schema
docker run -d --name pg -p 5432:5432 \
  -e POSTGRES_PASSWORD=postgres \
  -v "$PWD/init":/docker-entrypoint-initdb.d \
  postgres:16-alpine

# Run the app against it
go run .

curl localhost:8080/health
curl localhost:8080/users
curl -XPOST localhost:8080/users -d '{"name":"Grace Hopper"}'
```

## Run it with gtool

```bash
# 1. Build the app image (gtool runs it as a container)
docker build -t postgres-app:latest .

# 2. From this directory, run the full pipeline
gtool test --config component-config.yml
```

`gtool test` starts the PostgreSQL mock (seeded from `init/`), starts this app,
runs the Karate features in `features/`, and tears everything down.

## Networking notes

- **App → mock:** gtool runs the app as a bridge-network container while the
  mock publishes `5432` on the host. The app therefore reaches the mock through
  the host gateway. On Docker Desktop `host.docker.internal` resolves
  automatically; on Linux you currently need `host.docker.internal` mapped to
  the host gateway (or run the mock and app on the same Docker network). This is
  a known gtool limitation tracked for a future enhancement.
- **Tests → app:** the Karate launcher runs on the host network, so it reaches
  the app at `localhost:8080`.
- **Local image:** `postgres-app:latest` only exists locally after
  `docker build`; ensure it is built before running `gtool test`.
