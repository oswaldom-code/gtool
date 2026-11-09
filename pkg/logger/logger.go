package logger

import (
	"context"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	gtErrors "github.com/oswaldom-code/gtool/pkg/errors"
)

type Logger struct {
	*zap.Logger
}

func levelEncoder(isTerminal bool, noColor string) zapcore.LevelEncoder {
	if isTerminal && noColor == "" {
		return zapcore.CapitalColorLevelEncoder
	}
	return zapcore.CapitalLevelEncoder
}

var stderrIsTerminal = func() bool {
	info, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func New(level string, structured bool) (*Logger, error) {
	var config zap.Config

	if structured {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = levelEncoder(stderrIsTerminal(), os.Getenv("NO_COLOR"))
	}

	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "invalid log level "+level)
	}
	config.Level = zap.NewAtomicLevelAt(lvl)

	zapLogger, err := config.Build()
	if err != nil {
		return nil, gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "failed to build logger")
	}

	return &Logger{Logger: zapLogger}, nil
}

func (l *Logger) WithContext(ctx context.Context, fields ...zap.Field) *zap.Logger {
	// TODO: Extract fields from context (trace ID, request ID, etc.)
	return l.With(fields...)
}

var (
	levelMu      sync.Mutex
	defaultLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	levelPinned  bool
)

// SetDefaultLevel changes the level of every Default logger, including those
// already created. A pinned level (from a flag or env var) is not overridden
// by ApplyConfigLevel.
func SetDefaultLevel(level string, pin bool) error {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return gtErrors.Wrap(err, gtErrors.ErrInvalidArgument, "invalid log level "+level)
	}
	levelMu.Lock()
	defer levelMu.Unlock()
	defaultLevel.SetLevel(lvl)
	levelPinned = levelPinned || pin
	return nil
}

func ApplyConfigLevel(level string) error {
	levelMu.Lock()
	pinned := levelPinned
	levelMu.Unlock()
	if pinned || level == "" {
		return nil
	}
	return SetDefaultLevel(level, false)
}

func Default() *Logger {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = levelEncoder(stderrIsTerminal(), os.Getenv("NO_COLOR"))
	config.Level = defaultLevel
	zapLogger, err := config.Build()
	if err != nil {
		return &Logger{Logger: zap.NewNop()}
	}
	return &Logger{Logger: zapLogger}
}
