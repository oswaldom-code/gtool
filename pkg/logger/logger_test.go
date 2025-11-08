package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		level      string
		structured bool
		wantErr    bool
	}{
		{
			name:       "valid info level structured",
			level:      "info",
			structured: true,
			wantErr:    false,
		},
		{
			name:       "valid debug level",
			level:      "debug",
			structured: false,
			wantErr:    false,
		},
		{
			name:       "valid warn level",
			level:      "warn",
			structured: true,
			wantErr:    false,
		},
		{
			name:       "valid error level",
			level:      "error",
			structured: false,
			wantErr:    false,
		},
		{
			name:       "invalid level",
			level:      "invalid",
			structured: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.level, tt.structured)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
				assert.NotNil(t, logger.Logger)
			}
		})
	}
}

func TestDefault(t *testing.T) {
	logger := Default()
	assert.NotNil(t, logger)
	assert.NotNil(t, logger.Logger)
}

func TestWithContext(t *testing.T) {
	logger, err := New("info", true)
	require.NoError(t, err)
	require.NotNil(t, logger)

	ctx := context.Background()
	field1 := zap.String("key1", "value1")
	field2 := zap.Int("key2", 42)

	contextLogger := logger.WithContext(ctx, field1, field2)
	assert.NotNil(t, contextLogger)
}

func TestLoggerLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := New(level, true)
			require.NoError(t, err)
			require.NotNil(t, logger)

			assert.NotPanics(t, func() {
				logger.Debug("debug message")
				logger.Info("info message")
				logger.Warn("warn message")
				logger.Error("error message")
			})
		})
	}
}

func TestLoggerStructuredVsPlain(t *testing.T) {
	t.Run("structured logger", func(t *testing.T) {
		logger, err := New("info", true)
		require.NoError(t, err)
		require.NotNil(t, logger)

		assert.NotPanics(t, func() {
			logger.Info("test message", zap.String("key", "value"))
		})
	})

	t.Run("plain logger", func(t *testing.T) {
		logger, err := New("info", false)
		require.NoError(t, err)
		require.NotNil(t, logger)

		assert.NotPanics(t, func() {
			logger.Info("test message", zap.String("key", "value"))
		})
	})
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = New("info", true)
	}
}

func BenchmarkDefault(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Default()
	}
}

func BenchmarkLogging(b *testing.B) {
	logger, _ := New("info", true)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", zap.Int("iteration", i))
	}
}
