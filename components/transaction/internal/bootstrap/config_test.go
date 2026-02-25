// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
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
		tt := tt
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
		tt := tt
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := enforcePostgresSSLMode(tt.envName, tt.sslMode, "DB_TRANSACTION_SSLMODE")
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMatch)
				return
			}

			assert.NoError(t, err)
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
		tt := tt
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
		{name: "dirty version parsed", err: fmt.Errorf("migration failed: Dirty database version 13. Fix and force version."), want: 13, found: true},
		{name: "wrapped dirty version parsed", err: fmt.Errorf("bootstrap failed: %w", fmt.Errorf("migration failed: Dirty database version 22. Fix and force version.")), want: 22, found: true},
		{name: "non dirty error", err: fmt.Errorf("connection refused"), want: 0, found: false},
		{name: "nil error", err: nil, want: 0, found: false},
	}

	for _, tt := range tests {
		tt := tt
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
