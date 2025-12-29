package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfig_Validate_ValidConfig verifies Validate does not panic for valid config.
func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := &Config{
		ServerAddress:    ":8080",
		MongoDBHost:      "localhost",
		MongoDBName:      "midaz_crm",
		MongoDBPort:      "27017",
		MaxPoolSize:      100,
		HashSecretKey:    "test-hash-key-32-chars-long!!!!",
		EncryptSecretKey: "test-encrypt-key-32-chars-long!",
	}

	require.NotPanics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidPort verifies Validate panics for invalid port.
func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := &Config{
		ServerAddress:    ":8080",
		MongoDBHost:      "localhost",
		MongoDBName:      "midaz_crm",
		MongoDBPort:      "0", // Invalid port
		MaxPoolSize:      100,
		HashSecretKey:    "test-hash-key-32-chars-long!!!!",
		EncryptSecretKey: "test-encrypt-key-32-chars-long!",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_MissingCryptoKeys verifies Validate panics for missing crypto keys.
func TestConfig_Validate_MissingCryptoKeys(t *testing.T) {
	cfg := &Config{
		ServerAddress:    ":8080",
		MongoDBHost:      "localhost",
		MongoDBName:      "midaz_crm",
		MongoDBPort:      "27017",
		MaxPoolSize:      100,
		HashSecretKey:    "", // Missing required key
		EncryptSecretKey: "test-encrypt-key-32-chars-long!",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}
