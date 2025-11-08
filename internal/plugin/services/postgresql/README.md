# PostgreSQL Service Plugin

> **Documentation has moved!**
>
> For complete PostgreSQL plugin documentation, please visit:
> **[docs/services/postgresql/](../../../../docs/services/postgresql/)**

## Quick Links

- **[Quick Start Guide](../../../../docs/services/postgresql/quickstart.md)** - Get started in 5 minutes
- **[Plugin Documentation](../../../../docs/services/postgresql/README.md)** - Complete API reference
- **[Implementation Details](../../../../docs/services/postgresql/implementation.md)** - Technical architecture
- **[SQL Scripts Guide](../../../../docs/services/postgresql/sql-scripts.md)** - How to create SQL scripts

## Quick Example

```go
import (
    "context"
    "github.com/oswaldo-montano/gtool/internal/infra/docker"
    "github.com/oswaldo-montano/gtool/internal/plugin/services/postgresql"
)

// Create plugin
dockerClient, _ := docker.NewClient(logger)
plugin := postgresql.NewPostgreSQLPlugin(dockerClient, logger)

// Launch PostgreSQL
ctx := context.Background()
config := map[string]interface{}{
    "port":         "5432",
    "scripts-path": "./test/component/mocks-data/postgresql",
}
plugin.Launch(ctx, config)
defer plugin.Stop(ctx)
```

## Configuration

```yaml
third-party:
  mocks:
    - postgresql
  mock-config:
    postgresql:
      port: 5432
      scripts-path: ./test/component/mocks-data/postgresql
```

For more details, see the [full documentation](../../../../docs/services/postgresql/).
