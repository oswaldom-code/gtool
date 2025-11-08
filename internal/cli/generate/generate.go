package generate

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed templates/component-config.yml
var configTemplate string

var (
	outputFile string
	force      bool
)

// NewGenerateCmd creates the generate command
func NewGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generate code and configuration files",
		Long: `Generate various files for your project.

Available generators:
  - config: Generate a basic component-config.yml file

Examples:
  gtool generate config              # Generate component-config.yml
  gtool g config                     # Short form
  gtool g config -o my-config.yml    # Custom output file
  gtool g config --force             # Overwrite existing file`,
	}

	// Add subcommands
	cmd.AddCommand(newGenerateConfigCmd())

	return cmd
}

// newGenerateConfigCmd creates the config subcommand
func newGenerateConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Generate a basic configuration file",
		Long: `Generate a basic component-config.yml file with examples.

The generated file includes:
  - Basic structure with all sections
  - Comments explaining each option
  - Example values for PostgreSQL
  - Commented examples for other services

Examples:
  gtool generate config                    # Create component-config.yml
  gtool g config                           # Short form
  gtool g config -o my-config.yml          # Custom filename
  gtool g config --force                   # Overwrite if exists`,
		RunE: runGenerateConfig,
	}

	// Add flags
	cmd.Flags().StringVarP(&outputFile, "output", "o", "component-config.yml", "output file path")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite existing file")

	return cmd
}

func runGenerateConfig(cmd *cobra.Command, args []string) error {
	// Check if file exists
	if _, err := os.Stat(outputFile); err == nil && !force {
		return fmt.Errorf("file %s already exists. Use --force to overwrite", outputFile)
	}

	// Get absolute path
	absPath, err := filepath.Abs(outputFile)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Write file using embedded template
	if err := os.WriteFile(outputFile, []byte(configTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("✅ Configuration file created: %s\n\n", absPath)
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit the configuration file to match your needs")
	fmt.Println("  2. Validate: gtool config validate")
	fmt.Println("  3. Start services: gtool s up")

	return nil
}
