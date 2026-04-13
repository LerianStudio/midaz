// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBootstrapTestLogger(t *testing.T) libLog.Logger {
	t.Helper()

	logger, err := libZap.New(libZap.Config{
		Environment:     libZap.EnvironmentLocal,
		Level:           "info",
		OTelLibraryName: ApplicationName,
	})
	require.NoError(t, err)

	return logger
}

func TestResolveLoggerEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  string
		want libZap.Environment
	}{
		{name: "production exact", env: "production", want: libZap.EnvironmentProduction},
		{name: "staging exact", env: "staging", want: libZap.EnvironmentStaging},
		{name: "uat exact", env: "uat", want: libZap.EnvironmentUAT},
		{name: "development exact", env: "development", want: libZap.EnvironmentDevelopment},
		{name: "case insensitive trimmed", env: "  PrOdUcTiOn  ", want: libZap.EnvironmentProduction},
		{name: "unknown falls back to development", env: "qa", want: libZap.EnvironmentDevelopment},
		{name: "empty falls back to development", env: "", want: libZap.EnvironmentDevelopment},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, resolveLoggerEnvironment(tt.env))
		})
	}
}

func TestBuildRedisConfig_SelectsExpectedTopology(t *testing.T) {
	t.Parallel()

	logger := newBootstrapTestLogger(t)

	tests := []struct {
		name           string
		cfg            Config
		assertTopology func(t *testing.T, redisCfg libRedis.Config)
	}{
		{
			name: "standalone for single host",
			cfg:  Config{RedisHost: "127.0.0.1:6379"},
			assertTopology: func(t *testing.T, redisCfg libRedis.Config) {
				t.Helper()
				require.NotNil(t, redisCfg.Topology.Standalone)
				assert.Equal(t, "127.0.0.1:6379", redisCfg.Topology.Standalone.Address)
				assert.Nil(t, redisCfg.Topology.Cluster)
				assert.Nil(t, redisCfg.Topology.Sentinel)
			},
		},
		{
			name: "cluster for multiple hosts without master name",
			cfg:  Config{RedisHost: "10.0.0.1:6379,10.0.0.2:6379"},
			assertTopology: func(t *testing.T, redisCfg libRedis.Config) {
				t.Helper()
				require.NotNil(t, redisCfg.Topology.Cluster)
				assert.Equal(t, []string{"10.0.0.1:6379", "10.0.0.2:6379"}, redisCfg.Topology.Cluster.Addresses)
				assert.Nil(t, redisCfg.Topology.Standalone)
				assert.Nil(t, redisCfg.Topology.Sentinel)
			},
		},
		{
			name: "sentinel takes precedence over cluster",
			cfg: Config{
				RedisHost:       "10.0.0.1:26379,10.0.0.2:26379",
				RedisMasterName: "midaz-master",
			},
			assertTopology: func(t *testing.T, redisCfg libRedis.Config) {
				t.Helper()
				require.NotNil(t, redisCfg.Topology.Sentinel)
				assert.Equal(t, []string{"10.0.0.1:26379", "10.0.0.2:26379"}, redisCfg.Topology.Sentinel.Addresses)
				assert.Equal(t, "midaz-master", redisCfg.Topology.Sentinel.MasterName)
				assert.Nil(t, redisCfg.Topology.Standalone)
				assert.Nil(t, redisCfg.Topology.Cluster)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			redisCfg, err := buildRedisConfig(&tt.cfg, logger)
			require.NoError(t, err)
			tt.assertTopology(t, redisCfg)
		})
	}
}

func TestBuildRedisConfig_RejectsMissingRedisHost(t *testing.T) {
	t.Parallel()

	_, err := buildRedisConfig(&Config{RedisHost: "   "}, newBootstrapTestLogger(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis host is required")
}

func TestBuildRedisConfig_ConfiguresTLSAndStaticPassword(t *testing.T) {
	t.Parallel()

	redisCfg, err := buildRedisConfig(&Config{
		RedisHost:            "127.0.0.1:6379",
		RedisTLS:             true,
		RedisCACert:          "base64-cert",
		RedisPassword:        "super-secret",
		RedisDB:              7,
		RedisProtocol:        3,
		RedisPoolSize:        32,
		RedisMinIdleConns:    4,
		RedisReadTimeout:     11,
		RedisWriteTimeout:    12,
		RedisDialTimeout:     13,
		RedisPoolTimeout:     14,
		RedisMaxRetries:      6,
		RedisMinRetryBackoff: 15,
		RedisMaxRetryBackoff: 16,
	}, newBootstrapTestLogger(t))

	require.NoError(t, err)
	require.NotNil(t, redisCfg.TLS)
	assert.Equal(t, "base64-cert", redisCfg.TLS.CACertBase64)
	require.NotNil(t, redisCfg.Auth.StaticPassword)
	assert.Equal(t, "super-secret", redisCfg.Auth.StaticPassword.Password)
	assert.Nil(t, redisCfg.Auth.GCPIAM)
	assert.Equal(t, 7, redisCfg.Options.DB)
	assert.Equal(t, 3, redisCfg.Options.Protocol)
	assert.Equal(t, 32, redisCfg.Options.PoolSize)
	assert.Equal(t, 4, redisCfg.Options.MinIdleConns)
	assert.Equal(t, 11*time.Second, redisCfg.Options.ReadTimeout)
	assert.Equal(t, 12*time.Second, redisCfg.Options.WriteTimeout)
	assert.Equal(t, 13*time.Second, redisCfg.Options.DialTimeout)
	assert.Equal(t, 14*time.Second, redisCfg.Options.PoolTimeout)
	assert.Equal(t, 6, redisCfg.Options.MaxRetries)
	assert.Equal(t, 15*time.Millisecond, redisCfg.Options.MinRetryBackoff)
	assert.Equal(t, 16*time.Second, redisCfg.Options.MaxRetryBackoff)
}

func TestBuildRedisConfig_ConfiguresGCPIAMAuth(t *testing.T) {
	t.Parallel()

	redisCfg, err := buildRedisConfig(&Config{
		RedisHost:                    "127.0.0.1:6379",
		RedisUseGCPIAM:               true,
		GoogleApplicationCredentials: "creds-base64",
		RedisServiceAccount:          "svc@example.com",
		RedisTokenLifeTime:           60,
		RedisTokenRefreshDuration:    45,
		RedisPassword:                "must-be-ignored",
	}, newBootstrapTestLogger(t))

	require.NoError(t, err)
	require.NotNil(t, redisCfg.Auth.GCPIAM)
	assert.Equal(t, "creds-base64", redisCfg.Auth.GCPIAM.CredentialsBase64)
	assert.Equal(t, "svc@example.com", redisCfg.Auth.GCPIAM.ServiceAccount)
	assert.Equal(t, 60*time.Minute, redisCfg.Auth.GCPIAM.TokenLifetime)
	assert.Equal(t, 45*time.Minute, redisCfg.Auth.GCPIAM.RefreshEvery)
	assert.Nil(t, redisCfg.Auth.StaticPassword)
}
