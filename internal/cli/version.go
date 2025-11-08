package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "0.1.0"
	GitCommit = "dev"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long: `Display version, git commit, and build date information for gtool.

Examples:
  # Show basic version
  gtool version

  # Show full version information including git commit and build date
  gtool version --full`,
	Run: func(cmd *cobra.Command, args []string) {
		full, _ := cmd.Flags().GetBool("full")

		if full {
			fmt.Printf("gtool version %s\n", Version)
			fmt.Printf("Git commit: %s\n", GitCommit)
			fmt.Printf("Build date: %s\n", BuildDate)
		} else {
			fmt.Printf("gtool version %s\n", Version)
		}
	},
}

func init() {
	versionCmd.Flags().Bool("full", false, "Show full version information")
	rootCmd.AddCommand(versionCmd)
}
