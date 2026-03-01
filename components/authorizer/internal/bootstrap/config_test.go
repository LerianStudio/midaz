// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"
	"time"

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
	assert.Equal(t, 150*time.Millisecond, cfg.AuthorizeLatencySLO)
	assert.Contains(t, cfg.PostgresDSN, "host=localhost")
	assert.Contains(t, cfg.PostgresDSN, "dbname=transaction")
}

func TestLoadConfig_RejectsInvalidAuthorizeLatencySLO(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_AUTHORIZE_LATENCY_SLO_MS", "0")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_AUTHORIZE_LATENCY_SLO_MS")
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

func TestLoadConfig_RequiresPeerAuthTokenWhenPeersConfigured(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_INSTANCES", "authorizer-2:50051")
	t.Setenv("AUTHORIZER_INSTANCE_ADDRESS", "authorizer-1:50051")
	t.Setenv("AUTHORIZER_PEER_AUTH_TOKEN", "")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_PEER_AUTH_TOKEN")
}

func TestLoadConfig_ParsesPeerShardRanges(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_INSTANCES", "authorizer-2:50051,authorizer-3:50051")
	t.Setenv("AUTHORIZER_INSTANCE_ADDRESS", "authorizer-1:50051")
	t.Setenv("AUTHORIZER_PEER_SHARD_RANGES", "0-1,6-7")
	t.Setenv("AUTHORIZER_PEER_AUTH_TOKEN", "Str0ngPeerTokenValue!2026")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, []string{"0-1", "6-7"}, cfg.PeerShardRanges)
	assert.Equal(t, "Str0ngPeerTokenValue!2026", cfg.PeerAuthToken)
}

func TestLoadConfig_RequiresTLSFilesWhenGRPCTLSEnabled(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_GRPC_TLS_ENABLED", "true")
	t.Setenv("AUTHORIZER_GRPC_TLS_CERT_FILE", "")
	t.Setenv("AUTHORIZER_GRPC_TLS_KEY_FILE", "")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_GRPC_TLS_CERT_FILE")
}

func TestLoadConfig_RejectsNegativePeerPrepareMaxInFlight(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT", "-1")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT")
}

func TestLoadConfig_ParsesPeerTransportFlags(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_INSTANCES", "authorizer-2:50051")
	t.Setenv("AUTHORIZER_INSTANCE_ADDRESS", "authorizer-1:50051")
	t.Setenv("AUTHORIZER_PEER_AUTH_TOKEN", "Str0ngPeerTokenValue!2026")
	t.Setenv("AUTHORIZER_PEER_INSECURE_ALLOWED", "true")
	t.Setenv("AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT", "32")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.True(t, cfg.PeerInsecureAllowed)
	assert.Equal(t, 32, cfg.PeerPrepareMaxInFlight)
}

func TestLoadConfig_RejectsWeakPeerAuthToken(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_INSTANCES", "authorizer-2:50051")
	t.Setenv("AUTHORIZER_INSTANCE_ADDRESS", "authorizer-1:50051")
	t.Setenv("AUTHORIZER_PEER_AUTH_TOKEN", "midaz-local-peer-token")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "denied weak value")
}

func TestLoadConfig_RejectsShortPeerAuthToken(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_INSTANCES", "authorizer-2:50051")
	t.Setenv("AUTHORIZER_INSTANCE_ADDRESS", "authorizer-1:50051")
	t.Setenv("AUTHORIZER_PEER_AUTH_TOKEN", "Aa1!short")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "at least")
}

func TestLoadConfig_ValidatesOwnedShardBounds(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_SHARD_COUNT", "8")
	t.Setenv("AUTHORIZER_OWNED_SHARD_START", "-1")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_OWNED_SHARD_START")

	t.Setenv("AUTHORIZER_OWNED_SHARD_START", "0")
	t.Setenv("AUTHORIZER_OWNED_SHARD_END", "8")

	_, err = LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_OWNED_SHARD_END")
}

func TestLoadConfig_RejectsInvalidPrepareSettings(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PREPARE_TIMEOUT_MS", "0")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_PREPARE_TIMEOUT_MS")

	t.Setenv("AUTHORIZER_PREPARE_TIMEOUT_MS", "1000")
	t.Setenv("AUTHORIZER_PREPARE_MAX_PENDING", "0")

	_, err = LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_PREPARE_MAX_PENDING")
}
