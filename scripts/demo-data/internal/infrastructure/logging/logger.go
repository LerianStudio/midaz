package logging

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger interface defines the logging contract for the application
type Logger interface {
	Debug(msg string, fields ...any)
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
	Fatal(msg string, fields ...any)

	With(fields ...any) Logger
	WithContext(ctx context.Context) Logger

	Sync() error
}

// ZapLogger wraps the Zap logger to implement our Logger interface
type ZapLogger struct {
	logger *zap.SugaredLogger
}

// NewLogger creates a new logger instance with the specified configuration
func NewLogger(development bool, level string) (Logger, error) {
	var config zap.Config

	if development {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
		config.EncoderConfig.TimeKey = "timestamp"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	// Set log level
	logLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		logLevel = zapcore.InfoLevel
	}
	config.Level = zap.NewAtomicLevelAt(logLevel)

	// Build logger
	zapLogger, err := config.Build(
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return &ZapLogger{
		logger: zapLogger.Sugar(),
	}, nil
}

// Debug logs a debug-level message with optional structured fields
func (l *ZapLogger) Debug(msg string, fields ...any) {
	l.logger.Debugw(msg, fields...)
}

// Info logs an info-level message with optional structured fields
func (l *ZapLogger) Info(msg string, fields ...any) {
	l.logger.Infow(msg, fields...)
}

// Warn logs a warning-level message with optional structured fields
func (l *ZapLogger) Warn(msg string, fields ...any) {
	l.logger.Warnw(msg, fields...)
}

// Error logs an error-level message with optional structured fields
func (l *ZapLogger) Error(msg string, fields ...any) {
	l.logger.Errorw(msg, fields...)
}

// Fatal logs a fatal-level message with optional structured fields and exits
func (l *ZapLogger) Fatal(msg string, fields ...any) {
	l.logger.Fatalw(msg, fields...)
}

// With adds structured fields to the logger and returns a new logger instance
func (l *ZapLogger) With(fields ...any) Logger {
	return &ZapLogger{
		logger: l.logger.With(fields...),
	}
}

// WithContext adds context fields to the logger and returns a new logger instance
func (l *ZapLogger) WithContext(ctx context.Context) Logger {
	// Extract context values and add them as fields
	fields := extractContextFields(ctx)
	return &ZapLogger{
		logger: l.logger.With(fields...),
	}
}

// Sync flushes any buffered log entries
func (l *ZapLogger) Sync() error {
	return l.logger.Sync()
}

// extractContextFields extracts common context values for structured logging
func extractContextFields(ctx context.Context) []any {
	var fields []any

	// Extract common context values
	if requestID := ctx.Value("request_id"); requestID != nil {
		fields = append(fields, "request_id", requestID)
	}

	if userID := ctx.Value("user_id"); userID != nil {
		fields = append(fields, "user_id", userID)
	}

	if operationID := ctx.Value("operation_id"); operationID != nil {
		fields = append(fields, "operation_id", operationID)
	}

	if sessionID := ctx.Value("session_id"); sessionID != nil {
		fields = append(fields, "session_id", sessionID)
	}

	if traceID := ctx.Value("trace_id"); traceID != nil {
		fields = append(fields, "trace_id", traceID)
	}

	return fields
}
