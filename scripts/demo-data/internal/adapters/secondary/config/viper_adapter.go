package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"

	"demo-data/internal/domain/entities"
	"demo-data/internal/domain/ports"
)

// ViperConfigAdapter implements the ConfigurationPort using Viper
type ViperConfigAdapter struct {
	viper     *viper.Viper
	validator *validator.Validate
}

// NewViperConfigAdapter creates a new Viper-based configuration adapter
func NewViperConfigAdapter() ports.ConfigurationPort {
	v := viper.New()

	// Set up environment variable prefix
	v.SetEnvPrefix("DEMO_DATA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set up file configuration
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./configs")
	v.AddConfigPath("$HOME/.demo-data")

	return &ViperConfigAdapter{
		viper:     v,
		validator: validator.New(),
	}
}

// Load loads configuration from all sources (files, environment, defaults)
func (a *ViperConfigAdapter) Load(ctx context.Context) (*entities.Configuration, error) {
	config := entities.NewConfiguration()

	// Try to read config file (optional)
	if err := a.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK, we'll use environment variables and defaults
	}

	// Set default values in viper
	a.setDefaults()

	// Unmarshal into configuration struct
	if err := a.viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return config, nil
}

// Validate validates the configuration using struct tags and custom logic
func (a *ViperConfigAdapter) Validate(ctx context.Context, config *entities.Configuration) error {
	// Struct tag validation
	if err := a.validator.Struct(config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Custom validation logic
	if err := config.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	return nil
}

// GetAPIEndpoints returns the list of known API endpoints
func (a *ViperConfigAdapter) GetAPIEndpoints() []string {
	return []string{
		"/v1/organizations",
		"/v1/health",
		"/v1/auth/validate",
		"/v1/ledgers",
		"/v1/assets",
		"/v1/portfolios",
		"/v1/segments",
		"/v1/accounts",
		"/v1/transactions",
		"/v1/operations",
	}
}

// setDefaults sets default values in viper
func (a *ViperConfigAdapter) setDefaults() {
	a.viper.SetDefault("api_base_url", "https://api.midaz.io")
	a.viper.SetDefault("timeout_duration", "30s")
	a.viper.SetDefault("debug", false)
	a.viper.SetDefault("log_level", "info")

	// Volume config defaults - Small
	a.viper.SetDefault("volume_config.small.organizations", 2)
	a.viper.SetDefault("volume_config.small.ledgers_per_org", 2)
	a.viper.SetDefault("volume_config.small.assets_per_ledger", 3)
	a.viper.SetDefault("volume_config.small.portfolios_per_ledger", 2)
	a.viper.SetDefault("volume_config.small.segments_per_ledger", 2)
	a.viper.SetDefault("volume_config.small.accounts_per_ledger", 100)
	a.viper.SetDefault("volume_config.small.transactions_per_account", 20)

	// Volume config defaults - Medium
	a.viper.SetDefault("volume_config.medium.organizations", 5)
	a.viper.SetDefault("volume_config.medium.ledgers_per_org", 3)
	a.viper.SetDefault("volume_config.medium.assets_per_ledger", 5)
	a.viper.SetDefault("volume_config.medium.portfolios_per_ledger", 3)
	a.viper.SetDefault("volume_config.medium.segments_per_ledger", 4)
	a.viper.SetDefault("volume_config.medium.accounts_per_ledger", 500)
	a.viper.SetDefault("volume_config.medium.transactions_per_account", 50)

	// Volume config defaults - Large
	a.viper.SetDefault("volume_config.large.organizations", 10)
	a.viper.SetDefault("volume_config.large.ledgers_per_org", 5)
	a.viper.SetDefault("volume_config.large.assets_per_ledger", 8)
	a.viper.SetDefault("volume_config.large.portfolios_per_ledger", 5)
	a.viper.SetDefault("volume_config.large.segments_per_ledger", 6)
	a.viper.SetDefault("volume_config.large.accounts_per_ledger", 1000)
	a.viper.SetDefault("volume_config.large.transactions_per_account", 100)
}
