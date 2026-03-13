// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustTenantClient(t *testing.T, logger libLog.Logger) *tmclient.Client {
	t.Helper()

	client, err := tmclient.NewClient("http://localhost:0", logger, tmclient.WithAllowInsecureHTTP())
	require.NoError(t, err)

	return client
}

func TestInitMongo(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	cfg := &Config{}

	tests := []struct {
		name            string
		opts            *Options
		wantMultiTenant bool
		wantErr         bool
	}{
		{
			name:            "nil opts calls single-tenant path",
			opts:            nil,
			wantMultiTenant: false,
			wantErr:         true,
		},
		{
			name: "multi-tenant disabled calls single-tenant path",
			opts: &Options{
				MultiTenantEnabled: false,
			},
			wantMultiTenant: false,
			wantErr:         true,
		},
		{
			name: "multi-tenant enabled calls multi-tenant path",
			opts: &Options{
				MultiTenantEnabled: true,
				TenantClient:       mustTenantClient(t, logger),
				TenantServiceName:  "onboarding",
			},
			wantMultiTenant: true,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := initMongo(tt.opts, cfg, logger)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)

				return
			}

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

	logger := libLog.NewNop()

	client := mustTenantClient(t, logger)

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       client,
		TenantServiceName:  "onboarding",
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

	logger := libLog.NewNop()

	opts := &Options{
		MultiTenantEnabled: true,
		TenantClient:       nil,
		TenantServiceName:  "onboarding",
	}

	result, err := initMultiTenantMongo(opts, logger)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "TenantClient is required")
}

func TestInitSingleTenantMongo_CreatesComponents(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	// Empty config should fail fast with strict URI validation.
	cfg := &Config{}

	result, err := initSingleTenantMongo(cfg, logger)
	require.Error(t, err)
	assert.Nil(t, result)
}
