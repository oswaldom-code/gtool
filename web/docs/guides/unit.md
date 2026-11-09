# Unit tests

`gtool unit` runs the unit tests of a Go service:

1. Generates the mocks declared in `build-config.yml` with mockgen.
2. Runs the unit-test suite recursively with Ginkgo, producing a coverage profile
   and a JUnit report in `./coverage`.

```sh
gtool unit
gtool unit --skip-mocks                    # tests only
gtool unit --build-config build-config.yml
```

## Declaring mocks

```yaml
mocks:
  - source: internal/service/foo_interface.go
    filename: foo_interface.go
```

Each `source` interface file is turned into a mock named `filename` under the
`mocks` folder, in package `mocks`.

Sources outside the service can live in a module dependency. These placeholders
expand to paths in the Go module cache:

| Placeholder | Expands to |
|---|---|
| `<LIB-ROOT>` | the module cache root (`go env GOMODCACHE`) |
| `<MOD:name>` | the directory of the module in the build list whose path is `name` or ends with `/name`, e.g. `<MOD:shared-lib>/publisher.go` or `<MOD:example.com/acme/shared-lib>/publisher.go` |

`gtool unit` needs network access the first time: it runs `go mod download` and
installs the pinned tools below with `go install`.

## Pinned tools

mockgen (`go.uber.org/mock/mockgen@v0.3.0`) and the Ginkgo CLI
(`github.com/onsi/ginkgo/v2/ginkgo@v2.2.0`) are pinned so the generated mocks and
the Ginkgo flags stay identical across machines.
