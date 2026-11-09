# Servicios mock

gtool incluye 10 servicios mock. Cada uno se ejecuta como un contenedor Docker,
publica su puerto en el host y se carga con datos según tu configuración antes de que
arranque la app.

```yaml
third-party:
  mocks: [postgresql, kafka]
  mock-config:
    postgresql:
      scripts-path: ./test/component/mocks-data/postgresql
    kafka:
      topics: [orders]
```

Todos los servicios aceptan `image` (sobrescribe la imagen por defecto),
`container-name` y, salvo Couchbase, `port`. Los valores no definidos toman los
valores por defecto de la tabla siguiente. Cualquier clave de puerto (`port`,
`*-port`) acepta `auto` para elegir un puerto de host libre por ejecución (solo en
`gtool test` / `gtool stress`), salvo `data-port` de Couchbase, que debe seguir en
11210. Los valores del entorno de la app llegan a estos puertos mediante los
placeholders `${mocks.<servicio>.<clave-de-puerto>}`
([Puertos dinámicos](../reference/configuration.md#puertos-dinamicos-y-ejecuciones-concurrentes)). Las claves desconocidas son errores de validación,
y un fallo en la carga de datos hace fallar el arranque en lugar de dejar un mock
a medio cargar.

## Catálogo

| Servicio | Imagen por defecto | Puerto | Carga de datos y opciones |
|---|---|---|---|
| `postgresql` | `postgres:16-alpine` | 5432 | `user`, `password`, `database`; ejecuta los archivos SQL de `scripts-path` |
| `mysql` | `mysql:8.4` | 3306 | `root-password`, `user`, `password`, `database`; rechaza `user: root` y `user` sin `password` |
| `mongodb` | `mongo:7` | 27017 | `database` |
| `redis` | `redis:7-alpine` | 6379 | `password` |
| `kafka` | `apache/kafka:3.7.0` | 9092, 9094 | KRaft de un solo nodo; crea `topics` con `partitions`; `docker-port` (por defecto 9094) es el listener que usan los contenedores de la app vía `host.docker.internal`; ponlo también en `auto` para ejecuciones concurrentes |
| `pubsub` | `gcr.io/google.com/cloudsdktool/cloud-sdk:emulators` | 8085 | `project-id`, `topics`, `subscriptions` |
| `couchbase` | `couchbase:community-7.6.2` | 8091, 8093, 11210 | `user` (alias `username`), `password`, `bucket`, `services`, `bucket-ram-mb`, `cluster-ram-mb`; puertos vía `admin-port` y `data-port` (debe seguir en 11210); se publica el puerto de query 8093 |
| `gcs` | `fsouza/fake-gcs-server:1.49.0` | 4443 | `project-id`, `buckets` |
| `floci` | `floci/floci:2.3.0` | 4566 | emulador de AWS: `region`, `buckets` de S3, `queues` de SQS |
| `mountebank` | `bbyars/mountebank:2.9.1` | 2525 | carga los archivos JSON de impostores de `imposters-path` (impostores sueltos o `{"imposters": [...]}`); los puertos de los impostores se publican en el mismo puerto del host, no `auto` |

## Ejemplos

=== "Pub/Sub"

    ```yaml
    pubsub:
      project-id: test-project
      topics:
        - orders
      subscriptions:
        - name: orders-sub
          topic: orders
    ```

=== "Floci (S3 + SQS)"

    ```yaml
    floci:
      region: us-east-1
      buckets: [reports]
      queues: [events]
    ```

=== "Mountebank"

    ```yaml
    mountebank:
      port: "2525"
      imposters-path: ./test/component/mocks-data/mountebank
    ```

!!! note "Pub/Sub en modo estable"
    `services up --stable` y `test --stable` leen un formato de Pub/Sub distinto:
    `topics` es una lista de entradas `{topic-id, subscription-ids}`. El formato de
    arriba es el que usa el pipeline por defecto.

    ```yaml
    pubsub:
      project-id: test-project
      topics:
        - topic-id: orders
          subscription-ids: [orders-sub]
    ```

## Gestión de servicios

```sh
gtool services up                   # every service in the config
gtool s up postgresql kafka         # only these
gtool s status                      # alias: gtool s ps
gtool s logs kafka --tail 200 -f    # --tail defaults to 100; -f follows one service until Ctrl-C
gtool s logs --all                  # every service (a service name or --all is required)
gtool s down                        # every service; or name some; --all-runs for every run
gtool s down --stable               # the fixed-contract containers of services up --stable
```

Con `GTOOL_RUN_ID` definido, estos comandos actúan sobre los contenedores de esa
ejecución; sin él, solo sobre contenedores sin ejecución.

## Cobertura de tests

Todos los plugins de mock tienen tests de integración con Docker
(`go test -tags=integration ./internal/plugin/services/...`).
