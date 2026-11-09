# Pruebas de estrés

`gtool stress` reutiliza el pipeline de componente (mocks y app) y sustituye la fase
de tests por una rampa de carga escalonada. Responde a dos preguntas sobre el servicio
y usa la respuesta como gate de CI.

<div class="gt-pipeline">
  <span class="step">Mocks</span><span class="arr">→</span>
  <span class="step">App</span><span class="arr">→</span>
  <span class="step hot">Rampa de carga</span><span class="arr">→</span>
  <span class="step fail">Punto de ruptura</span><span class="arr">→</span>
  <span class="step">Reportes</span><span class="arr">→</span>
  <span class="step end">Limpieza</span>
</div>

- **Punto de degradación**: la primera etapa en la que la latencia p95 supera
  `degradation-latency-factor` × el p95 de referencia (primera etapa), o el SLO
  absoluto `degradation-p95-ms`, o el throughput cae por debajo del 90% de la tasa
  solicitada.
- **Punto de ruptura**: la primera etapa en la que la tasa de errores (5xx y errores
  de red: timeouts, conexiones rechazadas) supera `break-error-rate`. La rampa se
  detiene ahí. Las respuestas 4xx se reportan aparte y nunca cuentan como errores.
- **RPS máximo sostenido**: el RPS solicitado más alto completado antes del punto de
  ruptura. El comando falla cuando es inferior a `min-acceptable-rps`.

!!! info "Mediciones relativas al host"
    El generador de carga, la app y los mocks comparten el host. Usa los números para
    comparar commits; para mediciones serias, apunta `--target-url` a un entorno
    dedicado.

## Comandos

```sh
gtool stress plan                              # resolved targets, no load
gtool stress                                   # mocks → app → ramp → reports → cleanup
gtool stress --target-url http://staging:8080  # skip mocks and app, hit that URL
```

Sin `--target-url` la carga va a `http://localhost:<puerto del host>` (el
`app-config.host-port` publicado, o `port` si no está definido), después de que el
pipeline espere al `app-config.health-path`. En ambos modos el runner espera además
hasta `orchestration.startup-timeout` a que `/` responda por HTTP en el destino.
Ctrl-C cancela la rampa; los mocks y la app se desmontan igualmente.

## Configuración

El bloque siguiente lista todas las opciones con valores ilustrativos; el ejemplo
ejecutable
[`configs/stress-example.yml`](https://github.com/oswaldom-code/gtool/blob/main/configs/stress-example.yml)
usa una rampa más suave (de 50 a 500 RPS en pasos de 50 cada 10 s, SLO de p95 de
200 ms).

```yaml
stress-config:
  source:
    openapi: api/openapi.yml          # Swagger 2.0 or OpenAPI 3.x
    base-path: /api/v1                # default: servers[0] path (v3) / basePath (v2)
    methods: [GET]                    # default: GET only
    include-tags: [products]
    exclude-tags: []
    include-paths: ["/products/*"]    # path.Match globs on the spec path
    exclude-paths: ["/health"]
    headers:
      Authorization: "Bearer ${STRESS_TOKEN}"
    param-values:
      productId: ["1", "2", "3"]      # rotated across requests
    operation-overrides:              # keyed by operationId
      createOrder: { body-file: test/stress/order.json, weight: 3 }
  targets:                            # explicit targets, path used as-is
    - name: health
      method: GET
      path: /health
      weight: 1
  ramp:
    start-rps: 10
    step-rps: 10
    step-duration: 20s
    max-rps: 500                      # the last stage is always max-rps
  thresholds:
    degradation-latency-factor: 2.0   # default 2.0
    degradation-p95-ms: 800           # 0 disables the absolute SLO
    break-error-rate: 0.05            # default 0.05
  min-acceptable-rps: 100             # 0 disables the gate
  reports-path: test/stress/reports   # default
```

`stress-config` se valida por separado: `gtool stress` no necesita los ajustes de los
tests de componente, como `test-launcher`. La validación lista todos los problemas
del bloque:

- Se requiere `source.openapi` o al menos una entrada en `targets`.
- `ramp.start-rps` y `ramp.step-duration` deben ser positivos, `max-rps` debe ser al
  menos `start-rps` y `step-rps` debe ser positivo cuando `max-rps` supera a
  `start-rps`.
- `thresholds.degradation-latency-factor` debe ser mayor que 1,
  `degradation-p95-ms` no negativo y `break-error-rate` entre 0 y 1.
- `min-acceptable-rps` no puede ser negativo ni superar `ramp.max-rps`.
- Los targets necesitan un método soportado y una ruta que empiece por `/`; `body`
  y `body-file` son excluyentes, un `body-file` debe existir y los pesos no pueden
  ser negativos. `source.base-path` debe empezar por `/`.

## Resolución de targets

| Elemento | Precedencia |
|---|---|
| Parámetros de path, query y cabecera | `param-values` > `example` / `default` / `enum[0]` de la spec > valor generado a partir del esquema |
| Parámetros opcionales de query y cabecera | solo se envían cuando `param-values` los define |
| Body | override `body-file` > ejemplo de la spec > mock generado a partir del esquema |
| Cabeceras | `source.headers` + parámetros de cabecera + el `Content-Type` del body de la petición |
| Peso | override `weight`, por defecto 1; la carga se reparte proporcionalmente |
| Host | los hosts de la spec se ignoran; se usa la app del pipeline o `--target-url` |

Los `targets` explícitos sustituyen a una operación de la spec con el mismo método y
path completo, y en caso contrario se añaden. Los valores generados usan una semilla,
así que `plan` muestra exactamente lo que envía una ejecución.

`gtool stress plan` y los reportes avisan cuando hay métodos que modifican datos
habilitados (las escrituras llenan los mocks, que pasan a ser el cuello de botella),
cuando más de 20 targets comparten el peso por defecto y cuando más del 10% de las
respuestas de referencia son 4xx, señal de targets mal resueltos.

## Reportes

Ambos archivos se generan en `reports-path`:

- **`stress-report.json`**: esquema estable (`schema-version: 1`) para CI: el build de
  gtool y el host que lo generó, la rampa, los umbrales efectivos, los targets, las
  métricas por etapa (RPS alcanzado, throughput, latencia media/p50/p95/p99/máx., tasas
  de errores y de 4xx, códigos de estado), los dos puntos (`null` si no se alcanzan),
  `max-sustained-rps`, `passed` y `warnings`.
- **`stress-report.html`**: una página autocontenida: gráficas de latencia y de tasa
  de errores según el RPS solicitado con ambos puntos marcados, tablas de etapas y de
  targets, y los detalles de la ejecución.

```sh
gtool stress || exit 1
jq '.["max-sustained-rps"], .passed' test/stress/reports/stress-report.json
```

## Consejos

- Mantén `methods: [GET]` salvo que lo que quieras medir sean las escrituras.
- A cientos de RPS, sube el límite de archivos abiertos: `ulimit -n 65535`.
- El repositorio incluye un ejemplo ejecutable:
  [`configs/stress-example.yml`](https://github.com/oswaldom-code/gtool/blob/main/configs/stress-example.yml)
  somete a estrés `examples/postgres-app`.
