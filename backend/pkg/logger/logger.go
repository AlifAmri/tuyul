package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger wraps zerolog logger
type Logger struct {
	logger zerolog.Logger
}

// New creates a new logger instance
func New(level, format string) *Logger {
	// Set log level
	logLevel := parseLogLevel(level)
	zerolog.SetGlobalLevel(logLevel)

	// Configure output format
	var output zerolog.Logger
	if format == "pretty" {
		output = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Caller().Logger()
	} else {
		output = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	}

	return &Logger{
		logger: output,
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.logger.Debug().Msg(msg)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.logger.Debug().Msgf(format, args...)
}

// Info logs an info message
func (l *Logger) Info(msg string) {
	l.logger.Info().Msg(msg)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.logger.Info().Msgf(format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.logger.Warn().Msg(msg)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.logger.Warn().Msgf(format, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, err error) {
	l.logger.Error().Err(err).Msg(msg)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.logger.Error().Msgf(format, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, err error) {
	l.logger.Fatal().Err(err).Msg(msg)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.logger.Fatal().Msgf(format, args...)
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		logger: l.logger.With().Interface(key, value).Logger(),
	}
}

// WithFields adds multiple fields to the logger
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	ctx := l.logger.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &Logger{
		logger: ctx.Logger(),
	}
}

// parseLogLevel parses string log level to zerolog level
func parseLogLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

// Global logger instance
var globalLogger *Logger

// Init initializes the global logger
func Init(level, format string) {
	globalLogger = New(level, format)
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if globalLogger == nil {
		globalLogger = New("info", "json")
	}
	return globalLogger
}

// Convenience functions for global logger

// Debug logs a debug message using global logger
func Debug(msg string) {
	GetLogger().Debug(msg)
}

// Debugf logs a formatted debug message using global logger
func Debugf(format string, args ...interface{}) {
	GetLogger().Debugf(format, args...)
}

// Info logs an info message using global logger
func Info(msg string) {
	GetLogger().Info(msg)
}

// Infof logs a formatted info message using global logger
func Infof(format string, args ...interface{}) {
	GetLogger().Infof(format, args...)
}

// Warn logs a warning message using global logger
func Warn(msg string) {
	GetLogger().Warn(msg)
}

// Warnf logs a formatted warning message using global logger
func Warnf(format string, args ...interface{}) {
	GetLogger().Warnf(format, args...)
}

// Error logs an error message using global logger
func Error(msg string, err error) {
	GetLogger().Error(msg, err)
}

// Errorf logs a formatted error message using global logger
func Errorf(format string, args ...interface{}) {
	GetLogger().Errorf(format, args...)
}

// Fatal logs a fatal message and exits using global logger
func Fatal(msg string, err error) {
	GetLogger().Fatal(msg, err)
}

// Fatalf logs a formatted fatal message and exits using global logger
func Fatalf(format string, args ...interface{}) {
	GetLogger().Fatalf(format, args...)
}

