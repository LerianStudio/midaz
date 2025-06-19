package ports

import "context"

// ConfigurationPort defines the interface for configuration management
type ConfigurationPort interface {
	Load(ctx context.Context) (*Configuration, error)
	Validate(ctx context.Context, config *Configuration) error
	GetAPIEndpoints() []string
}

// Configuration represents the application configuration structure
type Configuration struct {
	APIBaseURL     string `json:"api_base_url" validate:"required,url"`
	AuthToken      string `json:"auth_token" validate:"required"`
	TimeoutSeconds int    `json:"timeout_seconds" validate:"min=1,max=300"`
	Debug          bool   `json:"debug"`
	LogLevel       string `json:"log_level" validate:"oneof=debug info warn error"`
}
