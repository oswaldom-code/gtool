package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/oswaldom-code/gtool/pkg/logger"
)

// newFlagCmd mirrors the root persistent flags on a throwaway command and
// restores the package state and the shared log level afterwards.
func newFlagCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "")
	t.Cleanup(func() {
		logLevel, verbose = "info", false
		require.NoError(t, logger.SetDefaultLevel("info", false))
	})
	return cmd
}

func debugEnabled() bool {
	return logger.Default().Core().Enabled(zapcore.DebugLevel)
}

func TestConfigureLogging(t *testing.T) {
	t.Run("explicit --log-level wins", func(t *testing.T) {
		cmd := newFlagCmd(t)
		t.Setenv("GTOOL_LOG_LEVEL", "error")
		require.NoError(t, cmd.Flags().Set("log-level", "debug"))
		verbose = false

		require.NoError(t, configureLogging(cmd))
		assert.True(t, debugEnabled())
	})

	t.Run("invalid --log-level is an error", func(t *testing.T) {
		cmd := newFlagCmd(t)
		require.NoError(t, cmd.Flags().Set("log-level", "bogus"))
		assert.Error(t, configureLogging(cmd))
	})

	t.Run("--verbose means debug", func(t *testing.T) {
		cmd := newFlagCmd(t)
		verbose = true
		require.NoError(t, configureLogging(cmd))
		assert.True(t, debugEnabled())
	})

	t.Run("GTOOL_LOG_LEVEL applies without flags", func(t *testing.T) {
		cmd := newFlagCmd(t)
		t.Setenv("GTOOL_LOG_LEVEL", "debug")
		require.NoError(t, configureLogging(cmd))
		assert.True(t, debugEnabled())
	})

	t.Run("nothing set keeps info", func(t *testing.T) {
		cmd := newFlagCmd(t)
		t.Setenv("GTOOL_LOG_LEVEL", "")
		require.NoError(t, configureLogging(cmd))
		assert.False(t, debugEnabled())
	})
}

func TestRootVersionUsesBuildVersion(t *testing.T) {
	assert.Equal(t, Version, rootCmd.Version)
}
