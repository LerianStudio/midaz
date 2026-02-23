// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package loader

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestApplyPoolConfig(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/db?sslmode=disable")
	require.NoError(t, err)

	applyPoolConfig(config, PoolConfig{
		MaxConns:          12,
		MinConns:          3,
		MaxConnLifetime:   12 * time.Minute,
		MaxConnIdleTime:   2 * time.Minute,
		HealthCheckPeriod: 15 * time.Second,
		ConnectTimeout:    4 * time.Second,
	})

	require.Equal(t, int32(12), config.MaxConns)
	require.Equal(t, int32(3), config.MinConns)
	require.Equal(t, 12*time.Minute, config.MaxConnLifetime)
	require.Equal(t, 2*time.Minute, config.MaxConnIdleTime)
	require.Equal(t, 15*time.Second, config.HealthCheckPeriod)
	require.Equal(t, 4*time.Second, config.ConnConfig.ConnectTimeout)
}

func TestApplyPoolConfig_MinConnsClampedToMax(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/db?sslmode=disable")
	require.NoError(t, err)

	applyPoolConfig(config, PoolConfig{MaxConns: 4, MinConns: 10})

	require.Equal(t, int32(4), config.MaxConns)
	require.Equal(t, int32(4), config.MinConns)
}
