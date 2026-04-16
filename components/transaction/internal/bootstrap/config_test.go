// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

func TestEnvFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefixed string
		fallback string
		want     string
	}{
		{name: "prefixed non-empty returns prefixed", prefixed: "prefixed-value", fallback: "fallback-value", want: "prefixed-value"},
		{name: "prefixed empty returns fallback", prefixed: "", fallback: "fallback-value", want: "fallback-value"},
		{name: "both empty returns empty", prefixed: "", fallback: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, utils.EnvFallback(tt.prefixed, tt.fallback))
		})
	}
}

func TestEnvFallbackInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefixed int
		fallback int
		want     int
	}{
		{name: "prefixed non-zero returns prefixed", prefixed: 10, fallback: 5, want: 10},
		{name: "prefixed zero returns fallback", prefixed: 0, fallback: 5, want: 5},
		{name: "both zero returns zero", prefixed: 0, fallback: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, utils.EnvFallbackInt(tt.prefixed, tt.fallback))
		})
	}
}

func TestParseSeedBrokers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "multiple brokers with spaces and empty entries",
			raw:  "broker-1:9092, broker-2:9092, ,broker-3:9092",
			want: []string{"broker-1:9092", "broker-2:9092", "broker-3:9092"},
		},
		{
			name: "single broker",
			raw:  "broker-1:9092",
			want: []string{"broker-1:9092"},
		},
		{
			name: "empty raw list",
			raw:  "",
			want: []string{},
		},
		{
			name: "only commas and spaces",
			raw:  " , , ",
			want: []string{},
		},
		{
			name: "trailing commas",
			raw:  "broker-1:9092,broker-2:9092,,",
			want: []string{"broker-1:9092", "broker-2:9092"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, redpanda.ParseSeedBrokers(tt.raw))
		})
	}
}

func TestResolveBrokerOperationTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "valid duration", raw: "7s", want: 7 * time.Second},
		{name: "invalid duration falls back to default", raw: "invalid", want: redpanda.DefaultOperationTimeout},
		{name: "negative duration falls back to default", raw: "-5s", want: redpanda.DefaultOperationTimeout},
		{name: "zero duration falls back to default", raw: "0s", want: redpanda.DefaultOperationTimeout},
		{name: "empty duration falls back to default", raw: "", want: redpanda.DefaultOperationTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, resolveBrokerOperationTimeout(tt.raw))
		})
	}
}

func TestEnforcePostgresSSLMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		envName    string
		sslMode    string
		expectErr  bool
		errorMatch string
	}{
		{name: "production-like blocks disable", envName: "production", sslMode: "disable", expectErr: true, errorMatch: "DB_TRANSACTION_SSLMODE=disable"},
		{name: "production-like allows require", envName: "production", sslMode: "require", expectErr: false},
		{name: "staging allows disable", envName: "staging", sslMode: "disable", expectErr: false},
		{name: "development allows disable", envName: "development", sslMode: "disable", expectErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := enforcePostgresSSLMode(tt.envName, tt.sslMode, "DB_TRANSACTION_SSLMODE")
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMatch)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestShouldAutoRecoverDirtyMigration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		envName string
		want    bool
	}{
		{name: "local environment", envName: "local", want: true},
		{name: "development environment", envName: "development", want: true},
		{name: "production environment", envName: "production", want: false},
		{name: "empty environment", envName: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, shouldAutoRecoverDirtyMigration(tt.envName))
		})
	}
}

func TestParseDirtyMigrationVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		err   error
		want  int64
		found bool
	}{
		{name: "dirty version parsed", err: errors.New("migration failed: dirty database version 13. fix and force version"), want: 13, found: true},                                             //nolint:err113
		{name: "wrapped dirty version parsed", err: fmt.Errorf("bootstrap failed: %w", errors.New("migration failed: dirty database version 22. fix and force version")), want: 22, found: true}, //nolint:err113
		{name: "non dirty error", err: errors.New("connection refused"), want: 0, found: false},                                                                                                  //nolint:err113
		{name: "nil error", err: nil, want: 0, found: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			version, found := parseDirtyMigrationVersion(tt.err)
			assert.Equal(t, tt.want, version)
			assert.Equal(t, tt.found, found)
		})
	}
}

