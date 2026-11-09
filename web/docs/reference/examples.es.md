# Ejemplos

El directorio
[`examples/`](https://github.com/oswaldom-code/gtool/tree/main/examples) del
repositorio contiene servicios de ejemplo autocontenidos. Cada uno tiene su propio
`go.mod`, un `Dockerfile`, un `component-config.yml`, datos iniciales y features de
Karate, y ejercita el pipeline de principio a fin contra un mock.

| Ejemplo | Mock | Qué hace |
|---|---|---|
| `postgres-app` | PostgreSQL | API de usuarios cargada desde scripts SQL; también es el ejemplo de estrés |
| `mysql-app` | MySQL | inserta filas JSON precargadas |
| `mongodb-app` | MongoDB | inserta documentos JSON precargados |
| `redis-app` | Redis | almacena valores JSON precargados |
| `kafka-app` | Kafka | publica mensajes JSON precargados |
| `pubsub-app` | Pub/Sub | publica mensajes JSON precargados |
| `gcs-app` | GCS | sube un objeto JSON precargado |
| `s3-app` | Floci (S3) | sube un objeto JSON precargado |
| `mountebank-app` | Mountebank | llama a una API externa simulada |

Ejecuta cualquiera de ellos desde su directorio:

```sh
make test-launcher-image            # once, from the repository root
cd examples/postgres-app
docker build -t postgres-app:latest .
gtool test
```

Todos los ejemplos usan `port: auto` en su mock y `host-port: auto` en la app, y
llegan al mock mediante un placeholder (`${mocks.postgresql.port}`;
`${mocks.kafka.docker-port}` en Kafka, el listener que usan los contenedores de la
app), así que se pueden ejecutar varios a la vez. `postgres-app` y `mountebank-app`
llaman a la app por HTTP con `* url appUrl`; los demás comprueban el mock con los
helpers del launcher. `mountebank-app` no es seguro de ejecutar en paralelo consigo
mismo: su impostor escucha en el puerto fijo 4545.

## CI

[`examples/ci/jenkins`](https://github.com/oswaldom-code/gtool/tree/main/examples/ci/jenkins)
levanta un Jenkins local que ejecuta los pipelines de ejemplo con gtool, y
`examples/postgres-app/Jenkinsfile` muestra un pipeline que ejecuta sus tests de
componente.
