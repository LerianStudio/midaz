// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

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
		{name: "unknown falls back to local", env: "qa", want: libZap.EnvironmentLocal},
		{name: "empty falls back to local", env: "", want: libZap.EnvironmentLocal},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, resolveLoggerEnvironment(tt.env))
		})
	}
}

func TestInitLogger_UsesInjectedLogger(t *testing.T) {
	t.Parallel()

	injectedLogger := newBootstrapTestLogger(t)
	logger, err := initLogger(&Options{Logger: injectedLogger}, &Config{EnvName: "production", LogLevel: "debug"})

	require.NoError(t, err)
	assert.Same(t, injectedLogger, logger)
}

func TestInitLogger_InvalidLevelReturnsError(t *testing.T) {
	t.Parallel()

	logger, err := initLogger(nil, &Config{EnvName: "production", LogLevel: "definitely-not-a-level"})

	assert.Nil(t, logger)
	require.Error(t, err)
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
