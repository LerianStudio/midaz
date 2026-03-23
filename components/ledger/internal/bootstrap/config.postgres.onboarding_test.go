// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPostgresConnector returns a Postgres client without network I/O.
func testOnboardingPostgresConnector(t *testing.T) func(*Config, libLog.Logger) (*libPostgres.Client, error) {
	t.Helper()

	return func(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
		return buildOnboardingPostgresConnection(cfg, logger)
	}
}

// withTestConnector temporarily replaces the package-level onboardingPostgresConnector
// with a test version that does not require a live database. It restores the
// original connector when the test finishes.
func withOnboardingTestConnector(t *testing.T) {
	t.Helper()

	originalConnector := onboardingPostgresConnector
	onboardingPostgresConnector = testOnboardingPostgresConnector(t)

	originalMigrator := onboardingPostgresMigrator
	onboardingPostgresMigrator = func(_ *Config, _ libLog.Logger) error { return nil }

	t.Cleanup(func() {
		onboardingPostgresConnector = originalConnector
		onboardingPostgresMigrator = originalMigrator
	})
}

// Note: t.Parallel() omitted because withTestConnector mutates package-level postgresConnector.
func TestInitOnboardingPostgres(t *testing.T) {
	logger := libLog.NewNop()

	cfg := &Config{}

	tests := []struct {
		name            string
		opts            *Options
		wantMultiTenant bool
	}{
		{
			name:            "nil opts calls single-tenant path",
			opts:            nil,
			wantMultiTenant: false,
		},
		{
			name: "multi-tenant disabled calls single-tenant path",
			opts: &Options{
				MultiTenantEnabled: false,
			},
			wantMultiTenant: false,
		},
		{
			name: "multi-tenant enabled calls multi-tenant path",
			opts: &Options{
				MultiTenantEnabled: true,
				TenantClient:       mustOnboardingTenantClient(t, logger),
				TenantServiceName:  "onboarding",
			},
			wantMultiTenant: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sub-tests must NOT call t.Parallel() because withTestConnector mutates package-level state.
			withOnboardingTestConnector(t)

			result, err := initOnboardingPostgres(tt.opts, cfg, logger)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.NotNil(t, result.organizationRepo)
			assert.NotNil(t, result.ledgerRepo)
			assert.NotNil(t, result.accountRepo)
			assert.NotNil(t, result.assetRepo)
			assert.NotNil(t, result.portfolioRepo)
			assert.NotNil(t, result.segmentRepo)
			assert.NotNil(t, result.accountTypeRepo)

			if tt.wantMultiTenant {
				assert.NotNil(t, result.pgManager, "multi-tenant mode should have a non-nil pgManager")
				assert.NotNil(t, result.connection, "multi-tenant mode should have a non-nil connection (placeholder)")
			} else {
				assert.Nil(t, result.pgManager, "single-tenant mode should have a nil pgManager")
				assert.NotNil(t, result.connection, "single-tenant mode should have a non-nil connection")
			}
		})
	}
}

// Note: t.Parallel() omitted because withTestConnector mutates package-level postgresConnector.
func TestInitOnboardingMultiTenantPostgres_Success(t *testing.T) {
	withOnboardingTestConnector(t)

	logger := libLog.NewNop()
	client := mustOnboardingTenantClient(t, logger)
	cfg := &Config{}

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       client,
		TenantServiceName:  "onboarding",
	}

	result, err := initOnboardingMultiTenantPostgres(opts, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotNil(t, result.pgManager, "pgManager must be set in multi-tenant mode")
	assert.NotNil(t, result.connection, "connection must be set in multi-tenant mode (placeholder)")
	assert.NotNil(t, result.organizationRepo, "organizationRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.ledgerRepo, "ledgerRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.accountRepo, "accountRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.assetRepo, "assetRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.portfolioRepo, "portfolioRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.segmentRepo, "segmentRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.accountTypeRepo, "accountTypeRepo must be set in multi-tenant mode")
}

func TestInitOnboardingMultiTenantPostgres_NilTenantClient_ReturnsError(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	cfg := &Config{}

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       nil,
		TenantServiceName:  "onboarding",
	}

	result, err := initOnboardingMultiTenantPostgres(opts, cfg, logger)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "TenantClient is required")
}

