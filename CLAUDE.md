# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 🚨 MANDATORY DIRECTIVES

**IMPORTANT: These directives MUST be followed at ALL times:**

### Git & Version Control
- **NEVER** execute `git add`, `git commit`, `git push`, or `git mv` without explicit user request
- **NEVER** include references to "Claude", "AI", "Generated with Claude", or similar in:
  - Commit messages
  - Co-authored-by tags
  - Code comments (unless discussing AI/ML features)
  - Documentation
- **ALWAYS** prepare commit messages as plain text for user review only
- **ALWAYS** let the user decide when and what to commit
- **USE** conventional commits format (feat:, fix:, docs:, refactor:, test:, chore:)

### Code Quality
- **ALWAYS** run `make test` after significant changes
- **MAINTAIN** minimum 80% test coverage for new code (95%+ for critical code)
- **PREFER** editing existing files over creating new ones
- **USE** typed errors from `pkg/errors/errors.go` (never plain errors)
- **USE** structured logging with zap (never fmt.Println for logs)
- **AVOID** creating documentation files unless explicitly requested
- **AVOID** scattering ad-hoc example/demo files (e.g., `*_example.go`) through the codebase
  - Documentation belongs in `docs/` markdown files
  - Runnable code belongs in tests (`*_test.go`) or the main application
  - Self-contained sample applications live under `examples/` (each with its own `go.mod`), used to exercise the gtool pipeline end-to-end
- **Only** add code comments strictly necessary for understanding complex logic
### File Standards
- **USE** `.yml` extension for all YAML configuration files (industry standard)
- **SUPPORT** `.yaml` for backward compatibility but prefer `.yml`
- **FOLLOW** existing naming conventions in the codebase

### Communication
- Be concise and direct (CLI/terminal context)
- Ask before making architectural changes
- Explain trade-offs when suggesting alternatives
- Only use emojis when explicitly requested by user

---

## Project Overview

GTOOL is a CLI orchestrator written in Go for component testing of microservices. It automates the complete testing pipeline: starting mock services, launching applications, running tests, and cleanup. Phases 1 (Foundation) and 2 (Mock Services) are complete; Phase 3 (App Launcher) is next.

**Key Technology Stack:**
- Go 1.24.9 (toolchain pinned in `go.mod`; use `gvm use go1.24.9`)
- Docker SDK for container management
- Cobra for CLI framework
- Viper for configuration
- Zap for structured logging
- Testify for testing

## Build and Test Commands

```bash
# Build
make build                    # Creates ./bin/gtool
./bin/gtool version          # Verify build

# Testing
make test                    # Run all tests with race detection
make test-coverage           # Generate HTML coverage report
go test ./pkg/...            # Test specific package
go test -run TestName ./...  # Run single test

# Development
make fmt                     # Format code
make lint                    # Run golangci-lint (requires golangci-lint installed)
make clean                   # Remove build artifacts
make mod                     # Download and tidy dependencies

# Running
./bin/gtool config validate --config my-config.yml
./bin/gtool config show --config my-config.yml --format json

# Services (mock management)
./bin/gtool services up                    # Start all configured mocks
./bin/gtool services up postgresql         # Start specific service
./bin/gtool services down                  # Stop all services
./bin/gtool services status                # Show services status
./bin/gtool services logs postgresql       # View service logs
./bin/gtool s up                           # Alias for services
```

## Architecture

### Plugin System
The core architecture uses a plugin-based design with three main plugin interfaces:

1. **ServicePlugin** (`internal/plugin/interface.go`): Mock services (Couchbase, Kafka, etc.)
2. **AppLauncher** (`internal/plugin/interface.go`): Application launchers (Go, Node.js, Generic)
3. **TestExecutor** (`internal/plugin/interface.go`): Test frameworks (Karate for backend testing)

All plugins are registered in a thread-safe **PluginRegistry** (`internal/plugin/registry.go`) that manages plugin lifecycle.

