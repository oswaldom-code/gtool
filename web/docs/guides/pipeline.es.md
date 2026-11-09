# Pipeline de componente

`gtool test` ejecuta los tests de componente de un microservicio contra un entorno
aislado y desechable:

<div class="gt-pipeline">
  <span class="step">Mocks</span><span class="arr">→</span>
  <span class="step">App</span><span class="arr">→</span>
  <span class="step hot">Tests</span><span class="arr">→</span>
  <span class="step">Reporte</span><span class="arr">→</span>
  <span class="step end">Limpieza</span>
</div>

1. **Mocks**: se levantan y se cargan con datos todos los servicios de
   `third-party.mocks` ([Servicios mock](mocks.md)).
2. **App**: cuando `app-config.docker-image` está definido, la aplicación bajo prueba
   se arranca como contenedor ([Aplicación bajo prueba](app.md)).
3. **Tests**: el launcher de Karate ejecuta las features de
   `test-config.features-path`.
4. **Reporte**: el reporte HTML de Karate se escribe en `test-config.reports-path`.
5. **Limpieza**: la app y los mocks se detienen en orden inverso.

La limpieza se ejecuta siempre: cuando todo va bien, cuando falla una fase y ante
Ctrl-C o SIGTERM. Usa un contexto nuevo, así que una ejecución cancelada también lo
desmonta todo.

```sh
gtool test --config component-config.yml
```

El comando termina con un código distinto de cero cuando falla una fase o cualquier
test, así que puede actuar directamente como gate de un job de CI.

## El launcher de Karate

Los tests se ejecutan en la imagen `gtool/test-launcher-back:latest`, construida a
partir de
[`test-launcher/`](https://github.com/oswaldom-code/gtool/tree/main/test-launcher)
con `make test-launcher-image` (consulta [Instalación](../getting-started/installation.md#imagen-del-launcher-de-karate));
no se publica en ningún registro. En cada ejecución gtool:

- monta `features-path` en solo lectura y `reports-path` en lectura-escritura dentro
  del contenedor;
- lo ejecuta en la **red del host**, de modo que las features llegan a la app y a los
  mocks en `localhost:<port>`;
- pasa `TAGS` desde `test-config.tags`, y `PUBSUB_EMULATOR_HOST` cuando el mock
  `pubsub` está configurado;
- pasa los valores de la ejecución: `GTOOL_APP_URL`, `GTOOL_APP_PORT`,
  `GTOOL_APP_HOST_PORT`, un `GTOOL_<SERVICIO>_<CLAVE_DE_PUERTO>` por cada clave de
  puerto de cada mock habilitado, incluidos los valores por defecto
  (`GTOOL_POSTGRESQL_PORT`, `GTOOL_KAFKA_DOCKER_PORT`, `GTOOL_COUCHBASE_ADMIN_PORT`,
  ...), y `GTOOL_<SERVICIO>_{USER,USERNAME,PASSWORD,DATABASE}` cuando están en
  `mock-config`; las features usan `* url appUrl` en lugar de un puerto fijo;
- ejecuta el launcher con tu usuario del host, así que los reportes te pertenecen
  (un `chown` final es solo una red de seguridad);
- lee los conteos reales de escenarios de `karate-summary-json.txt`; una
  ejecución con cero escenarios es un fallo.

`test-config.features-path` es obligatorio: sin él `gtool test` falla en lugar
de no probar nada.

Cada ejecución de `gtool test` delimita sus contenedores con un run ID
(`GTOOL_RUN_ID`, o uno generado), así las ejecuciones concurrentes en un host
nunca se desmontan entre sí. Consulta
[Puertos dinámicos y ejecuciones concurrentes](../reference/configuration.md#puertos-dinamicos-y-ejecuciones-concurrentes).

## Ejecutar solo los tests

Mantén el entorno levantado mientras escribes features y vuelve a ejecutar solo
Karate:

```sh
gtool services up
gtool app start
gtool test karate --tags "@smoke"
```

Este flujo abarca invocaciones separadas, así que necesita puertos explícitos:
`port: auto` y `host-port: auto` solo funcionan en pipelines de una sola
invocación (`gtool test`, `gtool stress`) y aquí se rechazan. Sin
`GTOOL_RUN_ID`, `services` y `app` solo actúan sobre contenedores sin
ejecución; `--all-runs` los amplía a todas.

`gtool test karate` usa el mismo launcher y el mismo entorno que `gtool test`: lee
`test-config` y los puertos del archivo de configuración (es obligatorio) e imprime
la ruta del reporte HTML. Acepta:

| Flag | Propósito |
|---|---|
| `--tags` | filtro de tags de Karate (sobrescribe `test-config.tags`) |
| `--features` | ruta de las features (sobrescribe `test-config.features-path`) |
| `--reports` | ruta de los reportes (sobrescribe `test-config.reports-path`) |
| `--image` | sobrescribe la imagen del launcher |
| `--stable` | runner heredado de `component e`, ver abajo |
| `--urls-to-block` | con `--stable`; se acepta por compatibilidad, el launcher de gtool lo ignora |
| `--no-open` | con `--stable`; no abre el reporte HTML |

## Modo estable

`gtool test --stable` reproduce el flujo heredado `component t` con un contrato fijo:
los mocks `postgresql`, `pubsub` y `mountebank` sobre imágenes públicas oficiales,
cargados desde `test/component/mocks-data`, los binarios de la app declarados en
`build-config.yml` lanzados como procesos nativos y, después, Karate.

```sh
gtool test --stable --tags "@smoke" --build-config build-config.yml
```

Las mismas piezas están disponibles por separado: `gtool services up --stable`,
`gtool app start --native` y `gtool test karate --stable`. Este último usa siempre
`test/component/features` y `test/component/reports` (que se vacía antes de
ejecutar), apunta los helpers de Pub/Sub al emulador estable y abre el reporte HTML
en una terminal interactiva (no con `CI` ni `NO_COLOR`).

## Red

El contenedor de la app se ejecuta en la red bridge de Docker, mientras que los mocks
publican sus puertos en el host. gtool mapea `host.docker.internal` a la puerta de
enlace del host en el contenedor de la app, así que apunta la app a
`host.docker.internal:<mock port>` tanto en Linux como en Docker Desktop.

!!! warning "Firewalls en Linux"
    Si el firewall del host descarta el tráfico del bridge de Docker (por ejemplo, ufw
    con su política por defecto), la app no puede llegar a los mocks. Permítelo con
    `sudo ufw allow in on docker0`.
