// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitMongo(t *testing.T) {
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
				TenantServiceName:  "transaction",
			},
			wantMultiTenant: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := initMongo(tt.opts, cfg, logger)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.NotNil(t, result.metadataRepo)

			if tt.wantMultiTenant {
				assert.NotNil(t, result.mongoManager, "multi-tenant mode should have a non-nil mongoManager")
				assert.Nil(t, result.connection, "multi-tenant mode should have a nil connection")
			} else {
				assert.Nil(t, result.mongoManager, "single-tenant mode should have a nil mongoManager")
				assert.NotNil(t, result.connection, "single-tenant mode should have a non-nil connection")
			}
		})
	}
}

func TestInitMultiTenantMongo_Success(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	client := tmclient.NewClient("http://localhost:0", logger)

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       client,
		TenantServiceName:  "transaction",
	}

	result, err := initMultiTenantMongo(opts, logger)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotNil(t, result.mongoManager, "mongoManager must be set in multi-tenant mode")
	assert.NotNil(t, result.metadataRepo, "metadataRepo must be set in multi-tenant mode")
	assert.Nil(t, result.connection, "connection must be nil in multi-tenant mode")
}

func TestInitMultiTenantMongo_NilTenantClient_ReturnsError(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       nil,
		TenantServiceName:  "transaction",
	}

	result, err := initMultiTenantMongo(opts, logger)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "TenantClient is required")
}

func TestInitSingleTenantMongo_CreatesComponents(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	// Empty config: ensureMongoIndexes will log warnings (no real MongoDB)
	// but initSingleTenantMongo will still return valid components.
	cfg := &Config{}

	result, err := initSingleTenantMongo(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotNil(t, result.connection, "single-tenant mode must have a non-nil connection")
	assert.NotNil(t, result.metadataRepo, "single-tenant mode must have a non-nil metadataRepo")
	assert.Nil(t, result.mongoManager, "single-tenant mode must have a nil mongoManager")
}
