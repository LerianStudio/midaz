// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Success(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_SHARD_COUNT", "8")
	t.Setenv("AUTHORIZER_SHARD_IDS", "0,1,2")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 8, cfg.ShardCount)
	assert.Equal(t, []int32{0, 1, 2}, cfg.ShardIDs)
	assert.Contains(t, cfg.PostgresDSN, "host=localhost")
	assert.Contains(t, cfg.PostgresDSN, "dbname=transaction")
}

func TestLoadConfig_RejectsDisableSSLInProductionLikeEnv(t *testing.T) {
	t.Setenv("ENV_NAME", "production")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "not allowed in production-like environments")
}

func TestParseInt32CSV_ValidWithUpperBound(t *testing.T) {
	ids, err := parseInt32CSV("0,1,7", 7)
	require.NoError(t, err)
	assert.Equal(t, []int32{0, 1, 7}, ids)
}

func TestParseInt32CSV_RejectsOutOfRangeID(t *testing.T) {
	_, err := parseInt32CSV("0,8", 7)
	require.Error(t, err)
	assert.ErrorContains(t, err, "out of range")
}

func TestParseInt32CSV_RejectsNegativeAndDuplicateIDs(t *testing.T) {
	_, err := parseInt32CSV("-1", 7)
	require.Error(t, err)
	assert.ErrorContains(t, err, "must be >= 0")

	_, err = parseInt32CSV("1,1", 7)
	require.Error(t, err)
	assert.ErrorContains(t, err, "duplicate")
}