// Note: t.Parallel() omitted because withTestConnector mutates package-level postgresConnector.
func TestInitOnboardingSingleTenantPostgres_CreatesComponents(t *testing.T) {
	withOnboardingTestConnector(t)

	logger := libLog.NewNop()

	cfg := &Config{}

	result, err := initOnboardingSingleTenantPostgres(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotNil(t, result.connection, "single-tenant mode must have a non-nil connection")
	assert.Nil(t, result.pgManager, "single-tenant mode must have a nil pgManager")
	assert.NotNil(t, result.organizationRepo, "single-tenant mode must have a non-nil organizationRepo")
	assert.NotNil(t, result.ledgerRepo, "single-tenant mode must have a non-nil ledgerRepo")
	assert.NotNil(t, result.accountRepo, "single-tenant mode must have a non-nil accountRepo")
	assert.NotNil(t, result.assetRepo, "single-tenant mode must have a non-nil assetRepo")
	assert.NotNil(t, result.portfolioRepo, "single-tenant mode must have a non-nil portfolioRepo")
	assert.NotNil(t, result.segmentRepo, "single-tenant mode must have a non-nil segmentRepo")
	assert.NotNil(t, result.accountTypeRepo, "single-tenant mode must have a non-nil accountTypeRepo")
}

// withFailingConnector temporarily replaces onboardingPostgresConnector with one that
// always returns the given error. This exercises the connector-error branches
// in initMultiTenantPostgres and initSingleTenantPostgres.
func withOnboardingFailingConnector(t *testing.T, connErr error) {
	t.Helper()

	original := onboardingPostgresConnector
	onboardingPostgresConnector = func(_ *Config, _ libLog.Logger) (*libPostgres.Client, error) {
		return nil, connErr
	}

	t.Cleanup(func() {
		onboardingPostgresConnector = original
	})
}

func TestInitOnboardingMultiTenantPostgres_ConnectorError_ReturnsWrappedError(t *testing.T) {
	// Note: t.Parallel() removed because withFailingConnector mutates package-level
	// postgresConnector, which is incompatible with parallel test execution.

	connErr := fmt.Errorf("simulated connection failure")
	withOnboardingFailingConnector(t, connErr)

	logger := libLog.NewNop()

	cfg := &Config{}
	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       mustOnboardingTenantClient(t, logger),
		TenantServiceName:  "onboarding",
	}

	result, err := initOnboardingMultiTenantPostgres(opts, cfg, logger)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to connect to PostgreSQL (multi-tenant)")
	assert.ErrorIs(t, err, connErr, "original error must be wrapped, not replaced")
}

func TestInitOnboardingSingleTenantPostgres_ConnectorError_ReturnsWrappedError(t *testing.T) {
	// Note: t.Parallel() removed because withFailingConnector mutates package-level
	// postgresConnector, which is incompatible with parallel test execution.

	connErr := fmt.Errorf("simulated connection failure")
	withOnboardingFailingConnector(t, connErr)

	logger := libLog.NewNop()

	cfg := &Config{}

	result, err := initOnboardingSingleTenantPostgres(cfg, logger)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to connect to PostgreSQL (single-tenant)")
	assert.ErrorIs(t, err, connErr, "original error must be wrapped, not replaced")
}

func TestBuildOnboardingPostgresConnection_PrefixedFallback(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "uses prefixed values when available",
			cfg: &Config{
				OnbPrefixedPrimaryDBHost:     "prefixed-host",
				OnbPrefixedPrimaryDBUser:     "prefixed-user",
				OnbPrefixedPrimaryDBPassword: "prefixed-pass",
				OnbPrefixedPrimaryDBName:     "prefixed-db",
				OnbPrefixedPrimaryDBPort:     "5433",
				OnbPrefixedPrimaryDBSSLMode:  "require",
				OnbPrefixedReplicaDBHost:     "prefixed-replica",
				OnbPrefixedReplicaDBUser:     "prefixed-ruser",
				OnbPrefixedReplicaDBPassword: "prefixed-rpass",
				OnbPrefixedReplicaDBName:     "prefixed-rdb",
				OnbPrefixedReplicaDBPort:     "5434",
				OnbPrefixedReplicaDBSSLMode:  "verify-full",
				PrimaryDBHost:                "fallback-host",
				PrimaryDBUser:                "fallback-user",
			},
		},
		{
			name: "falls back to non-prefixed values",
			cfg: &Config{
				PrimaryDBHost:     "fallback-host",
				PrimaryDBUser:     "fallback-user",
				PrimaryDBPassword: "fallback-pass",
				PrimaryDBName:     "fallback-db",
				PrimaryDBPort:     "5432",
				PrimaryDBSSLMode:  "disable",
				ReplicaDBHost:     "replica-host",
				ReplicaDBUser:     "replica-user",
				ReplicaDBPassword: "replica-pass",
				ReplicaDBName:     "replica-db",
				ReplicaDBPort:     "5432",
				ReplicaDBSSLMode:  "disable",
			},
		},
		{
			name: "empty config produces empty-valued connection strings",
			cfg:  &Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn, err := buildOnboardingPostgresConnection(tt.cfg, logger)
			require.NoError(t, err)
			require.NotNil(t, conn)

			connected, connectedErr := conn.IsConnected()
			require.NoError(t, connectedErr)
			assert.False(t, connected)
		})
	}
}
