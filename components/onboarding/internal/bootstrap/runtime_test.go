// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubBalancePort struct{}

func (stubBalancePort) CreateBalanceSync(context.Context, mmodel.CreateBalanceInput) (*mmodel.Balance, error) {
	return nil, nil
}

func (stubBalancePort) DeleteAllBalancesByAccountID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) error {
	return nil
}

func (stubBalancePort) CheckHealth(context.Context) error {
	return nil
}

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
		tt := tt

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
		tt := tt

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

func TestResolveBalancePort(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	t.Run("unified mode requires provided balance port", func(t *testing.T) {
		t.Parallel()

		port, err := resolveBalancePort(&Options{UnifiedMode: true}, &Config{}, logger)
		require.Error(t, err)
		assert.Nil(t, port)
	})

	t.Run("unified mode returns injected balance port", func(t *testing.T) {
		t.Parallel()

		expected := stubBalancePort{}
		port, err := resolveBalancePort(&Options{UnifiedMode: true, BalancePort: expected}, &Config{}, logger)
		require.NoError(t, err)
		assert.IsType(t, expected, port)
	})

	t.Run("microservices mode falls back to grpc adapter defaults", func(t *testing.T) {
		t.Parallel()

		port, err := resolveBalancePort(nil, &Config{}, logger)
		require.NoError(t, err)
		require.NotNil(t, port)
		_, ok := port.(mbootstrap.BalancePort)
		assert.True(t, ok)
	})
}

func TestResolveSettingsCacheTTL(t *testing.T) {
	t.Parallel()

	logger := libLog.NewNop()

	assert.Equal(t, time.Duration(0), resolveSettingsCacheTTL(&Config{}, logger))
	assert.Equal(t, time.Duration(0), resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "invalid"}, logger))
	assert.Equal(t, time.Duration(0), resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "-5s"}, logger))
	assert.Equal(t, 30*time.Second, resolveSettingsCacheTTL(&Config{SettingsCacheTTL: "30s"}, logger))
}
