// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"database/sql"
	"fmt"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/bxcodec/dbresolver/v2"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPostgresConnector returns a PostgresConnection with a pre-set ConnectionDB
// so that repo constructors' GetDB() calls succeed without requiring a live database.
// sql.Open with the pgx driver creates a lazy handle -- no actual connection is made.
func testPostgresConnector(t *testing.T) func(*Config, libLog.Logger) (*libPostgres.PostgresConnection, error) {
	t.Helper()

	return func(cfg *Config, logger libLog.Logger) (*libPostgres.PostgresConnection, error) {
		conn := buildPostgresConnection(cfg, logger)

		primary, err := sql.Open("pgx", "host=localhost dbname=test_noop sslmode=disable")
		if err != nil {
			return nil, err
		}

		replica, err := sql.Open("pgx", "host=localhost dbname=test_noop sslmode=disable")
		if err != nil {
			primary.Close()
			return nil, err
		}

		db := dbresolver.New(
			dbresolver.WithPrimaryDBs(primary),
			dbresolver.WithReplicaDBs(replica),
		)

		conn.ConnectionDB = &db
		conn.Connected = true

		t.Cleanup(func() {
			primary.Close()
			replica.Close()
		})

		return conn, nil
	}
}

// withTestConnector temporarily replaces the package-level postgresConnector
// with a test version that does not require a live database. It restores the
// original connector when the test finishes.
func withTestConnector(t *testing.T) {
	t.Helper()

	original := postgresConnector
	postgresConnector = testPostgresConnector(t)

	t.Cleanup(func() {
		postgresConnector = original
	})
}

func TestInitPostgres(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

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
				TenantClient:       tmclient.NewClient("http://localhost:0", logger),
				TenantServiceName:  "onboarding",
			},
			wantMultiTenant: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sub-tests must NOT call t.Parallel() because withTestConnector mutates package-level state.
			withTestConnector(t)

			result, err := initPostgres(tt.opts, cfg, logger)
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
func TestInitMultiTenantPostgres_Success(t *testing.T) {
	withTestConnector(t)

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	client := tmclient.NewClient("http://localhost:0", logger)
	cfg := &Config{}

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       client,
		TenantServiceName:  "onboarding",
	}

	result, err := initMultiTenantPostgres(opts, cfg, logger)
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

func TestInitMultiTenantPostgres_NilTenantClient_ReturnsError(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	cfg := &Config{}

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       nil,
		TenantServiceName:  "onboarding",
	}

	result, err := initMultiTenantPostgres(opts, cfg, logger)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "TenantClient is required")
}

// Note: t.Parallel() omitted because withTestConnector mutates package-level postgresConnector.
func TestInitSingleTenantPostgres_CreatesComponents(t *testing.T) {
	withTestConnector(t)

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	cfg := &Config{}

	result, err := initSingleTenantPostgres(cfg, logger)
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

// withFailingConnector temporarily replaces postgresConnector with one that
// always returns the given error. This exercises the connector-error branches
// in initMultiTenantPostgres and initSingleTenantPostgres.
func withFailingConnector(t *testing.T, connErr error) {
	t.Helper()

	original := postgresConnector
	postgresConnector = func(_ *Config, _ libLog.Logger) (*libPostgres.PostgresConnection, error) {
		return nil, connErr
	}

	t.Cleanup(func() {
		postgresConnector = original
	})
}

func TestInitMultiTenantPostgres_ConnectorError_ReturnsWrappedError(t *testing.T) {
	// Note: t.Parallel() removed because withFailingConnector mutates package-level
	// postgresConnector, which is incompatible with parallel test execution.

	connErr := fmt.Errorf("simulated connection failure")
	withFailingConnector(t, connErr)

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	cfg := &Config{}
	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       tmclient.NewClient("http://localhost:0", logger),
		TenantServiceName:  "onboarding",
	}

	result, err := initMultiTenantPostgres(opts, cfg, logger)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to connect to PostgreSQL (multi-tenant)")
	assert.ErrorIs(t, err, connErr, "original error must be wrapped, not replaced")
}

func TestInitSingleTenantPostgres_ConnectorError_ReturnsWrappedError(t *testing.T) {
	// Note: t.Parallel() removed because withFailingConnector mutates package-level
	// postgresConnector, which is incompatible with parallel test execution.

	connErr := fmt.Errorf("simulated connection failure")
	withFailingConnector(t, connErr)

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	cfg := &Config{}

	result, err := initSingleTenantPostgres(cfg, logger)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to connect to PostgreSQL (single-tenant)")
	assert.ErrorIs(t, err, connErr, "original error must be wrapped, not replaced")
}

func TestBuildPostgresConnection_PrefixedFallback(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	tests := []struct {
		name          string
		cfg           *Config
		wantPrimary   string
		wantReplica   string
		wantDBName    string
		wantComponent string
	}{
		{
			name: "uses prefixed values when available",
			cfg: &Config{
				PrefixedPrimaryDBHost:     "prefixed-host",
				PrefixedPrimaryDBUser:     "prefixed-user",
				PrefixedPrimaryDBPassword: "prefixed-pass",
				PrefixedPrimaryDBName:     "prefixed-db",
				PrefixedPrimaryDBPort:     "5433",
				PrefixedPrimaryDBSSLMode:  "require",
				PrefixedReplicaDBHost:     "prefixed-replica",
				PrefixedReplicaDBUser:     "prefixed-ruser",
				PrefixedReplicaDBPassword: "prefixed-rpass",
				PrefixedReplicaDBName:     "prefixed-rdb",
				PrefixedReplicaDBPort:     "5434",
				PrefixedReplicaDBSSLMode:  "verify-full",
				PrimaryDBHost:             "fallback-host",
				PrimaryDBUser:             "fallback-user",
			},
			wantPrimary:   "host=prefixed-host user=prefixed-user password=prefixed-pass dbname=prefixed-db port=5433 sslmode=require",
			wantReplica:   "host=prefixed-replica user=prefixed-ruser password=prefixed-rpass dbname=prefixed-rdb port=5434 sslmode=verify-full",
			wantDBName:    "prefixed-db",
			wantComponent: ApplicationName,
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
			wantPrimary:   "host=fallback-host user=fallback-user password=fallback-pass dbname=fallback-db port=5432 sslmode=disable",
			wantReplica:   "host=replica-host user=replica-user password=replica-pass dbname=replica-db port=5432 sslmode=disable",
			wantDBName:    "fallback-db",
			wantComponent: ApplicationName,
		},
		{
			name:          "empty config produces empty-valued connection strings",
			cfg:           &Config{},
			wantPrimary:   "host= user= password= dbname= port= sslmode=",
			wantReplica:   "host= user= password= dbname= port= sslmode=",
			wantDBName:    "",
			wantComponent: ApplicationName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			conn := buildPostgresConnection(tt.cfg, logger)
			require.NotNil(t, conn)

			assert.Equal(t, tt.wantPrimary, conn.ConnectionStringPrimary)
			assert.Equal(t, tt.wantReplica, conn.ConnectionStringReplica)
			assert.Equal(t, tt.wantDBName, conn.PrimaryDBName)
			assert.Equal(t, tt.wantComponent, conn.Component)
			assert.Equal(t, logger, conn.Logger)
		})
	}
}
