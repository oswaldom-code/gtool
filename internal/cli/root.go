package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/oswaldo-montano/gtool/internal/cli/app"
	"github.com/oswaldo-montano/gtool/internal/cli/config"
	"github.com/oswaldo-montano/gtool/internal/cli/generate"
	"github.com/oswaldo-montano/gtool/internal/cli/services"
	"github.com/oswaldo-montano/gtool/internal/cli/test"
)

var (
	cfgFile  string
	verbose  bool
	logLevel string
)

var rootCmd = &cobra.Command{
	Use:   "gtool",
	Short: "GTOOL - Component testing orchestrator",
	Long: `GTOOL is a CLI tool for orchestrating microservice component tests.

It automates the complete testing pipeline:
  - Launches mock services (Couchbase, PostgreSQL, Kafka, Pub/Sub, GCS, Mountebank)
  - Starts your microservice (Go, Node.js, or any technology)
  - Executes automated tests (Karate, Cypress)
  - Generates detailed reports
  - Cleans up everything automatically

All with a single command: gtool test`,
	Version: "0.1.0",
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./component-config.yml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	checkError(viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")))
	checkError(viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config")))

	// Register subcommands
	rootCmd.AddCommand(generate.NewGenerateCmd())
	rootCmd.AddCommand(services.NewServicesCmd(&cfgFile))
	rootCmd.AddCommand(config.NewConfigCmd(&cfgFile))
	rootCmd.AddCommand(app.NewAppCmd(&cfgFile))
	rootCmd.AddCommand(test.NewTestCmd(&cfgFile))
}

func initLogger() {
	level := viper.GetString("log-level")
	var cfg zap.Config

	if level == "debug" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	err := cfg.Level.UnmarshalText([]byte(level))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level: %v\n", err)
		os.Exit(1)
	}

	logger, err := cfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not initialize logger: %v\n", err)
		os.Exit(1)
	}
	zap.ReplaceGlobals(logger)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("component-config")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("GTOOL")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			_, ok := fmt.Fprintf(os.Stderr, "Config file error: %v\n", err)
			if ok != nil {
				return
			}
			os.Exit(1)
		}
	}
}

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

func Execute() error {
	initLogger()
	return rootCmd.Execute()
}
