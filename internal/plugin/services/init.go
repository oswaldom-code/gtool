package services

import (
	"go.uber.org/zap"

	"github.com/oswaldo-montano/gtool/internal/infra/docker"
	"github.com/oswaldo-montano/gtool/internal/plugin"
	"github.com/oswaldo-montano/gtool/internal/plugin/services/mountebank"
	"github.com/oswaldo-montano/gtool/internal/plugin/services/postgresql"
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

	logger.Info("all service plugins registered successfully")
	return nil
}
