// Package logger provides zap logger factories for production, development,
// and test contexts. Production loggers honor the LOG_LEVEL environment
// variable (debug/info/warn/error); unrecognized values fall back to info
// and emit a warning at startup so misconfiguration is visible.
package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const envLogLevel = "LOG_LEVEL"

// GetLogger returns a JSON-encoded production logger configured from the
// LOG_LEVEL environment variable. Unrecognized levels fall back to info
// and the logger emits a one-shot warning at construction time.
func GetLogger() *zap.Logger {
	raw := os.Getenv(envLogLevel)
	level, ok := parseLevel(raw)

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	l, err := cfg.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build logger: %v", err))
	}
	if !ok && raw != "" {
		l.Warn("unrecognized LOG_LEVEL; defaulting to info",
			zap.String("value", raw),
		)
	}
	return l
}

// GetDebugLogger returns a development-style logger (human-readable, debug
// level). Useful for local development; do not use in production.
func GetDebugLogger() *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	logger, err := cfg.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build debug logger: %v", err))
	}
	return logger
}

// GetTestLogger returns a no-op logger suitable for unit tests.
func GetTestLogger() *zap.Logger {
	return zap.NewNop()
}

func parseLevel(raw string) (zapcore.Level, bool) {
	switch raw {
	case "debug":
		return zapcore.DebugLevel, true
	case "info", "":
		return zapcore.InfoLevel, true
	case "warn":
		return zapcore.WarnLevel, true
	case "error":
		return zapcore.ErrorLevel, true
	}
	return zapcore.InfoLevel, false
}
