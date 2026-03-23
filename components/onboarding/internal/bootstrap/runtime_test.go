// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildShutdownHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pg        *libPostgres.Client
		mongo     *libMongo.Client
		redisConn *libRedis.Client
		wantCount int
	}{
		{
			name:      "all nil dependencies produce no hooks",
			wantCount: 0,
		},
		{
			name:      "all configured dependencies produce three hooks",
			pg:        &libPostgres.Client{},
			mongo:     &libMongo.Client{},
			redisConn: &libRedis.Client{},
			wantCount: 3,
		},
		{
			name:      "partial dependencies only produce configured hooks",
			mongo:     &libMongo.Client{},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hooks := buildShutdownHooks(tt.pg, tt.mongo, tt.redisConn)

			require.Len(t, hooks, tt.wantCount)
			for _, hook := range hooks {
				assert.NotNil(t, hook)
			}
		})
	}
}

func TestNewServer_ServerAddressFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{
			name: "prefixed address takes precedence",
			cfg: &Config{
				PrefixedServerAddress: ":4000",
				ServerAddress:         ":3000",
			},
			want: ":4000",
		},
		{
			name: "base address is used when prefixed is empty",
			cfg: &Config{
				ServerAddress: ":3000",
			},
			want: ":3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			telemetry := zeroTelemetry()
			server := NewServer(tt.cfg, fiber.New(), nil, telemetry)
			require.NotNil(t, server)
			assert.Equal(t, tt.want, server.ServerAddress())
		})
	}
}

func zeroTelemetry() *libOpentelemetry.Telemetry {
	return &libOpentelemetry.Telemetry{}
}

func TestResolveSettingsCacheTTL(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	assert.Equal(t, 5*time.Minute, resolveSettingsCacheTTL(&Config{}, logger))
	assert.Equal(t, 5*time.Minute, resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "invalid"}, logger))
	assert.Equal(t, 5*time.Minute, resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "-5s"}, logger))
	assert.Equal(t, 30*time.Second, resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "30s"}, logger))
}
