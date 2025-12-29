package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfig_Validate_ValidConfig verifies Validate does not panic for valid config.
func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "localhost",
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "5432",
		PrimaryDBSSLMode:   "disable",
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 25,
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.NotPanics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidPort verifies Validate panics for invalid port.
func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "localhost",
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "99999", // Invalid port
		PrimaryDBSSLMode:   "disable",
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 25,
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_EmptyRequiredField verifies Validate panics for empty required fields.
func TestConfig_Validate_EmptyRequiredField(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "", // Empty required field
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "5432",
		PrimaryDBSSLMode:   "disable",
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 25,
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidSSLMode verifies Validate panics for invalid SSL mode.
func TestConfig_Validate_InvalidSSLMode(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "localhost",
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "5432",
		PrimaryDBSSLMode:   "invalid-mode", // Invalid SSL mode
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 25,
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidPoolSize verifies Validate panics for invalid pool sizes.
func TestConfig_Validate_InvalidPoolSize(t *testing.T) {
	cfg := &Config{
		ServerAddress:      ":8080",
		PrimaryDBHost:      "localhost",
		PrimaryDBUser:      "postgres",
		PrimaryDBName:      "midaz",
		PrimaryDBPort:      "5432",
		PrimaryDBSSLMode:   "disable",
		ReplicaDBHost:      "localhost",
		ReplicaDBUser:      "postgres",
		ReplicaDBName:      "midaz",
		ReplicaDBPort:      "5432",
		ReplicaDBSSLMode:   "disable",
		MaxOpenConnections: 0, // Invalid: must be positive
		MaxIdleConnections: 5,
		MongoDBHost:        "localhost",
		MongoDBName:        "midaz_meta",
		MongoDBPort:        "27017",
		MaxPoolSize:        100,
		RedisHost:          "localhost:6379",
		RedisPoolSize:      10,
		RabbitMQHost:       "localhost",
		RabbitMQPortHost:   "5672",
		RabbitMQPortAMQP:   "5672",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}
