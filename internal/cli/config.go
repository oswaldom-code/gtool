package cli

import (
	"encoding/json"
	"fmt"

	"github.com/oswaldo-montano/gtool/pkg/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	coreConfig "github.com/oswaldo-montano/gtool/internal/core/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
	Long:  `Validate, show, and manage configuration files.`,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	Long:  `Validate the configuration file against the schema and business rules.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, _ := cmd.Flags().GetString("config")

		loader := coreConfig.NewLoader()
		var cfg *config.Config
		var err error

		if configPath != "" {
			cfg, err = loader.Load(configPath)
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

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current configuration with all resolved values.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, _ := cmd.Flags().GetString("config")
		format, _ := cmd.Flags().GetString("format")

		loader := coreConfig.NewLoader()
		var cfg *config.Config
		var err error

		if configPath != "" {
			cfg, err = loader.Load(configPath)
		} else {
			cfg, err = loader.LoadFromPath()
		}

		if err != nil {
			fmt.Printf("Failed to load configuration: %v\n", err)
			return err
		}

		switch format {
		case "json":
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal JSON: %w", err)
			}
			fmt.Println(string(data))
		case "yaml":
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("failed to marshal YAML: %w", err)
			}
			fmt.Println(string(data))
		default:
			return fmt.Errorf("unsupported format: %s (use 'json' or 'yaml')", format)
		}

		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new configuration",
	Long:  `Create a new configuration file from a template.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Initializing configuration - No implementation yet")
		return nil
	},
}

var configMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate configuration from component",
	Long:  `Migrate an existing component-config.yml to gtool format.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Migrating configuration - No implementation yet")
		return nil
	},
}

func init() {
	// Show flags
	configShowCmd.Flags().String("format", "yaml", "Output format (yaml, json)")

	// Init flags
	configInitCmd.Flags().String("template", "golang", "Template to use (golang, nodejs, generic)")

	// Migrate flags
	configMigrateCmd.Flags().String("from", "component-config.yml", "Source configuration file")

	// Add subcommands
	configCmd.AddCommand(configValidateCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configMigrateCmd)

	rootCmd.AddCommand(configCmd)
}
