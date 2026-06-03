// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisConnection_ToConfig_UsesSentinelForSingleAddressWhenMasterNameSet(t *testing.T) {
	t.Parallel()

	conn := &RedisConnection{
		Address:    []string{"sentinel-1:26379"},
		MasterName: "mymaster",
	}

	cfg := conn.toConfig()
	require.NotNil(t, cfg.Topology.Sentinel)
	assert.Nil(t, cfg.Topology.Standalone)
	assert.Equal(t, []string{"sentinel-1:26379"}, cfg.Topology.Sentinel.Addresses)
	assert.Equal(t, "mymaster", cfg.Topology.Sentinel.MasterName)
}

func TestRedisConnection_ToConfig_PropagatesSecuritySettings(t *testing.T) {
	t.Parallel()

	conn := &RedisConnection{
		Address:                      []string{"redis-1:6379", "redis-2:6379"},
		UseTLS:                       true,
		CACert:                       "base64-ca",
		UseGCPIAMAuth:                true,
		ServiceAccount:               "svc@example.com",
		GoogleApplicationCredentials: "encoded-creds",
		TokenLifeTime:                time.Hour,
		RefreshDuration:              45 * time.Minute,
	}

	cfg := conn.toConfig()
	require.NotNil(t, cfg.Topology.Cluster)
	require.NotNil(t, cfg.TLS)
	require.NotNil(t, cfg.Auth.GCPIAM)

	assert.Equal(t, "base64-ca", cfg.TLS.CACertBase64)
	assert.Equal(t, "svc@example.com", cfg.Auth.GCPIAM.ServiceAccount)
	assert.Equal(t, "encoded-creds", cfg.Auth.GCPIAM.CredentialsBase64)
	assert.Equal(t, time.Hour, cfg.Auth.GCPIAM.TokenLifetime)
	assert.Equal(t, 45*time.Minute, cfg.Auth.GCPIAM.RefreshEvery)
}
