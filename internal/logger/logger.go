package logger

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.SugaredLogger

// Init initializes the global logger
func Init(level string) error {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(zapLevel),
		Development:      false,
		Encoding:         "console",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "T",
			LevelKey:       "L",
			NameKey:        "N",
			CallerKey:      "C",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "M",
			StacktraceKey:  "S",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := config.Build()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	log = logger.Sugar()
	return nil
}

// Debug logs a debug message
func Debug(msg string, keysAndValues ...interface{}) {
	if log != nil {
		log.Debugw(msg, keysAndValues...)
	}
}

// Info logs an info message
func Info(msg string, keysAndValues ...interface{}) {
	if log != nil {
		log.Infow(msg, keysAndValues...)
	}
}

// Warn logs a warning message
func Warn(msg string, keysAndValues ...interface{}) {
	if log != nil {
		log.Warnw(msg, keysAndValues...)
	}
}

// Error logs an error message
func Error(msg string, keysAndValues ...interface{}) {
	if log != nil {
		log.Errorw(msg, keysAndValues...)
	}
}

// Fatal logs a fatal message and exits
func Fatal(msg string, keysAndValues ...interface{}) {
	if log != nil {
		log.Fatalw(msg, keysAndValues...)
	}
}

// With creates a new logger with additional fields
func With(keysAndValues ...interface{}) *zap.SugaredLogger {
	if log == nil {
		return zap.NewNop().Sugar()
	}
	return log.With(keysAndValues...)
}

// Sync flushes any buffered log entries
func Sync() error {
	if log != nil {
		return log.Sync()
	}
	return nil
}
