package config

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	coreConfig "github.com/oswaldo-montano/gtool/internal/core/config"
	"github.com/oswaldo-montano/gtool/pkg/config"
)

// NewConfigCmd creates the config command
func NewConfigCmd(configFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration management",
		Long:  `Validate, show, and manage configuration files.`,
	}

	// Add subcommands
	cmd.AddCommand(newConfigValidateCmd(configFile))
	cmd.AddCommand(newConfigShowCmd(configFile))

	return cmd
}

func newConfigValidateCmd(configFile *string) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		Long:  `Validate the configuration file against the schema and business rules.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader := coreConfig.NewLoader()
			var cfg *config.Config
			var err error

			// Use the config file from the global flag
			if configFile != nil && *configFile != "" {
				cfg, err = loader.Load(*configFile)
			} else {
				cfg, err = loader.LoadFromPath()
			}

			if err != nil {
				fmt.Printf("❌ Failed to load configuration: %v\n", err)
				return err
			}

			fmt.Printf("✓ Configuration loaded successfully\n")

			validator := coreConfig.NewValidator()
			if err := validator.Validate(cfg); err != nil {
				fmt.Printf("❌ Validation failed:\n%v\n", err)
				return err
			}

			fmt.Printf("✓ Configuration is valid\n")
			fmt.Printf("\nSummary:\n")
			fmt.Printf("  Version: %s\n", cfg.Version)
			fmt.Printf("  Technology: %s\n", cfg.AppTechnology)
			fmt.Printf("  Test Launcher: %s\n", cfg.TestLauncher)
			fmt.Printf("  Mock Services: %d configured\n", len(cfg.ThirdParty.Mocks))

			return nil
		},
	}
}

func newConfigShowCmd(configFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Long:  `Display the current configuration with all resolved values.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")

			loader := coreConfig.NewLoader()
			var cfg *config.Config
			var err error

			// Use the config file from the global flag
			if configFile != nil && *configFile != "" {
				cfg, err = loader.Load(*configFile)
			} else {
				cfg, err = loader.LoadFromPath()
			}

			if err != nil {
				fmt.Printf("Failed to load configuration: %v\n", err)
				return err
			}

			var output []byte

			switch format {
			case "json":
				output, err = json.MarshalIndent(cfg, "", "  ")
			case "yml":
				output, err = yaml.Marshal(cfg)
			default:
				return fmt.Errorf("unsupported format: %s (use 'json' or 'yml')", format)
			}

			if err != nil {
				return fmt.Errorf("failed to marshal configuration: %w", err)
			}

			fmt.Println(string(output))
			return nil
		},
	}

	cmd.Flags().StringP("format", "f", "yml", "output format (yml or json)")

	return cmd
}
