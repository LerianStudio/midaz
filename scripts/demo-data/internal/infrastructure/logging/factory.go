package logging

import (
	"demo-data/internal/domain/entities"
)

// LoggerFactory creates loggers with different configurations
type LoggerFactory struct{}

// NewLoggerFactory creates a new logger factory instance
func NewLoggerFactory() *LoggerFactory {
	return &LoggerFactory{}
}

// CreateLogger creates a logger from application configuration
func (f *LoggerFactory) CreateLogger(config *entities.Configuration) (Logger, error) {
	return NewLogger(config.Debug, config.LogLevel)
}

// CreateDevelopmentLogger creates a logger configured for development use
func (f *LoggerFactory) CreateDevelopmentLogger() (Logger, error) {
	return NewLogger(true, "debug")
}

// CreateProductionLogger creates a logger configured for production use
func (f *LoggerFactory) CreateProductionLogger() (Logger, error) {
	return NewLogger(false, "info")
}

// CreateTestLogger creates a logger configured for testing
func (f *LoggerFactory) CreateTestLogger() (Logger, error) {
	return NewLogger(true, "warn")
}
