package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oswaldom-code/gtool/internal/cli/app"
	"github.com/oswaldom-code/gtool/internal/cli/config"
	"github.com/oswaldom-code/gtool/internal/cli/generate"
	"github.com/oswaldom-code/gtool/internal/cli/services"
	"github.com/oswaldom-code/gtool/internal/cli/stress"
	"github.com/oswaldom-code/gtool/internal/cli/test"
	"github.com/oswaldom-code/gtool/internal/cli/unit"
	coreStress "github.com/oswaldom-code/gtool/internal/core/stress"
	pluginServices "github.com/oswaldom-code/gtool/internal/plugin/services"
	"github.com/oswaldom-code/gtool/pkg/logger"
)

var (
	cfgFile  string
	verbose  bool
	logLevel string
)

// configureLogging applies the log level by precedence: --log-level, then
// --verbose (debug), then GTOOL_LOG_LEVEL. The config file level applies only
// when none of them is set (see cliconfig.Load).
func configureLogging(cmd *cobra.Command) error {
	if f := cmd.Flag("log-level"); f != nil && f.Changed {
		return logger.SetDefaultLevel(logLevel, true)
	}
	if verbose {
		return logger.SetDefaultLevel("debug", true)
	}
	if env := os.Getenv("GTOOL_LOG_LEVEL"); env != "" {
		return logger.SetDefaultLevel(env, true)
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:   "gtool",
	Short: "GTOOL - Component testing orchestrator",
	Long: `GTOOL is a CLI tool for orchestrating microservice component tests.

It automates the complete testing pipeline:
  - Launches mock services (` + strings.Join(pluginServices.Names(), ", ") + `)
  - Starts your microservice as a Docker container
  - Executes the Karate API tests
  - Generates detailed reports
  - Cleans up everything automatically

All with a single command: gtool test`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		return configureLogging(cmd)
	},
}

func init() {
	rootCmd.Version = Version

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./component-config.yml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	// Register subcommands
	rootCmd.AddCommand(generate.NewGenerateCmd())
	rootCmd.AddCommand(services.NewServicesCmd(&cfgFile))
	rootCmd.AddCommand(config.NewConfigCmd(&cfgFile))
	rootCmd.AddCommand(app.NewAppCmd(&cfgFile))
	rootCmd.AddCommand(test.NewTestCmd(&cfgFile))
	rootCmd.AddCommand(stress.NewStressCmd(&cfgFile, coreStress.ToolInfo{
		Name:       "gtool",
		Version:    Version,
		Commit:     GitCommit,
		BuildDate:  BuildDate,
		Repository: RepositoryURL,
	}))
	rootCmd.AddCommand(unit.NewUnitCmd())
}

func Execute() error {
	return rootCmd.Execute()
}
