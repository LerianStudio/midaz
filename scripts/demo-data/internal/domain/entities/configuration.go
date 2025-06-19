package entities

import (
	"errors"
	"time"
)

// Configuration represents the complete application configuration
type Configuration struct {
	APIBaseURL      string        `mapstructure:"api_base_url" validate:"required,url"`
	AuthToken       string        `mapstructure:"auth_token" validate:"required"`
	TimeoutDuration time.Duration `mapstructure:"timeout_duration" validate:"min=1s,max=5m"`
	Debug           bool          `mapstructure:"debug"`
	LogLevel        string        `mapstructure:"log_level" validate:"oneof=debug info warn error"`
	VolumeConfig    VolumeConfig  `mapstructure:"volume_config"`
}

// VolumeConfig defines different volume presets for data generation
type VolumeConfig struct {
	Small  VolumeMetrics `mapstructure:"small"`
	Medium VolumeMetrics `mapstructure:"medium"`
	Large  VolumeMetrics `mapstructure:"large"`
}

// VolumeMetrics defines the quantity of entities to generate
type VolumeMetrics struct {
	Organizations          int `mapstructure:"organizations" validate:"min=1"`
	LedgersPerOrg          int `mapstructure:"ledgers_per_org" validate:"min=1"`
	AssetsPerLedger        int `mapstructure:"assets_per_ledger" validate:"min=1"`
	PortfoliosPerLedger    int `mapstructure:"portfolios_per_ledger" validate:"min=1"`
	SegmentsPerLedger      int `mapstructure:"segments_per_ledger" validate:"min=1"`
	AccountsPerLedger      int `mapstructure:"accounts_per_ledger" validate:"min=1"`
	TransactionsPerAccount int `mapstructure:"transactions_per_account" validate:"min=1"`
}

// NewConfiguration creates a new configuration with default values
func NewConfiguration() *Configuration {
	return &Configuration{
		APIBaseURL:      "https://api.midaz.io",
		TimeoutDuration: 30 * time.Second,
		Debug:           false,
		LogLevel:        "info",
		VolumeConfig:    DefaultVolumeConfig(),
	}
}

// DefaultVolumeConfig returns default volume configuration
func DefaultVolumeConfig() VolumeConfig {
	return VolumeConfig{
		Small: VolumeMetrics{
			Organizations:          2,
			LedgersPerOrg:          2,
			AssetsPerLedger:        3,
			PortfoliosPerLedger:    2,
			SegmentsPerLedger:      2,
			AccountsPerLedger:      100,
			TransactionsPerAccount: 20,
		},
		Medium: VolumeMetrics{
			Organizations:          5,
			LedgersPerOrg:          3,
			AssetsPerLedger:        5,
			PortfoliosPerLedger:    3,
			SegmentsPerLedger:      4,
			AccountsPerLedger:      500,
			TransactionsPerAccount: 50,
		},
		Large: VolumeMetrics{
			Organizations:          10,
			LedgersPerOrg:          5,
			AssetsPerLedger:        8,
			PortfoliosPerLedger:    5,
			SegmentsPerLedger:      6,
			AccountsPerLedger:      1000,
			TransactionsPerAccount: 100,
		},
	}
}

// Validate performs custom validation logic
func (c *Configuration) Validate() error {
	if c.AuthToken == "" {
		return errors.New("auth token is required")
	}

	if c.TimeoutDuration <= 0 {
		return errors.New("timeout duration must be positive")
	}

	if c.TimeoutDuration > 5*time.Minute {
		return errors.New("timeout duration cannot exceed 5 minutes")
	}

	return nil
}
