// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_CORSFields_Exist verifies that the Config struct has CORS
// configuration fields loaded from environment variables.
func TestConfig_CORSFields_Exist(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		CORSAllowedOrigins: "https://app.example.com,https://admin.example.com",
		CORSAllowedMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		CORSAllowedHeaders: "Origin,Content-Type,Accept,Authorization,X-Request-ID",
	}

	assert.Equal(t, "https://app.example.com,https://admin.example.com", cfg.CORSAllowedOrigins)
	assert.Equal(t, "GET,POST,PUT,PATCH,DELETE,OPTIONS", cfg.CORSAllowedMethods)
	assert.Equal(t, "Origin,Content-Type,Accept,Authorization,X-Request-ID", cfg.CORSAllowedHeaders)
}

// TestConfig_CORSFields_Defaults verifies that CORS fields have sensible
// zero values when not explicitly set.
func TestConfig_CORSFields_Defaults(t *testing.T) {
	t.Parallel()

	cfg := &Config{}

	assert.Equal(t, "", cfg.CORSAllowedOrigins)
	assert.Equal(t, "", cfg.CORSAllowedMethods)
	assert.Equal(t, "", cfg.CORSAllowedHeaders)
}

// TestConfig_Validate_ProductionBlocksWildcardCORSOrigins verifies that production
// config validation rejects wildcard "*" in CORS_ALLOWED_ORIGINS.
func TestConfig_Validate_ProductionBlocksWildcardCORSOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		origins     string
		expectErr   bool
		errContains string
	}{
		{
			name:        "Error - wildcard star origin in production",
			origins:     "*",
			expectErr:   true,
			errContains: "CORS_ALLOWED_ORIGINS must not contain wildcard (*) in production",
		},
		{
			name:        "Error - wildcard among multiple origins in production",
			origins:     "https://app.example.com,*",
			expectErr:   true,
			errContains: "CORS_ALLOWED_ORIGINS must not contain wildcard (*) in production",
		},
		{
			name:        "Error - empty CORS origins in production",
			origins:     "",
			expectErr:   true,
			errContains: "CORS_ALLOWED_ORIGINS must not be empty in production",
		},
		{
			name:        "Error - HTTP origin in production",
			origins:     "http://app.example.com",
			expectErr:   true,
			errContains: "CORS_ALLOWED_ORIGINS must use HTTPS in production",
		},
		{
			name:        "Error - mixed HTTP and HTTPS origins in production",
			origins:     "https://admin.example.com,http://app.example.com",
			expectErr:   true,
			errContains: "CORS_ALLOWED_ORIGINS must use HTTPS in production",
		},
		{
			name:      "Success - explicit HTTPS origins in production",
			origins:   "https://app.example.com,https://admin.example.com",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validManagerConfig()
			cfg.EnvName = "production"
			cfg.EnableTelemetry = true
			cfg.AuthEnabled = true
			cfg.MongoDBPassword = "real-password"
			cfg.RabbitMQPass = "real-password"
			cfg.RedisPassword = "real-password"
			cfg.RedisTLS = true
			cfg.ObjectStorageSecretKey = "real-secret"
			cfg.ObjectStorageEndpoint = "https://storage.example.com"
			cfg.CORSAllowedOrigins = tt.origins

			err := cfg.Validate()

			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestConfig_Validate_NonProductionAllowsAnyCORSOrigins verifies that non-production
// environments do not enforce CORS origin restrictions.
func TestConfig_Validate_NonProductionAllowsAnyCORSOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		envName string
		origins string
	}{
		{
			name:    "development with wildcard",
			envName: "development",
			origins: "*",
		},
		{
			name:    "staging with wildcard",
			envName: "staging",
			origins: "*",
		},
		{
			name:    "empty env with wildcard",
			envName: "",
			origins: "*",
		},
		{
			name:    "development with empty origins",
			envName: "development",
			origins: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validManagerConfig()
			cfg.EnvName = tt.envName
			cfg.CORSAllowedOrigins = tt.origins

			err := cfg.Validate()
			require.NoError(t, err)
		})
	}
}