func TestMigrationRepairStatements(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		[]string{"DROP INDEX CONCURRENTLY IF EXISTS idx_operation_account"},
		migrationRepairStatements(13),
	)

	assert.Nil(t, migrationRepairStatements(99))
}

// TestDefaultPostgresSSLMode pins the env-aware default contract: operators
// who drop DB_TRANSACTION_SSLMODE in production-like environments get
// "require" (which enforcePostgresSSLMode accepts), while dev/local/test/
// staging fall back to "disable" (which enforcePostgresSSLMode also accepts
// because those envs are non-production). An empty ENV_NAME — which can
// happen on fresh deploys before the env is propagated — is treated as
// production to fail closed against accidental plaintext connections.
func TestDefaultPostgresSSLMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		envName string
		want    string
	}{
		{name: "empty env defaults to require", envName: "", want: "require"},
		{name: "production defaults to require", envName: "production", want: "require"},
		{name: "unknown env defaults to require", envName: "mars", want: "require"},
		{name: "development allows disable", envName: "development", want: "disable"},
		{name: "local allows disable", envName: "local", want: "disable"},
		{name: "staging allows disable", envName: "staging", want: "disable"},
		{name: "test allows disable", envName: "test", want: "disable"},
		{name: "whitespace trimmed around production", envName: "  production  ", want: "require"},
		{name: "whitespace trimmed around development", envName: "  development  ", want: "disable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, defaultPostgresSSLMode(tt.envName))
		})
	}
}

// TestConfigDefault_AuthorizerTimeoutMS guards the 250ms authorizer timeout
// baseline declared via the `default` struct tag. lib-commons/v2 does not
// honour `default` at runtime (that behaviour moved to v4), so the tag is
// effectively documentation until we migrate. The test pins the declared
// value so operators reading the source see the correct target and so the
// struct tag does not silently regress during refactors. The .env.example
// files (which ARE read by docker-compose and k8s) also carry 250.
func TestConfigDefault_AuthorizerTimeoutMS(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf(Config{})
	field, ok := typ.FieldByName("AuthorizerTimeoutMS")
	require.True(t, ok, "AuthorizerTimeoutMS field must exist")
	assert.Equal(t, "AUTHORIZER_TIMEOUT_MS", field.Tag.Get("env"))
	assert.Equal(t, "250", field.Tag.Get("default"),
		"AUTHORIZER_TIMEOUT_MS declared default must cover cross-shard 2PC RTTs + WAL fsync + GC pauses")
}

// TestConfig_NoCasdoorFields proves the Casdoor dead-code fields have been
// removed. We iterate the Config struct's field set via reflection to avoid
// a compile-time-only check: if someone re-adds one of these fields, this
// test will fail loudly rather than the typo being caught only by a future
// reviewer. This closes the D5 Casdoor dead-code finding.
func TestConfig_NoCasdoorFields(t *testing.T) {
	// Not t.Parallel: t.Setenv cannot be used with parallel tests.

	// reflect-based audit: walk the Config struct and ensure no exported field
	// starts with "Casdoor" or equals "JWKAddress".
	cfg := Config{}
	typ := reflect.TypeOf(cfg)

	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		assert.False(t, strings.HasPrefix(name, "Casdoor"),
			"Config must not contain Casdoor-prefixed field %q (dead code)", name)
		assert.NotEqual(t, "JWKAddress", name,
			"Config must not contain JWKAddress field (dead code)")
	}

	// Belt-and-braces: assert the legacy env tags are absent. Use
	// SetConfigFromEnvVars to confirm the zero-value struct does not carry
	// residue from removed env bindings.
	t.Setenv("CASDOOR_ADDRESS", "should-not-be-read")
	t.Setenv("CASDOOR_CLIENT_ID", "should-not-be-read")
	t.Setenv("CASDOOR_JWK_ADDRESS", "should-not-be-read")

	loadedCfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(loadedCfg))
	// No field should reflect any of the CASDOOR_* env vars. If one does,
	// SetConfigFromEnvVars would populate it from the env.
	// (We can't name the field here because it's been removed — that's the
	// point — so we rely on the struct walk above.)
	_ = loadedCfg
}
