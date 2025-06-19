package di

import "demo-data/internal/domain/ports"

// Container provides dependency injection for the application
type Container struct {
	configurationPort ports.ConfigurationPort
}

// NewContainer creates a new dependency injection container
func NewContainer() *Container {
	return &Container{}
}

// SetConfigurationPort sets the configuration port implementation
func (c *Container) SetConfigurationPort(port ports.ConfigurationPort) {
	c.configurationPort = port
}

// GetConfigurationPort returns the configuration port implementation
func (c *Container) GetConfigurationPort() ports.ConfigurationPort {
	return c.configurationPort
}
