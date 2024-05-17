package pkg

import (
	"encoding/json"
	"os"
	"path"
)

const (
	defaultMembershipURI = "https://app.midaz.cloud/api"
)

// ConfigManager is a struct that use a string configFilePath
type ConfigManager struct {
	configFilePath string
}

// Load is a func that load a filepath and return a *Config of application
func (m *ConfigManager) Load() (*Config, error) {
	f, err := os.Open(m.configFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				profiles: map[string]*Profile{},
				manager:  m,
			}, nil
		}

		return nil, err
	}
	defer f.Close()

	cfg := &Config{}
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, err
	}

	cfg.manager = m
	if cfg.profiles == nil {
		cfg.profiles = map[string]*Profile{}
	}

	return cfg, nil
}

// UpdateConfig is a func that update a *Config of application
func (m *ConfigManager) UpdateConfig(config *Config) error {
	if err := os.MkdirAll(path.Dir(m.configFilePath), 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(m.configFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	if err := enc.Encode(config); err != nil {
		return err
	}

	return nil
}

// NewConfigManager is a func that return a struc of *ConfigManager
func NewConfigManager(configFilePath string) *ConfigManager {
	return &ConfigManager{
		configFilePath: configFilePath,
	}
}