### Configuration System
- Config types defined in `pkg/config/types.go`
- Validation in `internal/core/config/validator.go` with strict schema enforcement
- Loader in `internal/core/config/loader.go` supports YAML/JSON
- Default configuration available via `config.DefaultConfig()`

**Supported values:**
- Versions: `v1`
- App technologies: `golang`, `nodejs`, `generic`
- Test launchers: `test-launcher-back` (Karate for backend API testing)
  - Note: `test-launcher-front` (Cypress) is out of scope - focus is on backend component testing
- Mock services: `mountebank`, `couchbase`, `postgresql`, `kafka`, `pubsub`, `gcs`

### Core Managers
- **MockManager** (`internal/core/mock/manager.go`): Manages mock service lifecycle. Implemented (Phase 2) — wired to the Docker client and plugin registry; unit-tested (~90% coverage).
- **AppLauncher** (`internal/core/app/launcher.go`): Launches applications. Skeleton — Phase 3.
- **TestExecutor** (`internal/core/test/executor.go`): Executes test suites. Skeleton — Phase 4.
- **Orchestrator** (`internal/core/orchestrator/orchestrator.go`): Coordinates the pipeline. Skeleton — Phase 5.

### Error Handling
Use the typed error system in `pkg/errors/errors.go`:
```go
import gtErrors "github.com/oswaldo-montano/gtool/pkg/errors"

// Creating errors
return gtErrors.New(gtErrors.ErrConfigInvalid, "description")

// Wrapping errors
return gtErrors.Wrap(err, gtErrors.ErrDockerFailed, "context")

// Checking error types
if gtErrors.Is(err, gtErrors.ErrConfigNotFound) { ... }
```

Error codes include: `ErrConfigNotFound`, `ErrConfigInvalid`, `ErrInvalidArgument`, `ErrDockerFailed`, `ErrServiceFailed`, etc.

### Logging
Use structured logging via `pkg/logger/logger.go`:
```go
import "go.uber.org/zap"

logger := logger.New()  // or logger.NewDevelopment() for dev
logger.Info("message", zap.String("key", "value"))
logger.Error("error occurred", zap.Error(err))
```

## Code Organization

```
gtool/
├── cmd/gtool/              # Main entry point
├── internal/
│   ├── cli/                # Cobra commands (root, test, mock, app, config, version)
│   ├── core/               # Core business logic
│   │   ├── config/         # Config loader & validator
│   │   ├── mock/           # Mock manager
│   │   ├── app/            # App launcher
│   │   ├── test/           # Test executor
│   │   └── orchestrator/   # Pipeline orchestrator
│   ├── plugin/             # Plugin interfaces & registry
│   └── infra/              # Infrastructure (docker client)
├── pkg/                    # Public packages
│   ├── config/             # Config types
│   ├── errors/             # Error types
│   └── logger/             # Logger wrapper
└── test/                   # Test files and fixtures
```

## Testing Standards

### Requirements
- Minimum 80% coverage for new code
- 95%+ coverage for critical code (config, orchestration)
- Use table-driven tests for multiple test cases
- Follow the pattern in `pkg/config/types_test.go`

