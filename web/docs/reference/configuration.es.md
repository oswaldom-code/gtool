# Configuración

gtool lee `./component-config.yml` por defecto; indica otro archivo con `--config`.
También busca `component-config.yaml`, `gtool-config.yml` y `gtool-config.yaml`.
`gtool test` falla si no encuentra un archivo de config.

```sh
gtool config validate     # strict validation
gtool config show --format yml|yaml|json
gtool generate config     # writes a commented sample
```

## Estructura completa

```yaml
version: v1                         # only v1 (the default when omitted)

app-technology: golang              # required: golang | generic
app-config:
  docker-image: users-api:latest    # run the app as a container
  port: 8080                        # 0-65535, inside the container
  host-port: auto                   # auto | number; omitted = same as port (requires port)
  health-path: /health              # polled before tests, any status = ready (default /)
  environment:
    DB_HOST: host.docker.internal
    DB_PORT: "${mocks.postgresql.port}"   # this run's mock port

test-launcher: test-launcher-back   # required; only test-launcher-back
test-config:
  features-path: ./test/component/features   # required by gtool test; must exist
  reports-path: ./test/component/reports
  tags: "@smoke"

third-party:
  mocks: [postgresql, kafka]        # see Mock services
  mock-config:
    postgresql:
      port: auto                    # any port key (port, *-port) accepts auto
      user: postgres
      password: postgres
      scripts-path: ./test/component/mocks-data/postgresql

orchestration:
  parallel-mocks: true
  startup-timeout: 180s             # must be positive
  health-check-interval: 3s         # must be positive
  health-check-retries: 60          # must be positive
  cleanup-on-failure: true

observability:
  log-level: info                   # debug | info | warn | error

stress-config: {}                   # see Stress testing
```

Los valores que aparecen en `orchestration` son los valores por defecto:

- `parallel-mocks` arranca los mocks en paralelo en lugar de uno a uno.
- `startup-timeout` limita el arranque de las fases, incluida la espera al
  `health-path` de la app antes de los tests.
- `health-check-interval` y `health-check-retries` controlan el sondeo de
  disponibilidad de cada mock: un mock que no está listo tras `retries`
  comprobaciones hace fallar el arranque.
- `cleanup-on-failure` detiene los mocks ya arrancados cuando otro falla al
  arrancar. Con `false`, un mock que arrancó pero nunca llegó a estar listo se
  deja en ejecución para inspeccionarlo (`gtool s logs`); las ejecuciones
  canceladas siempre limpian, y `gtool test` desmonta todo al final igualmente.

Sin `docker-image`, `gtool test` avisa y omite la fase de la app, así que los tests
se ejecutan contra una app que hayas arrancado tú.

Las claves canónicas de credenciales de los mocks son `user` y `password`; se
sigue aceptando el alias `username` de couchbase.

!!! note "Opciones y valores eliminados"
    `orchestration.preserve-logs`, `test-config.parallel`,
    `observability.structured-logs`, `metrics-enabled` y `report-format` ya no
    existen. Los archivos antiguos siguen cargando, porque los campos
    desconocidos se ignoran en todos los niveles salvo las claves de
    `mock-config`. Los valores `minio` (mock; usa `floci`), `nodejs` (tecnología
    de la app) y `test-launcher-front` (launcher) ahora fallan la validación.

## Expansión de variables de entorno

Las referencias `${VAR}` y `$VAR` en los valores se expanden desde el entorno
tras parsear:

- las variables no definidas quedan como están;
- una variable definida pero vacía se expande a `""`;
- `$$` es un `$` literal.

## Puertos dinámicos y ejecuciones concurrentes

- `port: auto` en cualquier clave de puerto de un mock (`port`, `*-port`) y
  `host-port: auto` en la app eligen puertos de host libres por ejecución, así
  varias ejecuciones de `gtool test` comparten un host (p. ej. un agente de CI).
- `${mocks.<servicio>.<clave-de-puerto>}`, `${app.port}` y `${app.host-port}`
  en `app-config.environment` se reemplazan con los valores de la ejecución;
  los mocks que no fijan puerto exponen el valor por defecto del plugin. Un
  placeholder desconocido es un error.
- Cada ejecución de `gtool test` / `gtool stress` etiqueta y nombra sus
  contenedores con un run ID (`GTOOL_RUN_ID`, o uno generado), así el teardown
  de una nunca toca a otra. Con `GTOOL_RUN_ID` definido, los comandos `services`
  y `app` actúan sobre los contenedores de esa ejecución; sin él, solo sobre
  contenedores sin ejecución; `--all-runs` los amplía a todas.

!!! warning "`auto` necesita una sola invocación"
    `auto` solo funciona en pipelines de una sola invocación (`gtool test`,
    `gtool stress`). Los comandos separados `services up` / `app start` no
    pueden compartir los puertos asignados y lo rechazan; usa puertos
    explícitos ahí.

## Validación

`gtool config validate` lista los problemas que encuentra, uno por sección
(versión, app, launcher, mocks, ...); `mock-config` y `stress-config` listan todos
sus problemas:

- `app-technology` y `test-launcher` son obligatorios, y ellos y `version` deben
  tener un valor soportado.
- Las entradas de `third-party.mocks` deben ser una de: `mountebank`, `couchbase`,
  `postgresql`, `kafka`, `pubsub`, `gcs`, `redis`, `mongodb`, `mysql`, `floci`.
- `test-config.features-path` debe existir cuando está definido; `gtool test`
  lo exige (sin features es un error).
- `app-config.health-path` debe empezar por `/`; `host-port` requiere `port`.
- Las claves desconocidas en `mock-config`, los puertos inválidos y dos claves
  de puerto de un mismo mock en el mismo puerto de host son errores.
- `stress-config`, si está presente, se valida por completo; consulta
  [Pruebas de estrés](../guides/stress.md).

## Sobrescritura por variables de entorno

| Variable | Sobrescribe |
|---|---|
| `GTOOL_LOG_LEVEL` | `observability.log-level` |
| `GTOOL_RUN_ID` | el run ID que delimita los contenedores de la ejecución |
| `GTOOL_PARALLEL_MOCKS` | `orchestration.parallel-mocks` (`true`/`false`) |
| `GTOOL_DOCKER_IMAGE` | `app-config.docker-image` |

El nivel de log se resuelve como
`--log-level` > `--verbose` > `GTOOL_LOG_LEVEL` > `observability.log-level`.

## build-config.yml

Lo usan `gtool unit`, `gtool app start --native` y `gtool test --stable`:

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
