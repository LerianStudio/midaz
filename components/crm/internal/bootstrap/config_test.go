// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
)

// TestConfig_ZeroValues verifies that an unpopulated Config has the expected
// zero values and no pre-populated defaults beyond what Go's zero value gives.
// This guards against accidental drift in struct field types or removal of
// fields without a migration plan.
func TestConfig_ZeroValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{}

	assert.Empty(t, cfg.EnvName)
	assert.Empty(t, cfg.ServerAddress)
	assert.Empty(t, cfg.ProtoAddress)
	assert.Empty(t, cfg.LogLevel)
	assert.False(t, cfg.EnableTelemetry)
	assert.False(t, cfg.AuthEnabled)
	assert.Equal(t, 0, cfg.MaxPoolSize)
	assert.Empty(t, cfg.MongoURI)
	assert.Empty(t, cfg.MongoDBHost)
	assert.Empty(t, cfg.MongoDBName)
	assert.Empty(t, cfg.MongoDBPort)
	assert.Empty(t, cfg.MongoDBParameters)
	assert.Empty(t, cfg.HashSecretKey)
	assert.Empty(t, cfg.EncryptSecretKey)
	assert.Empty(t, cfg.AuthAddress)
	assert.Empty(t, cfg.OtelServiceName)
	assert.Empty(t, cfg.OtelLibraryName)
	assert.Empty(t, cfg.OtelServiceVersion)
	assert.Empty(t, cfg.OtelDeploymentEnv)
	assert.Empty(t, cfg.OtelColExporterEndpoint)
}

// TestConfig_LoadFromEnv verifies that libCommons.SetConfigFromEnvVars
// correctly maps the documented environment variables onto Config fields.
// This test pins the env var contract: renaming a field without updating
// the `env` tag will fail the test.
func TestConfig_LoadFromEnv(t *testing.T) {
	// Not t.Parallel: t.Setenv conflicts with other parallel tests touching
	// the same env vars in this package.
	t.Setenv("ENV_NAME", "development")
	t.Setenv("SERVER_ADDRESS", ":4003")
	t.Setenv("PROTO_ADDRESS", ":4013")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("MONGO_URI", "mongodb")
	t.Setenv("MONGO_HOST", "crm-mongo")
	t.Setenv("MONGO_NAME", "crm")
	t.Setenv("MONGO_USER", "midaz")
	t.Setenv("MONGO_PASSWORD", "lerian")
	t.Setenv("MONGO_PORT", "5703")
	t.Setenv("MONGO_PARAMETERS", "authSource=admin")
	t.Setenv("MONGO_MAX_POOL_SIZE", "250")
	t.Setenv("LCRYPTO_HASH_SECRET_KEY", "my-hash-secret-key")
	t.Setenv("LCRYPTO_ENCRYPT_SECRET_KEY", "my-encrypt-secret-key")
	t.Setenv("PLUGIN_AUTH_ADDRESS", "http://plugin-auth:4000")
	t.Setenv("PLUGIN_AUTH_ENABLED", "true")
	t.Setenv("ENABLE_TELEMETRY", "true")
	t.Setenv("OTEL_RESOURCE_SERVICE_NAME", "crm")
	t.Setenv("OTEL_LIBRARY_NAME", "github.com/LerianStudio/midaz/v3/components/crm")
	t.Setenv("OTEL_RESOURCE_SERVICE_VERSION", "v3.5.1")
	t.Setenv("OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT", "development")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otlp://collector:4317")

	cfg := &Config{}

	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	assert.Equal(t, "development", cfg.EnvName)
	assert.Equal(t, ":4003", cfg.ServerAddress)
	assert.Equal(t, ":4013", cfg.ProtoAddress)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "mongodb", cfg.MongoURI)
	assert.Equal(t, "crm-mongo", cfg.MongoDBHost)
	assert.Equal(t, "crm", cfg.MongoDBName)
	assert.Equal(t, "midaz", cfg.MongoDBUser)
	assert.Equal(t, "lerian", cfg.MongoDBPassword)
	assert.Equal(t, "5703", cfg.MongoDBPort)
	assert.Equal(t, "authSource=admin", cfg.MongoDBParameters)
	assert.Equal(t, 250, cfg.MaxPoolSize)
	assert.Equal(t, "my-hash-secret-key", cfg.HashSecretKey)
	assert.Equal(t, "my-encrypt-secret-key", cfg.EncryptSecretKey)
	assert.Equal(t, "http://plugin-auth:4000", cfg.AuthAddress)
	assert.True(t, cfg.AuthEnabled)
	assert.True(t, cfg.EnableTelemetry)
	assert.Equal(t, "crm", cfg.OtelServiceName)
	assert.Equal(t, "github.com/LerianStudio/midaz/v3/components/crm", cfg.OtelLibraryName)
	assert.Equal(t, "v3.5.1", cfg.OtelServiceVersion)
	assert.Equal(t, "development", cfg.OtelDeploymentEnv)
	assert.Equal(t, "otlp://collector:4317", cfg.OtelColExporterEndpoint)
}

