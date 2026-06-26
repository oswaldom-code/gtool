package services

import (
	"go.uber.org/zap"

	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	"github.com/oswaldo-montano/gtool/internal/plugin/services/couchbase"
	"github.com/oswaldo-montano/gtool/internal/plugin/services/gcs"
	"github.com/oswaldo-montano/gtool/internal/plugin/services/kafka"
	"github.com/oswaldo-montano/gtool/internal/plugin/services/mountebank"
	"github.com/oswaldo-montano/gtool/internal/plugin/services/postgresql"
	"github.com/oswaldo-montano/gtool/internal/plugin/services/pubsub"
)

func RegisterAll(registry *plugin.Registry, dockerClient *docker.Client, logger *zap.Logger) error {
	postgresPlugin := postgresql.NewPostgreSQLPlugin(dockerClient, logger)
	if err := registry.RegisterService(postgresPlugin); err != nil {
		return err
	}

	mountebankPlugin := mountebank.NewMountebankPlugin(dockerClient, logger)
	if err := registry.RegisterService(mountebankPlugin); err != nil {
		return err
	}

	kafkaPlugin := kafka.NewKafkaPlugin(dockerClient, logger)
	if err := registry.RegisterService(kafkaPlugin); err != nil {
		return err
	}

	couchbasePlugin := couchbase.NewCouchbasePlugin(dockerClient, logger)
	if err := registry.RegisterService(couchbasePlugin); err != nil {
		return err
	}

	pubsubPlugin := pubsub.NewPubSubPlugin(dockerClient, logger)
	if err := registry.RegisterService(pubsubPlugin); err != nil {
		return err
	}

	gcsPlugin := gcs.NewGCSPlugin(dockerClient, logger)
	if err := registry.RegisterService(gcsPlugin); err != nil {
		return err
	}

	logger.Info("all service plugins registered successfully")
	return nil
}
