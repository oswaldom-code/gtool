# CLI

Flags globales, aceptados por todos los comandos:

| Flag | Propósito |
|---|---|
| `--config <file>` | archivo de configuración (por defecto `./component-config.yml`) |
| `--log-level debug\|info\|warn\|error` | nivel de log (por defecto `info`) |
| `-v, --verbose` | salida detallada (debug) |
| `--version` | la misma salida que `gtool version` |

El nivel de log se resuelve como `--log-level` > `--verbose` >
`GTOOL_LOG_LEVEL` > `observability.log-level`. Los errores se imprimen una sola
vez, sin el texto de uso. Los reportes se abren en el navegador solo en una
terminal interactiva (no con `CI` ni `NO_COLOR`).

## gtool test

Ejecuta el pipeline de componente completo. Consulta
[Pipeline de componente](../guides/pipeline.md).

| Comando / flag | Propósito |
|---|---|
| `gtool test` | mocks → app → Karate → limpieza |
| `--stable` | mocks de contrato fijo + app nativa + Karate |
| `--tags` | sobrescribe `test-config.tags` (con o sin `--stable`) |
| `--build-config`, `--no-open` | opciones para `--stable` (lo requieren) |
| `gtool test karate` | solo Karate, contra lo que ya está en ejecución |

Flags de `gtool test karate`:

| Flag | Propósito |
|---|---|
| `--tags` | sobrescribe `test-config.tags` |
| `--features` | sobrescribe `test-config.features-path` |
| `--reports` | sobrescribe `test-config.reports-path` |
| `--image` | sobrescribe la imagen del launcher |
| `--stable` | runner de contrato fijo: `test/component/{features,reports}` fijos, abre el reporte |
| `--urls-to-block`, `--no-open` | opciones de `--stable` (lo requieren) |

Sin `--stable` necesita un archivo de configuración con puertos explícitos (sin `auto`).

## gtool stress

Rampa de carga con puntos de degradación y de ruptura. Consulta
[Pruebas de estrés](../guides/stress.md).

| Comando / flag | Propósito |
|---|---|
| `gtool stress` | mocks → app → rampa → reportes → limpieza |
| `--target-url <url>` | omite los mocks y la app, y ataca esa URL |
| `gtool stress plan` | muestra los targets resueltos sin generar carga |

## gtool services (alias `s`)

| Comando | Propósito |
|---|---|
| `up [service...]` | levanta todos los mocks configurados, o solo los indicados; `--stable` para el contrato fijo |
| `down [service...]` | detiene todos los mocks, o solo los indicados; `--stable` detiene los contenedores del contrato fijo |
| `status` (alias `ps`) | muestra su estado |
| `logs <service>` | `--tail N` (por defecto 100); `-f/--follow` sigue un servicio hasta Ctrl-C; `--all` para todos los servicios (se requiere un servicio o `--all`) |

Con `GTOOL_RUN_ID` definido, los comandos `services` actúan sobre esa ejecución;
sin él, solo sobre contenedores sin ejecución; `--all-runs` los amplía a todas.

## gtool app

| Comando | Propósito |
|---|---|
| `start` | `--docker-image`, `--port` (solo el puerto del contenedor), `--env KEY=VALUE`; `--native` con `--build-config` (por defecto `build-config.yml`) |
| `stop` | `--native` (con `--build-config`) detiene los binarios nativos |
| `restart` | los mismos flags que `start`; `--native` (con `--build-config`) reinicia los binarios nativos |
| `status` (alias `ps`) | |
| `logs` | `--tail N` (por defecto 100) |

Los comandos `app` también aceptan `--all-runs`, con el mismo alcance por run ID
que `services`.

## gtool unit (alias `u`)

Generación de mocks + Ginkgo. Flags: `--build-config`, `--skip-mocks`. Consulta
[Tests unitarios](../guides/unit.md).

## gtool config

| Comando | Propósito |
|---|---|
| `validate` | validación estricta del esquema |
| `show` | muestra la configuración resuelta, `-f/--format yml\|yaml\|json` (por defecto `yml`) |

## Otros comandos

| Comando | Propósito |
|---|---|
| `gtool generate config` | escribe una configuración de ejemplo: `-o/--output` (por defecto `component-config.yml`), `-f/--force` para sobrescribir; `generate` tiene el alias `g` |
| `gtool version` | versión; `--full` añade el commit de git y la fecha de compilación |
| `gtool completion <shell>` | script de autocompletado para la shell |

## Códigos de salida

Todos los comandos terminan con `0` si todo va bien y con `1` si fallan. `gtool test`
falla cuando falla una fase o cualquier test; `gtool stress` falla cuando la carga
sostenida es inferior a `min-acceptable-rps`.
