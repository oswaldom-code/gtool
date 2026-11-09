package services

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/oswaldom-code/gtool/internal/infra/docker"
	"github.com/oswaldom-code/gtool/internal/plugin"
	"github.com/oswaldom-code/gtool/internal/plugin/services/couchbase"
	"github.com/oswaldom-code/gtool/internal/plugin/services/floci"
	"github.com/oswaldom-code/gtool/internal/plugin/services/gcs"
	"github.com/oswaldom-code/gtool/internal/plugin/services/kafka"
	"github.com/oswaldom-code/gtool/internal/plugin/services/minio"
	"github.com/oswaldom-code/gtool/internal/plugin/services/mongodb"
	"github.com/oswaldom-code/gtool/internal/plugin/services/mountebank"
	"github.com/oswaldom-code/gtool/internal/plugin/services/mysql"
	"github.com/oswaldom-code/gtool/internal/plugin/services/postgresql"
	"github.com/oswaldom-code/gtool/internal/plugin/services/pubsub"
	"github.com/oswaldom-code/gtool/internal/plugin/services/redis"
	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

// RegisterAll registers every service plugin, or none: names are checked
// first, so a conflict never leaves the registry half filled.
func RegisterAll(registry *plugin.Registry, dockerClient *docker.Client, logger *zap.Logger) error {
	plugins := []plugin.ServicePlugin{
		postgresql.NewPostgreSQLPlugin(dockerClient, logger),
		mountebank.NewMountebankPlugin(dockerClient, logger),
		kafka.NewKafkaPlugin(dockerClient, logger),
		couchbase.NewCouchbasePlugin(dockerClient, logger),
		pubsub.NewPubSubPlugin(dockerClient, logger),
		gcs.NewGCSPlugin(dockerClient, logger),
		redis.NewRedisPlugin(dockerClient, logger),
		mongodb.NewMongoDBPlugin(dockerClient, logger),
		mysql.NewMySQLPlugin(dockerClient, logger),
		minio.NewMinIOPlugin(dockerClient, logger),
		floci.NewFlociPlugin(dockerClient, logger),
	}

	for _, p := range plugins {
		if _, err := registry.GetService(p.Name()); err == nil {
			return gtErrors.New(gtErrors.ErrInvalidArgument, fmt.Sprintf("service plugin %s already registered", p.Name()))
		}
	}
	for _, p := range plugins {
		if err := registry.RegisterService(p); err != nil {
			return err
		}
	}

	logger.Info("all service plugins registered successfully")
	return nil
}
