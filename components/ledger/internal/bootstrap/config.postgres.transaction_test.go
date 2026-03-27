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
func testTransactionPostgresConnector(t *testing.T) func(*Config, libLog.Logger) (*libPostgres.Client, error) {
	t.Helper()

	return func(cfg *Config, logger libLog.Logger) (*libPostgres.Client, error) {
		return buildTransactionPostgresConnection(cfg, logger)
	}
}

// withTestConnector temporarily replaces the package-level transactionPostgresConnector
// with a test version that does not require a live database. It restores the
// original connector when the test finishes.
func withTransactionTestConnector(t *testing.T) {
	t.Helper()

	originalConnector := transactionPostgresConnector
	transactionPostgresConnector = testTransactionPostgresConnector(t)

	originalMigrator := transactionPostgresMigrator
	transactionPostgresMigrator = func(_ *Config, _ libLog.Logger) error { return nil }

	t.Cleanup(func() {
		transactionPostgresConnector = originalConnector
		transactionPostgresMigrator = originalMigrator
	})
}

// Note: t.Parallel() omitted because withTestConnector mutates package-level transactionPostgresConnector.
func TestInitTransactionPostgres(t *testing.T) {
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
				TenantClient:       mustTransactionTenantClient(t, logger),
				TenantServiceName:  "transaction",
			},
			wantMultiTenant: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sub-tests must NOT call t.Parallel() because withTestConnector mutates package-level state.
			withTransactionTestConnector(t)

			result, err := initTransactionPostgres(tt.opts, cfg, logger)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.NotNil(t, result.transactionRepo)
			assert.NotNil(t, result.operationRepo)
			assert.NotNil(t, result.assetRateRepo)
			assert.NotNil(t, result.balanceRepo)
			assert.NotNil(t, result.operationRouteRepo)
			assert.NotNil(t, result.transactionRouteRepo)

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

// Note: t.Parallel() omitted because withTestConnector mutates package-level transactionPostgresConnector.
func TestInitTransactionMultiTenantPostgres_Success(t *testing.T) {
	withTransactionTestConnector(t)

	logger := libLog.NewNop()
	client := mustTransactionTenantClient(t, logger)
	cfg := &Config{}

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       client,
		TenantServiceName:  "transaction",
	}

	result, err := initTransactionMultiTenantPostgres(opts, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotNil(t, result.pgManager, "pgManager must be set in multi-tenant mode")
	assert.NotNil(t, result.connection, "connection must be set in multi-tenant mode (placeholder)")
	assert.NotNil(t, result.transactionRepo, "transactionRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.operationRepo, "operationRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.assetRateRepo, "assetRateRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.balanceRepo, "balanceRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.operationRouteRepo, "operationRouteRepo must be set in multi-tenant mode")
	assert.NotNil(t, result.transactionRouteRepo, "transactionRouteRepo must be set in multi-tenant mode")
}

func TestInitTransactionMultiTenantPostgres_NilTenantClient_ReturnsError(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	cfg := &Config{}

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       nil,
		TenantServiceName:  "transaction",
	}

	result, err := initTransactionMultiTenantPostgres(opts, cfg, logger)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "TenantClient is required")
}

// Note: t.Parallel() omitted because withTestConnector mutates package-level transactionPostgresConnector.
func TestInitTransactionSingleTenantPostgres_CreatesComponents(t *testing.T) {
	withTransactionTestConnector(t)

	logger := libLog.NewNop()

	cfg := &Config{}

	result, err := initTransactionSingleTenantPostgres(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotNil(t, result.connection, "single-tenant mode must have a non-nil connection")
	assert.Nil(t, result.pgManager, "single-tenant mode must have a nil pgManager")
	assert.NotNil(t, result.transactionRepo, "single-tenant mode must have a non-nil transactionRepo")
	assert.NotNil(t, result.operationRepo, "single-tenant mode must have a non-nil operationRepo")
	assert.NotNil(t, result.assetRateRepo, "single-tenant mode must have a non-nil assetRateRepo")
	assert.NotNil(t, result.balanceRepo, "single-tenant mode must have a non-nil balanceRepo")
	assert.NotNil(t, result.operationRouteRepo, "single-tenant mode must have a non-nil operationRouteRepo")
	assert.NotNil(t, result.transactionRouteRepo, "single-tenant mode must have a non-nil transactionRouteRepo")
}

// withFailingConnector temporarily replaces transactionPostgresConnector with one that
// always returns the given error. This exercises the connector-error branches
// in initMultiTenantPostgres and initSingleTenantPostgres.
func withTransactionFailingConnector(t *testing.T, connErr error) {
	t.Helper()

	original := transactionPostgresConnector
	transactionPostgresConnector = func(_ *Config, _ libLog.Logger) (*libPostgres.Client, error) {
		return nil, connErr
	}

	t.Cleanup(func() {
		transactionPostgresConnector = original
	})
}

func TestInitTransactionMultiTenantPostgres_ConnectorError_ReturnsWrappedError(t *testing.T) {
	// Note: t.Parallel() removed because withFailingConnector mutates package-level
	// transactionPostgresConnector, which is incompatible with parallel test execution.

	connErr := fmt.Errorf("simulated connection failure")
	withTransactionFailingConnector(t, connErr)

	logger := libLog.NewNop()

	cfg := &Config{}
	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       mustTransactionTenantClient(t, logger),
		TenantServiceName:  "transaction",
	}

	result, err := initTransactionMultiTenantPostgres(opts, cfg, logger)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to connect to PostgreSQL (multi-tenant)")
	assert.ErrorIs(t, err, connErr, "original error must be wrapped, not replaced")
}

func TestInitTransactionSingleTenantPostgres_ConnectorError_ReturnsWrappedError(t *testing.T) {
	// Note: t.Parallel() removed because withFailingConnector mutates package-level
	// transactionPostgresConnector, which is incompatible with parallel test execution.

	connErr := fmt.Errorf("simulated connection failure")
	withTransactionFailingConnector(t, connErr)

	logger := libLog.NewNop()

	cfg := &Config{}

	result, err := initTransactionSingleTenantPostgres(cfg, logger)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to connect to PostgreSQL (single-tenant)")
	assert.ErrorIs(t, err, connErr, "original error must be wrapped, not replaced")
}

func TestBuildTransactionPostgresConnection_PrefixedValues(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "uses prefixed values",
			cfg: &Config{
				TxnPrefixedPrimaryDBHost:     "prefixed-host",
				TxnPrefixedPrimaryDBUser:     "prefixed-user",
				TxnPrefixedPrimaryDBPassword: "prefixed-pass",
				TxnPrefixedPrimaryDBName:     "prefixed-db",
				TxnPrefixedPrimaryDBPort:     "5433",
				TxnPrefixedPrimaryDBSSLMode:  "require",
				TxnPrefixedReplicaDBHost:     "prefixed-replica",
				TxnPrefixedReplicaDBUser:     "prefixed-ruser",
				TxnPrefixedReplicaDBPassword: "prefixed-rpass",
				TxnPrefixedReplicaDBName:     "prefixed-rdb",
				TxnPrefixedReplicaDBPort:     "5434",
				TxnPrefixedReplicaDBSSLMode:  "verify-full",
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

			conn, err := buildTransactionPostgresConnection(tt.cfg, logger)
			require.NoError(t, err)
			require.NotNil(t, conn)

			connected, connectedErr := conn.IsConnected()
			require.NoError(t, connectedErr)
			assert.False(t, connected)
		})
	}
}
