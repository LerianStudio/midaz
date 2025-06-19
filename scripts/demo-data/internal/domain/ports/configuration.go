package ports

import (
	"context"

	"demo-data/internal/domain/entities"
)

// ConfigurationPort defines the interface for configuration management
type ConfigurationPort interface {
	Load(ctx context.Context) (*entities.Configuration, error)
	Validate(ctx context.Context, config *entities.Configuration) error
	GetAPIEndpoints() []string
}
