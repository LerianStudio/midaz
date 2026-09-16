// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
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

	t.Cleanup(func() {
		transactionPostgresConnector = originalConnector
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
			name: "primary only: absent replica configuration is accepted",
			cfg: &Config{
				TxnPrefixedPrimaryDBHost:     "prefixed-host",
				TxnPrefixedPrimaryDBUser:     "prefixed-user",
				TxnPrefixedPrimaryDBPassword: "prefixed-pass",
				TxnPrefixedPrimaryDBName:     "prefixed-db",
				TxnPrefixedPrimaryDBPort:     "5433",
				TxnPrefixedPrimaryDBSSLMode:  "require",
			},
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

func TestNewTransactionPostgresConfig_ReplicaHandling(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	primary := &Config{
		TxnPrefixedPrimaryDBHost:      "primary-host",
		TxnPrefixedPrimaryDBUser:      "primary-user",
		TxnPrefixedPrimaryDBPassword:  "primary-pass",
		TxnPrefixedPrimaryDBName:      "primary-db",
		TxnPrefixedPrimaryDBPort:      "5433",
		TxnPrefixedPrimaryDBSSLMode:   "require",
		TxnPrefixedMaxOpenConnections: 7,
		TxnPrefixedMaxIdleConnections: 3,
	}

	const wantPrimaryDSN = "host=primary-host user=primary-user password=primary-pass dbname=primary-db port=5433 sslmode=require"

	t.Run("absent replica passes an empty ReplicaDSN", func(t *testing.T) {
		t.Parallel()

		cfg := *primary

		pgCfg, err := newTransactionPostgresConfig(&cfg, logger)
		require.NoError(t, err)

		assert.Equal(t, wantPrimaryDSN, pgCfg.PrimaryDSN)
		assert.Equal(t, "", pgCfg.ReplicaDSN, "absent replica must reach lib-commons as the empty sentinel, not a host= user= ... shell")
		assert.Equal(t, 7, pgCfg.MaxOpenConnections)
		assert.Equal(t, 3, pgCfg.MaxIdleConnections)
	})

	t.Run("full replica keeps the legacy DSN format", func(t *testing.T) {
		t.Parallel()

		cfg := *primary
		cfg.TxnPrefixedReplicaDBHost = "replica-host"
		cfg.TxnPrefixedReplicaDBUser = "replica-user"
		cfg.TxnPrefixedReplicaDBPassword = "replica-pass"
		cfg.TxnPrefixedReplicaDBName = "replica-db"
		cfg.TxnPrefixedReplicaDBPort = "5434"
		cfg.TxnPrefixedReplicaDBSSLMode = "verify-full"

		pgCfg, err := newTransactionPostgresConfig(&cfg, logger)
		require.NoError(t, err)

		assert.Equal(t, wantPrimaryDSN, pgCfg.PrimaryDSN)
		assert.Equal(t, "host=replica-host user=replica-user password=replica-pass dbname=replica-db port=5434 sslmode=verify-full", pgCfg.ReplicaDSN)
	})

	t.Run("partial replica is a configuration error naming module and fields", func(t *testing.T) {
		t.Parallel()

		cfg := *primary
		cfg.TxnPrefixedReplicaDBHost = "replica-host"
		cfg.TxnPrefixedReplicaDBUser = "replica-user"

		_, err := newTransactionPostgresConfig(&cfg, logger)
		require.Error(t, err)
		assert.ErrorIs(t, err, errIncompleteReplicaConfig)
		assert.Contains(t, err.Error(), "transaction")
		assert.Contains(t, err.Error(), "missing: password, dbname, port, sslmode")

		conn, err := buildTransactionPostgresConnection(&cfg, logger)
		require.Error(t, err, "the builder must refuse to start rather than silently downgrade to primary-only")
		assert.ErrorIs(t, err, errIncompleteReplicaConfig)
		assert.Nil(t, conn)
	})
}
