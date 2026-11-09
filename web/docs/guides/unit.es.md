# Tests unitarios

`gtool unit` reproduce el flujo heredado `legacy-tool u` para servicios en Go:

1. Genera con mockgen los mocks declarados en `build-config.yml`.
2. Ejecuta la suite de tests unitarios de forma recursiva con Ginkgo, generando un
   perfil de cobertura y un reporte JUnit en `./coverage`.

```sh
gtool unit
gtool unit --skip-mocks                    # tests only
gtool unit --build-config build-config.yml
```

## Declaración de mocks

```yaml
mocks:
  - source: internal/service/foo_interface.go
    filename: foo_interface.go
```

Cada archivo de interfaz `source` se convierte en un mock llamado `filename` dentro
de la carpeta `mocks`, en el paquete `mocks`.

Las fuentes que están fuera del servicio pueden vivir en una dependencia del módulo.
Estos placeholders se expanden a rutas de la caché de módulos de Go:

| Placeholder | Se expande a |
|---|---|
| `<LIB-ROOT>` | la raíz de la caché de módulos (`go env GOMODCACHE`) |
| `<COMMON-LIB>` | el directorio del módulo `shared-lib` en la lista de build |
| `<SEARCH-LIB>`, `<BUSINESS-LIB>`, `<ANALYTICS-LIB>`, `<BACKOFFICE-LIB>` | lo mismo para `search-lib`, `business-lib`, `analytics-lib`, `backoffice-lib` |

`gtool unit` necesita acceso a la red la primera vez: ejecuta `go mod download` e
instala con `go install` las herramientas fijadas que se indican abajo.

## Herramientas con versión fijada

mockgen (`go.uber.org/mock/mockgen@v0.3.0`) y la CLI de Ginkgo
(`github.com/onsi/ginkgo/v2/ginkgo@v2.2.0`) tienen la versión fijada para que los
mocks generados y los flags de Ginkgo sean idénticos a los de la herramienta heredada.
