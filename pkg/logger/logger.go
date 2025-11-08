package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.Logger
}

func New(level string, structured bool) (*Logger, error) {
	var config zap.Config

	if structured {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
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
	return l.Logger.With(fields...)
}

func Default() *Logger {
	logger, _ := New("info", false)
	return logger
}
