package di

import (
	"fmt"

	"demo-data/internal/adapters/secondary/sdk"
	"demo-data/internal/domain/entities"
	"demo-data/internal/domain/ports"
	"demo-data/internal/infrastructure/logging"
)

// Container provides dependency injection for the application
type Container struct {
	configurationPort ports.ConfigurationPort
	configuration     *entities.Configuration
	midazClientPort   ports.MidazClientPort
	logger            logging.Logger
	loggerFactory     *logging.LoggerFactory
}

// NewContainer creates a new dependency injection container
func NewContainer() *Container {
	return &Container{
		loggerFactory: logging.NewLoggerFactory(),
	}
}

// SetConfigurationPort sets the configuration port implementation
func (c *Container) SetConfigurationPort(port ports.ConfigurationPort) {
	c.configurationPort = port
}

// GetConfigurationPort returns the configuration port implementation
func (c *Container) GetConfigurationPort() ports.ConfigurationPort {
	return c.configurationPort
}

// SetConfiguration sets the loaded configuration
func (c *Container) SetConfiguration(config *entities.Configuration) {
	c.configuration = config
}

// GetConfiguration returns the loaded configuration
func (c *Container) GetConfiguration() *entities.Configuration {
	return c.configuration
}

// GetMidazClientPort returns the Midaz client port, creating it if necessary
func (c *Container) GetMidazClientPort() (ports.MidazClientPort, error) {
	if c.midazClientPort == nil {
		if c.configuration == nil {
			return nil, fmt.Errorf("configuration is required to create Midaz client")
		}

		client, err := sdk.NewMidazClientAdapter(c.configuration)
		if err != nil {
			return nil, fmt.Errorf("failed to create Midaz client: %w", err)
		}

		c.midazClientPort = client
	}

	return c.midazClientPort, nil
}

// SetMidazClientPort sets the Midaz client port implementation
func (c *Container) SetMidazClientPort(port ports.MidazClientPort) {
	c.midazClientPort = port
}

// SetLogger sets the logger implementation
func (c *Container) SetLogger(logger logging.Logger) {
	c.logger = logger
}

// GetLogger returns the logger implementation
func (c *Container) GetLogger() logging.Logger {
	return c.logger
}

// CreateLoggerFromConfig creates a logger from the current configuration
func (c *Container) CreateLoggerFromConfig(config *entities.Configuration) (logging.Logger, error) {
	return c.loggerFactory.CreateLogger(config)
}

// GetLoggerFactory returns the logger factory
func (c *Container) GetLoggerFactory() *logging.LoggerFactory {
	return c.loggerFactory
}
