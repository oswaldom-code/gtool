package logger

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
		return nil, err
	}
	config.Level = zap.NewAtomicLevelAt(lvl)

	zapLogger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{Logger: zapLogger}, nil
}

func (l *Logger) WithContext(ctx context.Context, fields ...zap.Field) *zap.Logger {
	// TODO: Extract fields from context (trace ID, request ID, etc.)
	return l.With(fields...)
}

func Default() *Logger {
	logger, _ := New("info", false)
	return logger
}