### Test Structure
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name        string
        input       InputType
        want        OutputType
        wantErr     bool
        errContains string
    }{
        {
            name:    "valid case",
            input:   validInput,
            want:    expectedOutput,
            wantErr: false,
        },
        {
            name:        "error case",
            input:       invalidInput,
            wantErr:     true,
            errContains: "expected error message",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := FunctionName(tt.input)

            if tt.wantErr {
                require.Error(t, err)
                if tt.errContains != "" {
                    assert.Contains(t, err.Error(), tt.errContains)
                }
                return
            }

            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

## Development Guidelines

### Adding New Features
1. Check `docs/IMPLEMENTATION_ROADMAP.md` for phase planning
2. Implement interfaces from `internal/plugin/interface.go`
3. Register plugins in `PluginRegistry`
4. Write tests achieving >80% coverage
5. Use typed errors from `pkg/errors/errors.go`
6. Add structured logging with zap

### Code Style
- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- Use `gofmt` and `goimports` (run `make fmt`)
- Package names: lowercase, no underscores (e.g., `mockmanager` not `mock_manager`)
- Exported types/functions require godoc comments
- Use conventional commits for messages

### Common Patterns
```go
// Context usage - always pass context
func (m *Manager) Start(ctx context.Context, cfg *config.Config) error {
    // Implementation
}

// Structured logging
m.logger.Info("starting service",
    zap.String("service", name),
    zap.Int("port", port),
)

// Error handling
if err := operation(); err != nil {
    return gtErrors.Wrap(err, gtErrors.ErrServiceFailed, "failed to start service")
}
```

## Configuration Example

See `configs/config.example.yml` for a complete example. Basic structure:

```yaml
version: v1
app-technology: golang
app-config:
  binary-name: myapp
  port: 8080
test-launcher: test-launcher-back
third-party:
  mocks:
    - couchbase
    - mountebank
  mock-config:
    couchbase:
      bucket: test-bucket
```

## Current Phase Status

**Phase 1 (Foundation)** - ✅ Complete
- CLI framework with Cobra (`config`, `services`, `generate`, `version` commands)
- Configuration system with validation (loader + strict validator)
- Structured logging with Zap
- Typed error system (`pkg/errors`, 100% coverage)
- Plugin registry (100% coverage)

**Phase 2 (Mock Services)** - ✅ Complete
- Docker client wrapper (`internal/infra/docker/client.go`), with custom container `Cmd` support
- MockManager lifecycle (`internal/core/mock/manager.go`)
- `services` CLI command (up/down/status/logs), Docker client injected via factory
- All 6 service plugins implemented, registered in `RegisterAll` and validated by build-tagged integration tests:
  - **PostgreSQL** — `psql`/`pg_isready` via ExecInContainer, SQL script seeding
  - **Mountebank** — HTTP admin API, imposter loading from JSON
  - **Kafka** — single-node KRaft, `kafka-topics.sh`, topic creation
  - **Couchbase** — `couchbase-cli` cluster-init (retry-based), bucket creation
  - **Pub/Sub** — emulator via custom `Cmd`, topics/subscriptions over REST
  - **GCS** — fake-gcs-server via custom `Cmd`, bucket creation over the JSON API

**Follow-ups deferred:** Couchbase scopes/collections + JSON data loading; GCS initial object/file seeding.

**Integration tests:** each plugin has a `//go:build integration` test that requires a running Docker daemon. Run with `go test -tags=integration ./internal/plugin/services/...`. They are excluded from the default suite.

See `docs/IMPLEMENTATION_ROADMAP.md` for complete 14-week roadmap.

## Important Notes

- All tests must pass before committing: `make test && make lint`
- Always use the error types from `pkg/errors/errors.go`
- Never use `fmt.Println` for logging - use structured logger
- Docker operations use the `internal/infra/docker/client.go` wrapper
- Configuration validation is strict - see `internal/core/config/validator.go` for supported values
- The orchestrator pipeline is skeleton only - full implementation in Phase 5

## Documentation

Comprehensive documentation is organized in the `docs/` directory:

- **[Documentation Index](docs/README.md)** - Main documentation hub with navigation
- **[Implementation Roadmap](docs/IMPLEMENTATION_ROADMAP.md)** - Project phases and timeline

### Service Plugins

- **[PostgreSQL Service](docs/services/postgresql/)** - Complete PostgreSQL mock service documentation
  - [Quick Start](docs/services/postgresql/quickstart.md) - Get started in 5 minutes
  - [API Reference](docs/services/postgresql/README.md) - Complete plugin documentation
  - [Implementation](docs/services/postgresql/implementation.md) - Technical details
  - [SQL Scripts](docs/services/postgresql/sql-scripts.md) - Script creation guide

### Configuration Examples

- [PostgreSQL Example](configs/postgresql-example.yml) - Full configuration with PostgreSQL mock

See the [docs/README.md](docs/README.md) for complete documentation navigation.
