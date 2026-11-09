# Aplicación bajo prueba

gtool ejecuta el servicio bajo prueba como un contenedor Docker (el pipeline por
defecto) o como procesos nativos (modo estable).

## Como contenedor

Define `app-config.docker-image`; el pipeline arranca la app después de los mocks:

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

- La imagen se toma de la caché local de Docker si existe y solo se hace pull cuando
  falta, así que las imágenes que acabas de construir con `docker build` funcionan tal
  cual.
- `port` se publica en `host-port` cuando está definido (`auto` o un número); si
  no, en el mismo puerto del host. `auto` solo funciona dentro de `gtool test` /
  `gtool stress`.
- Antes de correr los tests, el orquestador consulta
  `http://localhost:<host-port><health-path>` (por defecto `/`; cualquier estado
  HTTP cuenta como listo) hasta `orchestration.startup-timeout`.
- `host.docker.internal` resuelve a la puerta de enlace del host, de modo que la app
  llega a los mocks en sus puertos del host.
- Si el contenedor no logra arrancar, se elimina; no queda nada detrás. Un
  contenedor de la app ya terminado de una ejecución anterior se reemplaza al
  arrancar.

Sin `docker-image` la fase de la app se omite, lo que resulta útil cuando la app se
ejecuta en otro lugar.

## Gestión de la app

```sh
gtool app start                                   # from app-config
gtool app start --docker-image users-api:dev --port 9090 --env LOG_LEVEL=debug
gtool app status                                  # alias: gtool app ps
gtool app logs --tail 200                         # --tail defaults to 100
gtool app restart                                 # --native restarts the native binaries
gtool app stop
```

Los flags sobrescriben los valores del archivo de configuración; `--port` solo
reemplaza el puerto del contenedor (`host-port` no tiene flag). Con `GTOOL_RUN_ID`
definido, estos comandos actúan sobre la app de esa ejecución; sin él, solo sobre
contenedores sin ejecución; añade `--all-runs` para alcanzar todas.

## Como procesos nativos

`gtool app start --native` lanza los binarios declarados en `build-config.yml` como
procesos desacoplados:

```yaml
build:
  binaries:
    - name: api
      path: cmd/server/main.go
```

- Cada binario se busca en `$GOPATH/bin` como `<project-dir>-<name>`; compílalo
  antes.
- Los binarios reciben `--port 8080`, `8081`, … y `CUSTOM_SERVER_ADDRESS` en `7080`,
  `7081`, …
- `PUBSUB_EMULATOR_HOST` y `STORAGE_EMULATOR_HOST` apuntan a los mocks del modo
  estable (`127.0.0.1:9085` y `127.0.0.1:9086`).
- gtool espera hasta 30 s a que el puerto 8080 acepte conexiones.

`gtool app restart --native` los reinicia y `gtool app stop --native` los
detiene: gtool guarda el PID de cada binario que arrancó y detiene su grupo de
procesos completo, con la línea de comandos como alternativa para instancias que
no arrancó. `--build-config` (por defecto `build-config.yml`) apunta a otro archivo.
