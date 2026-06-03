// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clog "github.com/LerianStudio/lib-observability/log"
)

func TestInitEventListener_NilCleanupWhenRedisHostEmpty(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MultiTenantRedisHost: "",
	}
	logger := clog.NewNop()

	cleanup, err := initEventListener(cfg, logger, nil, nil, nil, nil)

	require.NoError(t, err)
	assert.Nil(t, cleanup, "cleanup must be nil when MULTI_TENANT_REDIS_HOST is empty")
}

func TestInitEventListener_NilCleanupWhenRedisHostWhitespace(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MultiTenantRedisHost: "   ",
	}
	logger := clog.NewNop()

	cleanup, err := initEventListener(cfg, logger, nil, nil, nil, nil)

	require.NoError(t, err)
	assert.Nil(t, cleanup, "cleanup must be nil when MULTI_TENANT_REDIS_HOST is whitespace")
}

func TestBuildMultiTenantRedisClientForWorker_DefaultPort(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MultiTenantRedisHost:     "mt-redis",
		MultiTenantRedisPort:     "",
		MultiTenantRedisPassword: "secret",
	}

	client, err := buildMultiTenantRedisClientForWorker(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify the client was created (we can't inspect internal options easily,
	// but we can verify it's non-nil and closeable).
	err = client.Close()
	assert.NoError(t, err)
}

func TestBuildMultiTenantRedisClientForWorker_CustomPort(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MultiTenantRedisHost: "mt-redis",
		MultiTenantRedisPort: "6380",
	}

	client, err := buildMultiTenantRedisClientForWorker(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	err = client.Close()
	assert.NoError(t, err)
}

func TestBuildDispatcherOptions_BaseOptionsWithoutManagers(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MultiTenantCacheTTLSec: 120,
	}
	logger := clog.NewNop()

	// When both managers are nil, we get base options: logger, cacheTTL, onAdded, onRemoved = 4
	opts := buildDispatcherOptions(cfg, logger, nil, nil, nil, nil, 0)

	assert.Len(t, opts, 4, "must have 4 base dispatcher options when no infra managers provided")
}

func TestInitEventListener_GracefulDegradation_NoError(t *testing.T) {
	t.Parallel()

	// Verify that empty MULTI_TENANT_REDIS_HOST produces no error and nil cleanup.
	// This is the backward-compatibility contract: workers without Redis Pub/Sub
	// continue to work with lazy-load discovery only.
	cfg := &Config{
		MultiTenantRedisHost: "",
		MultiTenantEnabled:   true,
		MultiTenantURL:       "http://tenant-manager:8080",
	}
	logger := clog.NewNop()

	cleanup, err := initEventListener(cfg, logger, nil, nil, nil, nil)

	require.NoError(t, err, "initEventListener must not error when Redis host is empty")
	assert.Nil(t, cleanup, "cleanup must be nil for graceful degradation")
}

func TestPerformInitialTenantSync_NilClient(t *testing.T) {
	t.Parallel()

	logger := clog.NewNop()

	// Must not panic when tmClient is nil — logs a warning and returns.
	assert.NotPanics(t, func() {
		performInitialTenantSync(t.Context(), logger, nil, nil)
	})
}
