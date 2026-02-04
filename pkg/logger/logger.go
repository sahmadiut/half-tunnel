// Package logger provides a structured logging wrapper for the Half-Tunnel system.
package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger wraps zerolog.Logger for structured logging.
type Logger struct {
	zl zerolog.Logger
}

// Config holds logger configuration.
type Config struct {
	// Level sets the minimum log level: debug, info, warn, error
	Level string
	// Format sets the output format: json, console
	Format string
	// Output sets the output destination (file path or empty for stdout)
	Output string
	// Fields are additional fields to add to all log entries
	Fields map[string]interface{}
}

// New creates a new logger with the given configuration.
func New(cfg Config) (*Logger, error) {
	var output io.Writer = os.Stdout

	// Set up output
	if cfg.Output != "" {
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, err
		}
		output = file
	}

	// Set up format
	if cfg.Format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
		}
	}

	// Set up level
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	// Create logger
	zl := zerolog.New(output).With().Timestamp().Logger()

	// Add additional fields
	if len(cfg.Fields) > 0 {
		ctx := zl.With()
		for k, v := range cfg.Fields {
			ctx = ctx.Interface(k, v)
		}
		zl = ctx.Logger()
	}

	return &Logger{zl: zl}, nil
}

// NewDefault creates a logger with default configuration.
func NewDefault() *Logger {
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}
	zl := zerolog.New(output).With().Timestamp().Logger()
	return &Logger{zl: zl}
}

// parseLevel converts a string log level to zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// Debug logs a debug message.
func (l *Logger) Debug() *zerolog.Event {
	return l.zl.Debug()
}

// Info logs an info message.
func (l *Logger) Info() *zerolog.Event {
	return l.zl.Info()
}

// Warn logs a warning message.
func (l *Logger) Warn() *zerolog.Event {
	return l.zl.Warn()
}

// Error logs an error message.
func (l *Logger) Error() *zerolog.Event {
	return l.zl.Error()
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal() *zerolog.Event {
	return l.zl.Fatal()
}

// With returns a new logger with the given key-value pair added.
func (l *Logger) With(key string, value interface{}) *Logger {
	return &Logger{zl: l.zl.With().Interface(key, value).Logger()}
}

// WithStr returns a new logger with the given string key-value pair added.
func (l *Logger) WithStr(key, value string) *Logger {
	return &Logger{zl: l.zl.With().Str(key, value).Logger()}
}

// WithError returns a new logger with the given error added.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{zl: l.zl.With().Err(err).Logger()}
}

// WithFields returns a new logger with the given fields added.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	ctx := l.zl.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &Logger{zl: ctx.Logger()}
}

// WithDuration returns a new logger with the given duration added.
func (l *Logger) WithDuration(key string, d time.Duration) *Logger {
	return &Logger{zl: l.zl.With().Dur(key, d).Logger()}
}

// WithBytes returns a new logger with the given byte count added.
func (l *Logger) WithBytes(key string, b int64) *Logger {
	return &Logger{zl: l.zl.With().Int64(key, b).Logger()}
}