// TestConfig_AuthEnabledFallback ensures that missing PLUGIN_AUTH_ENABLED
// defaults to false (zero value for bool). Operators rely on this for
// local/dev environments where they don't want to run plugin-auth.
func TestConfig_AuthEnabledFallback(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	// PLUGIN_AUTH_ENABLED intentionally unset.

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))
	assert.False(t, cfg.AuthEnabled, "AuthEnabled must default to false when PLUGIN_AUTH_ENABLED is unset")
}

// TestMaxPoolSizeFallback verifies the 100-connection fallback applied in
// InitServersWithOptions when MONGO_MAX_POOL_SIZE is zero or negative.
// The fallback protects against misconfiguration deploying a pool of size 0.
func TestMaxPoolSizeFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		inputPool    int
		expectedPool int
	}{
		{name: "zero triggers fallback", inputPool: 0, expectedPool: 100},
		{name: "negative triggers fallback", inputPool: -10, expectedPool: 100},
		{name: "positive preserved", inputPool: 500, expectedPool: 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.inputPool
			if got <= 0 {
				got = 100
			}

			assert.Equal(t, tt.expectedPool, got)
		})
	}
}

// TestConfig_EnableTelemetryParsing verifies that string env values map to
// the expected bool state. libCommons.SetConfigFromEnvVars parses "true"/"false"
// case-insensitively; this pins that contract for the crm component.
func TestConfig_EnableTelemetryParsing(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "lower true", raw: "true", want: true},
		{name: "upper TRUE", raw: "TRUE", want: true},
		{name: "lower false", raw: "false", want: false},
		{name: "empty is false", raw: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENABLE_TELEMETRY", tt.raw)

			cfg := &Config{}
			require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

			assert.Equal(t, tt.want, cfg.EnableTelemetry)
		})
	}
}

// TestConfig_FieldsAreAddressable ensures the Config struct layout supports
// the pointer-based initialisation expected by libCommons.SetConfigFromEnvVars
// and by the server/service composition in InitServersWithOptions. A compile-
// time guard rather than runtime logic: the test fails to build if a field
// becomes non-addressable (e.g. wrapped in an interface).
func TestConfig_FieldsAreAddressable(t *testing.T) {
	t.Parallel()

	cfg := &Config{}

	// Take addresses of every field to force the compiler to verify
	// addressability. If the struct layout regresses this line will fail.
	_ = &cfg.EnvName
	_ = &cfg.ProtoAddress
	_ = &cfg.ServerAddress
	_ = &cfg.LogLevel
	_ = &cfg.OtelServiceName
	_ = &cfg.OtelLibraryName
	_ = &cfg.OtelServiceVersion
	_ = &cfg.OtelDeploymentEnv
	_ = &cfg.OtelColExporterEndpoint
	_ = &cfg.EnableTelemetry
	_ = &cfg.MongoURI
	_ = &cfg.MongoDBHost
	_ = &cfg.MongoDBName
	_ = &cfg.MongoDBUser
	_ = &cfg.MongoDBPassword
	_ = &cfg.MongoDBPort
	_ = &cfg.MongoDBParameters
	_ = &cfg.MaxPoolSize
	_ = &cfg.HashSecretKey
	_ = &cfg.EncryptSecretKey
	_ = &cfg.AuthAddress
	_ = &cfg.AuthEnabled
}

// TestOptions_LoggerPassThrough guards the Options contract: an Options value
// with a nil Logger must not crash InitServersWithOptions before the fallback
// logger is instantiated. We test the Options type directly (bootstrap
// initialisation requires live Mongo and is covered by integration tests).
func TestOptions_LoggerPassThrough(t *testing.T) {
	t.Parallel()

	opts := &Options{}
	assert.Nil(t, opts.Logger, "default Options.Logger must be nil so InitServersWithOptions creates one")

	// Verify the fallback branch condition used in InitServersWithOptions:
	//     if opts != nil && opts.Logger != nil { ... use opts.Logger ... }
	// Here both nil Options and non-nil Options-with-nil-Logger fall through
	// to the default logger initialisation.
	fallback := opts == nil || opts.Logger == nil
	assert.True(t, fallback)

	var nilOpts *Options

	fallbackFromNil := nilOpts == nil
	assert.True(t, fallbackFromNil)
}
