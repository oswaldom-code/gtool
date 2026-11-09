# Inicio rápido

Este recorrido usa el ejemplo `postgres-app` del repositorio: un pequeño servicio HTTP
en Go respaldado por PostgreSQL, con features de Karate y un esquema de datos inicial.

## 1. Construye las imágenes

gtool ejecuta la aplicación bajo prueba como un contenedor. Las imágenes que solo
existen en local se usan tal cual; gtool solo hace pull cuando falta una imagen. La
imagen del launcher de Karate no se publica, así que constrúyela también una vez:

```sh
git clone https://github.com/oswaldom-code/gtool.git
cd gtool
make test-launcher-image            # gtool/test-launcher-back:latest
cd examples/postgres-app
docker build -t postgres-app:latest .
```

## 2. Revisa la configuración

`component-config.yml` declara la app, los tests y los mocks:

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

## 3. Valida y ejecuta

```sh
gtool config validate --config component-config.yml
gtool test --config component-config.yml
```

gtool levanta PostgreSQL y ejecuta los scripts SQL de `scripts-path`, arranca el
contenedor de la app, ejecuta las features de Karate con el tag `smoke` y escribe el
reporte HTML en `test/component/reports`. La app y el mock se eliminan al final, sea
cual sea el resultado.

## 4. Trabaja fase a fase

Mientras escribes features es más rápido mantener el entorno levantado. Los
comandos separados no pueden compartir puertos asignados con `auto`, así que
define antes puertos explícitos (por ejemplo `host-port: 18080` y `port: 25432`;
el placeholder `${mocks.postgresql.port}` los sigue) o quédate con `gtool test`:

```sh
gtool services up          # mocks only
gtool app start            # the app from app-config
gtool test karate          # features against what is running
gtool app stop && gtool services down
```

`gtool test karate` lee `test-config` (features, reportes, tags) y los puertos del
mismo archivo, y acepta `--tags`, `--features` y `--reports` para sobrescribirlos.

## 5. Ponlo a prueba de estrés

La misma app incluye un contrato OpenAPI. Desde la raíz del repositorio:

```sh
gtool stress plan --config configs/stress-example.yml
gtool stress --config configs/stress-example.yml
```

Consulta [Pruebas de estrés](../guides/stress.md) para saber qué te dice el reporte.
