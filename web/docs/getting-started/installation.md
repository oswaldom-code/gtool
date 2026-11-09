# Installation

## Requirements

- **Go 1.26+** to install or build gtool.
- **Docker**: mocks, the application under test and the Karate launcher run as
  containers. gtool talks to the local Docker daemon through the Docker SDK.

## Install the latest release

```sh
go install github.com/oswaldom-code/gtool/cmd/gtool@latest
```

The binary lands in `$GOPATH/bin` (usually `~/go/bin`). If your shell cannot find
`gtool`, add that directory to your `PATH`:

```sh
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc   # or ~/.bashrc
```

## Build from source

```sh
git clone https://github.com/oswaldom-code/gtool.git
cd gtool
make build               # ./bin/gtool
make install             # copies the binary to $(go env GOPATH)/bin
```

`make build` stamps the version, git commit and build date into the binary. A plain
`go install` does not, so `gtool version --full` reports commit `dev`.

## Karate launcher image

`gtool test` runs your features with the in-repo Karate launcher. Its image is not
published to a registry: build it once per machine before the first `gtool test`,
straight from the repository (no clone needed):

```sh
docker build -t gtool/test-launcher-back:latest \
  https://github.com/oswaldom-code/gtool.git#v1.0.0:test-launcher
```

From a clone, `make test-launcher-image` does the same.

## Verify

```sh
gtool version --full
gtool --help
```

Shell completion is available through `gtool completion bash|zsh|fish|powershell`.

## AI coding agents

The repository ships an [Agent Skill](https://github.com/oswaldom-code/gtool/tree/main/skills/gtool)
that teaches coding agents (Claude Code, Codex, Cursor, Gemini CLI and others) to
install gtool and wire it into a service:

```sh
npx skills add oswaldom-code/gtool --skill gtool
```
