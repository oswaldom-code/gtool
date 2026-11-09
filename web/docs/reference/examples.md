# Examples

The repository's
[`examples/`](https://github.com/oswaldom-code/gtool/tree/main/examples) directory
holds self-contained sample services. Each one has its own `go.mod`, a `Dockerfile`,
a `component-config.yml`, seed data and Karate features, and exercises the pipeline
end to end against one mock.

| Example | Mock | What it does |
|---|---|---|
| `postgres-app` | PostgreSQL | users API seeded from SQL scripts; also the stress sample |
| `mysql-app` | MySQL | inserts preloaded JSON rows |
| `mongodb-app` | MongoDB | inserts preloaded JSON documents |
| `redis-app` | Redis | stores preloaded JSON values |
| `kafka-app` | Kafka | publishes preloaded JSON messages |
| `pubsub-app` | Pub/Sub | publishes preloaded JSON messages |
| `gcs-app` | GCS | uploads a preloaded JSON object |
| `s3-app` | Floci (S3) | uploads a preloaded JSON object |
| `mountebank-app` | Mountebank | calls a stubbed external API |

Run any of them from its directory:

```sh
make test-launcher-image            # once, from the repository root
cd examples/postgres-app
docker build -t postgres-app:latest .
gtool test
```

Every example sets `port: auto` on its mock and `host-port: auto` on the app, and
reaches the mock through a placeholder (`${mocks.postgresql.port}`;
`${mocks.kafka.docker-port}` for Kafka, the listener app containers use), so
several examples can run at once. `postgres-app` and `mountebank-app` call the app
over HTTP with `* url appUrl`; the others assert on the mock through the launcher
helpers. `mountebank-app` is not safe to run concurrently with itself: its
imposter listens on the fixed port 4545.

## CI

[`examples/ci/jenkins`](https://github.com/oswaldom-code/gtool/tree/main/examples/ci/jenkins)
starts a local Jenkins that runs the example pipelines with gtool, and
`examples/postgres-app/Jenkinsfile` shows a pipeline running its component tests.
