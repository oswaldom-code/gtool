package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// GitCommit and BuildDate are injected at link time by `make build`; they stay
// empty in other builds (e.g. go install).
var (
	Version   = "1.0.0"
	GitCommit = ""
	BuildDate = ""
)

const shortCommitLen = 7

// vcsCommit returns the short revision Go stamps into binaries built inside
// the repository, or "" when there is none.
func vcsCommit(settings []debug.BuildSetting) string {
	for _, s := range settings {
		if s.Key == "vcs.revision" && len(s.Value) >= shortCommitLen {
			return s.Value[:shortCommitLen]
		}
	}
	return ""
}

// buildCommit returns the commit injected at link time, else the VCS stamp,
// else "".
func buildCommit() string {
	if GitCommit != "" {
		return GitCommit
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		return vcsCommit(info.Settings)
	}
	return ""
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

// RepositoryURL is the canonical gtool repository.
const RepositoryURL = "https://github.com/oswaldom-code/gtool"

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
			fmt.Printf("Git commit: %s\n", orUnknown(buildCommit()))
			fmt.Printf("Build date: %s\n", orUnknown(BuildDate))
		} else {
			fmt.Printf("gtool version %s\n", Version)
		}
	},
}

func init() {
	versionCmd.Flags().Bool("full", false, "Show full version information")
	rootCmd.AddCommand(versionCmd)
}
