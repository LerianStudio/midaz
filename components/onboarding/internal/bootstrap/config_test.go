package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfig_Validate_ValidConfig verifies Validate does not panic for valid config.
func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := &Config{
		ServerAddress:          ":8080",
		PrimaryDBHost:          "localhost",
		PrimaryDBUser:          "postgres",
		PrimaryDBName:          "midaz",
		PrimaryDBPort:          "5432",
		PrimaryDBSSLMode:       "disable",
		ReplicaDBHost:          "localhost",
		ReplicaDBUser:          "postgres",
		ReplicaDBName:          "midaz",
		ReplicaDBPort:          "5432",
		ReplicaDBSSLMode:       "disable",
		MaxOpenConnections:     25,
		MaxIdleConnections:     5,
		MongoDBHost:            "localhost",
		MongoDBName:            "midaz_meta",
		MongoDBPort:            "27017",
		MaxPoolSize:            100,
		RedisHost:              "localhost:6379",
		RedisPoolSize:          10,
		TransactionGRPCAddress: "localhost",
		TransactionGRPCPort:    "50051",
	}

	require.NotPanics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_InvalidPort verifies Validate panics for invalid port.
func TestConfig_Validate_InvalidPort(t *testing.T) {
	cfg := &Config{
		ServerAddress:          ":8080",
		PrimaryDBHost:          "localhost",
		PrimaryDBUser:          "postgres",
		PrimaryDBName:          "midaz",
		PrimaryDBPort:          "invalid", // Invalid port
		PrimaryDBSSLMode:       "disable",
		ReplicaDBHost:          "localhost",
		ReplicaDBUser:          "postgres",
		ReplicaDBName:          "midaz",
		ReplicaDBPort:          "5432",
		ReplicaDBSSLMode:       "disable",
		MaxOpenConnections:     25,
		MaxIdleConnections:     5,
		MongoDBHost:            "localhost",
		MongoDBName:            "midaz_meta",
		MongoDBPort:            "27017",
		MaxPoolSize:            100,
		RedisHost:              "localhost:6379",
		RedisPoolSize:          10,
		TransactionGRPCAddress: "localhost",
		TransactionGRPCPort:    "50051",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}

// TestConfig_Validate_MissingGRPCConfig verifies Validate panics for missing gRPC config.
func TestConfig_Validate_MissingGRPCConfig(t *testing.T) {
	cfg := &Config{
		ServerAddress:          ":8080",
		PrimaryDBHost:          "localhost",
		PrimaryDBUser:          "postgres",
		PrimaryDBName:          "midaz",
		PrimaryDBPort:          "5432",
		PrimaryDBSSLMode:       "disable",
		ReplicaDBHost:          "localhost",
		ReplicaDBUser:          "postgres",
		ReplicaDBName:          "midaz",
		ReplicaDBPort:          "5432",
		ReplicaDBSSLMode:       "disable",
		MaxOpenConnections:     25,
		MaxIdleConnections:     5,
		MongoDBHost:            "localhost",
		MongoDBName:            "midaz_meta",
		MongoDBPort:            "27017",
		MaxPoolSize:            100,
		RedisHost:              "localhost:6379",
		RedisPoolSize:          10,
		TransactionGRPCAddress: "", // Empty required field
		TransactionGRPCPort:    "50051",
	}

	require.Panics(t, func() {
		cfg.Validate()
	})
}
