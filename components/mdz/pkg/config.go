package pkg

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type persistedConfig struct {
	CurrentProfile string              `json:"currentProfile"`
	Profiles       map[string]*Profile `json:"profiles"`
	UniqueID       string              `json:"uniqueID"`
}

// Config represents the configuration for the application.
// It encapsulates details such as the current profile in use, a unique identifier for the configuration,
// a map of profile names to Profile instances, and a reference to the ConfigManager responsible for managing this configuration.
type Config struct {
	currentProfile string
	uniqueID       string
	profiles       map[string]*Profile
	manager        *ConfigManager
}

// MarshalJSON implements the json.Marshaler interface for Config.
// It converts the Config instance into a JSON-encoded byte slice.
func (c *Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(persistedConfig{
		CurrentProfile: c.currentProfile,
		Profiles:       c.profiles,
		UniqueID:       c.uniqueID,
	})
}

// UnmarshalJSON implements the json.Unmarshaler interface for Config.
// It populates the Config instance with data from a JSON-encoded byte slice.
func (c *Config) UnmarshalJSON(data []byte) error {
	cfg := &persistedConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return err
	}

	*c = Config{
		currentProfile: cfg.CurrentProfile,
		profiles:       cfg.Profiles,
		uniqueID:       cfg.UniqueID,
	}

	return nil
}

// GetProfile retrieves a profile by name from the configuration.
// If the profile does not exist, it returns nil.
func (c *Config) GetProfile(name string) *Profile {
	p := c.profiles[name]
	if p != nil {
		p.config = c
	}

	return p
}

// GetProfileOrDefault retrieves a profile by name from the configuration.
// If the profile does not exist, it creates a new profile with the given membership URI and returns it.
func (c *Config) GetProfileOrDefault(name, membershipURI string) *Profile {
	p := c.GetProfile(name)
	if p == nil {
		if c.profiles == nil {
			c.profiles = map[string]*Profile{}
		}

		f := &Profile{
			membershipURI: membershipURI,
			config:        c,
		}

		c.profiles[name] = f

		return f
	}

	return p
}

// DeleteProfile removes a profile from the configuration by name.
// If the profile does not exist, it returns an error.
func (c *Config) DeleteProfile(s string) error {
	_, ok := c.profiles[s]
	if !ok {
		return errors.New("not found")
	}

	delete(c.profiles, s)

	return nil
}

// Persist saves the configuration to the file system.
// It uses the ConfigManager to update the configuration file with the current configuration.
func (c *Config) Persist() error {
	return c.manager.UpdateConfig(c)
}

// SetCurrentProfile sets the current profile to the given name and profile.
// It also updates the current profile name to the given name.
func (c *Config) SetCurrentProfile(name string, profile *Profile) {
	c.profiles[name] = profile
	c.currentProfile = name
}

// SetUniqueID sets the unique identifier for the configuration.
func (c *Config) SetUniqueID(id string) {
	c.uniqueID = id
}

// SetProfile sets the profile with the given name to the given profile.
func (c *Config) SetProfile(name string, profile *Profile) {
	c.profiles[name] = profile
}

// GetUniqueID retrieves the unique identifier for the configuration.
func (c *Config) GetUniqueID() string {
	return c.uniqueID
}

// GetProfiles retrieves the map of profile names to Profile instances from the configuration.
func (c *Config) GetProfiles() map[string]*Profile {
	return c.profiles
}

// GetCurrentProfileName retrieves the name of the current profile from the configuration.
func (c *Config) GetCurrentProfileName() string {
	return c.currentProfile
}

// SetCurrentProfileName sets the name of the current profile to the given string.
func (c *Config) SetCurrentProfileName(s string) {
	c.currentProfile = s
}

// GetConfig retrieves the configuration from the file system.
// It uses the ConfigManager to load the configuration from the file system.
func GetConfig(cmd *cobra.Command) (*Config, error) {
	return GetConfigManager(cmd).Load()
}

// GetConfigManager retrieves the ConfigManager instance associated with the given cobra.Command.
// It uses the FileFlag to determine the configuration file path and returns a new ConfigManager instance.
func GetConfigManager(cmd *cobra.Command) *ConfigManager {
	return NewConfigManager(GetString(cmd, FileFlag))
}

// GetCurrentProfileName retrieves the name of the current profile from the given cobra.Command and configuration.
// If the ProfileFlag is set, it returns the value of the ProfileFlag.
// Otherwise, it returns the current profile name from the configuration.
func GetCurrentProfileName(cmd *cobra.Command, config *Config) string {
	if profile := GetString(cmd, ProfileFlag); profile != "" {
		return profile
	}

	currentProfileName := config.GetCurrentProfileName()

	if currentProfileName == "" {
		currentProfileName = "default"
	}

	return currentProfileName
}

// GetCurrentProfile retrieves the current profile from the given cobra.Command and configuration.
// It returns the current profile from the configuration.
// If the current profile does not exist, it creates a new profile with the default membership URI and returns it.
func GetCurrentProfile(cmd *cobra.Command, cfg *Config) *Profile {
	return cfg.GetProfileOrDefault(GetCurrentProfileName(cmd, cfg), defaultMembershipURI)
}
