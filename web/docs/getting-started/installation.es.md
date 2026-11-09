# Instalación

## Requisitos

- **Go 1.26+** para instalar o compilar gtool.
- **Docker**: los mocks, la aplicación bajo prueba y el launcher de Karate se ejecutan
  como contenedores. gtool se comunica con el daemon local de Docker mediante el
  Docker SDK.

## Instala la última versión

```sh
go install github.com/oswaldom-code/gtool/cmd/gtool@latest
```

El binario se instala en `$GOPATH/bin` (normalmente `~/go/bin`). Si tu shell no
encuentra `gtool`, añade ese directorio a tu `PATH`:

```sh
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc   # or ~/.bashrc
```

## Compila desde el código fuente

```sh
git clone https://github.com/oswaldom-code/gtool.git
cd gtool
make build               # ./bin/gtool
make install             # copies the binary to $(go env GOPATH)/bin
```

`make build` incrusta en el binario la versión, el commit de git y la fecha de
compilación. Un `go install` simple no lo hace, así que `gtool version --full`
muestra el commit `dev`.

## Imagen del launcher de Karate

`gtool test` ejecuta tus features con el launcher de Karate incluido en el
repositorio. Su imagen no se publica en ningún registro: constrúyela una vez por
máquina antes del primer `gtool test`, directamente desde el repositorio (sin
clonarlo):

```sh
docker build -t gtool/test-launcher-back:latest \
  https://github.com/oswaldom-code/gtool.git#v1.0.0:test-launcher
```

Desde un clon, `make test-launcher-image` hace lo mismo.

## Verificación

```sh
gtool version --full
gtool --help
```

El autocompletado de la shell está disponible mediante
`gtool completion bash|zsh|fish|powershell`.

## Agentes de IA

El repositorio incluye una [Agent Skill](https://github.com/oswaldom-code/gtool/tree/main/skills/gtool)
que enseña a los agentes de código (Claude Code, Codex, Cursor, Gemini CLI y otros) a
instalar gtool e integrarlo en un servicio:

```sh
npx skills add oswaldom-code/gtool --skill gtool
```
