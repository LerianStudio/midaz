// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"strconv"
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
	assert.Equal(t, 2048, cfg.MaxOperationsPerRequest)
	assert.Equal(t, 2048, cfg.MaxUniqueBalancesPerRequest)
	assert.Equal(t, 2048, cfg.WALReplayMaxMutationsPerEntry)
	assert.Equal(t, 2048, cfg.WALReplayMaxUniqueBalancesPerEntry)
	assert.True(t, cfg.WALReplayStrictMode)
	assert.Contains(t, cfg.PostgresDSN, "postgres://")
	assert.Contains(t, cfg.PostgresDSN, "localhost:5432")
	assert.Contains(t, cfg.PostgresDSN, "/transaction")
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

func TestLoadConfig_AllowsDisablingReplayStrictModeExplicitly(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_REPLAY_STRICT_MODE", "false")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.WALReplayStrictMode)
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

func TestLoadConfig_DefaultsSSLModeToRequire(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "")
	t.Setenv("DB_SSLMODE", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Contains(t, cfg.PostgresDSN, "sslmode=require")
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
	require.ErrorContains(t, err, "must be >= 0")

	_, err = parseInt32CSV("1,1", 7)
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate")
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
	assert.ErrorContains(t, err, "too short")
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
	require.ErrorContains(t, err, "AUTHORIZER_OWNED_SHARD_START")

	t.Setenv("AUTHORIZER_OWNED_SHARD_START", "0")
	t.Setenv("AUTHORIZER_OWNED_SHARD_END", "8")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_OWNED_SHARD_END")
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
	require.ErrorContains(t, err, "AUTHORIZER_PREPARE_TIMEOUT_MS")

	t.Setenv("AUTHORIZER_PREPARE_TIMEOUT_MS", "1000")
	t.Setenv("AUTHORIZER_PREPARE_MAX_PENDING", "0")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_PREPARE_MAX_PENDING")

	t.Setenv("AUTHORIZER_PREPARE_MAX_PENDING", "1000")
	t.Setenv("AUTHORIZER_MAX_OPERATIONS_PER_REQUEST", "0")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_MAX_OPERATIONS_PER_REQUEST")

	t.Setenv("AUTHORIZER_MAX_OPERATIONS_PER_REQUEST", "10")
	t.Setenv("AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST", "0")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST")

	t.Setenv("AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST", "10")
	t.Setenv("AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY", "0")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY")

	t.Setenv("AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY", "10")
	t.Setenv("AUTHORIZER_WAL_REPLAY_MAX_UNIQUE_BALANCES_PER_ENTRY", "0")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_REPLAY_MAX_UNIQUE_BALANCES_PER_ENTRY")
}

func TestLoadConfig_RejectsNegativeRequestAndReplayLimits(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")

	t.Setenv("AUTHORIZER_MAX_OPERATIONS_PER_REQUEST", "-1")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_MAX_OPERATIONS_PER_REQUEST")

	t.Setenv("AUTHORIZER_MAX_OPERATIONS_PER_REQUEST", "10")
	t.Setenv("AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST", "-1")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST")

	t.Setenv("AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST", "10")
	t.Setenv("AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY", "-1")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY")

	t.Setenv("AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY", "10")
	t.Setenv("AUTHORIZER_WAL_REPLAY_MAX_UNIQUE_BALANCES_PER_ENTRY", "-1")

	_, err = LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_REPLAY_MAX_UNIQUE_BALANCES_PER_ENTRY")
}

func TestLoadConfig_RejectsShardCountAboveInt32Range(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_SHARD_COUNT", "2147483649")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_SHARD_COUNT")
	require.ErrorContains(t, err, "exceeds supported maximum")
}

func TestLoadConfig_ParsesNewPeerPerformanceFields(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_PREPARE_BOUNDED_WAIT_MS", "100")
	t.Setenv("AUTHORIZER_PEER_CONN_POOL_SIZE", "8")
	t.Setenv("AUTHORIZER_ASYNC_COMMIT_INTENT", "true")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_ENABLED", "true")
	// Ensure timing constraint is satisfied: grace(30s) + interval(10s) < prepareTimeout(60s).
	t.Setenv("AUTHORIZER_PREPARE_TIMEOUT_MS", "60000")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, 100, cfg.PeerPrepareBoundedWaitMs)
	assert.Equal(t, 8, cfg.PeerConnPoolSize)
	assert.True(t, cfg.AsyncCommitIntent)
	assert.True(t, cfg.WALReconcilerEnabled)
}

func TestLoadConfig_RejectsAsyncCommitIntentWithoutReconciler(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_ASYNC_COMMIT_INTENT", "true")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_ENABLED", "false")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_ASYNC_COMMIT_INTENT=true requires AUTHORIZER_WAL_RECONCILER_ENABLED=true")
}

func TestLoadConfig_RejectsInvalidPeerPrepareBoundedWaitMs(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_PREPARE_BOUNDED_WAIT_MS", "-1")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_PEER_PREPARE_BOUNDED_WAIT_MS")
}

func TestLoadConfig_RejectsInvalidPeerConnPoolSize(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_PEER_CONN_POOL_SIZE", "0")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_PEER_CONN_POOL_SIZE")
}

func TestLoadConfig_WALReconcilerDefaults(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.False(t, cfg.WALReconcilerEnabled)
	assert.Equal(t, 10*time.Second, cfg.WALReconcilerInterval)
	assert.Equal(t, 5*time.Minute, cfg.WALReconcilerLookback)
	assert.Equal(t, 30*time.Second, cfg.WALReconcilerGrace)
	assert.Equal(t, 10*time.Minute, cfg.WALReconcilerCompletedTTL)
}

func TestLoadConfig_WALReconcilerCustomValues(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_ENABLED", "true")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_INTERVAL_MS", "5000")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_LOOKBACK_MS", "600000")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_GRACE_MS", "60000")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_COMPLETED_TTL_MS", "1200000")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.True(t, cfg.WALReconcilerEnabled)
	assert.Equal(t, 5*time.Second, cfg.WALReconcilerInterval)
	assert.Equal(t, 10*time.Minute, cfg.WALReconcilerLookback)
	assert.Equal(t, 1*time.Minute, cfg.WALReconcilerGrace)
	assert.Equal(t, 20*time.Minute, cfg.WALReconcilerCompletedTTL)
}

func TestLoadConfig_RejectsZeroReconcilerInterval(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_INTERVAL_MS", "0")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.ErrorContains(t, err, "AUTHORIZER_WAL_RECONCILER_INTERVAL_MS")
}

func TestLoadConfig_RejectsValuesAboveMaxCeiling(t *testing.T) {
	cases := []struct {
		name     string
		envVar   string
		maxValue int
	}{
		{
			name:     "AUTHORIZER_PREPARE_MAX_PENDING",
			envVar:   "AUTHORIZER_PREPARE_MAX_PENDING",
			maxValue: maxConfigPrepareMaxPending,
		},
		{
			name:     "AUTHORIZER_MAX_OPERATIONS_PER_REQUEST",
			envVar:   "AUTHORIZER_MAX_OPERATIONS_PER_REQUEST",
			maxValue: maxConfigOperationsPerRequest,
		},
		{
			name:     "AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST",
			envVar:   "AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST",
			maxValue: maxConfigUniqueBalancesPerRequest,
		},
		{
			name:     "AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY",
			envVar:   "AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY",
			maxValue: maxConfigWALReplayMutationsPerEntry,
		},
		{
			name:     "AUTHORIZER_WAL_REPLAY_MAX_UNIQUE_BALANCES_PER_ENTRY",
			envVar:   "AUTHORIZER_WAL_REPLAY_MAX_UNIQUE_BALANCES_PER_ENTRY",
			maxValue: maxConfigWALReplayUniqueBalancesPerEntry,
		},
		{
			name:     "AUTHORIZER_WAL_BUFFER_SIZE",
			envVar:   "AUTHORIZER_WAL_BUFFER_SIZE",
			maxValue: maxConfigWALBufferSize,
		},
		{
			name:     "AUTHORIZER_MAX_RECV_BYTES",
			envVar:   "AUTHORIZER_MAX_RECV_BYTES",
			maxValue: maxConfigReceiveMessageSizeBytes,
		},
		{
			name:     "AUTHORIZER_PEER_NONCE_MAX_ENTRIES",
			envVar:   "AUTHORIZER_PEER_NONCE_MAX_ENTRIES",
			maxValue: maxConfigPeerNonceEntries,
		},
		{
			name:     "AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT",
			envVar:   "AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT",
			maxValue: maxConfigPeerPrepareMaxInFlight,
		},
		{
			name:     "AUTHORIZER_PEER_PREPARE_BOUNDED_WAIT_MS",
			envVar:   "AUTHORIZER_PEER_PREPARE_BOUNDED_WAIT_MS",
			maxValue: maxConfigPeerPrepareBoundedWaitMs,
		},
		{
			name:     "AUTHORIZER_PEER_CONN_POOL_SIZE",
			envVar:   "AUTHORIZER_PEER_CONN_POOL_SIZE",
			maxValue: maxConfigPeerConnPoolSize,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Required env vars so LoadConfig doesn't fail for unrelated reasons.
			t.Setenv("ENV_NAME", "development")
			t.Setenv("DB_TRANSACTION_HOST", "localhost")
			t.Setenv("DB_TRANSACTION_PORT", "5432")
			t.Setenv("DB_TRANSACTION_USER", "midaz")
			t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
			t.Setenv("DB_TRANSACTION_NAME", "transaction")
			t.Setenv("DB_TRANSACTION_SSLMODE", "disable")

			// Set the target env var to maxValue + 1 (one above the ceiling).
			t.Setenv(tc.envVar, strconv.Itoa(tc.maxValue+1))

			_, err := LoadConfig()
			require.Error(t, err, "expected LoadConfig to reject %s=%d (max=%d)", tc.envVar, tc.maxValue+1, tc.maxValue)
			require.ErrorContains(t, err, tc.envVar, "error message should reference the env var name")
		})
	}
}

func TestLoadConfig_RejectsLookbackNotGreaterThanGrace(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_LOOKBACK_MS", "30000")
	t.Setenv("AUTHORIZER_WAL_RECONCILER_GRACE_MS", "30000")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_RECONCILER_LOOKBACK_MS")
	require.ErrorContains(t, err, "AUTHORIZER_WAL_RECONCILER_GRACE_MS")
}

// TestWALPathProductionRejectsTmp proves a production-like ENV_NAME refuses
// to persist WAL state under /tmp. /tmp is world-writable on POSIX systems
// and a classic target for symlink-swap + tmpfs-eviction attacks.
func TestWALPathProductionRejectsTmp(t *testing.T) {
	t.Setenv("ENV_NAME", "production")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "require")
	t.Setenv("AUTHORIZER_WAL_PATH", "/tmp/forbidden-authorizer.wal")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_PATH")
	require.ErrorContains(t, err, "/tmp")
}

// TestWALPathDevelopmentAcceptsTmp documents the complementary case: /tmp is
// fine in non-production envs so local `make up` flows don't need to provision
// /var/lib/midaz before iterating.
func TestWALPathDevelopmentAcceptsTmp(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_PATH", "/tmp/dev-authorizer.wal")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, "/tmp/dev-authorizer.wal", cfg.WALPath)
}

// TestLoadConfig_RejectsMissingWALHMACKey proves AUTHORIZER_WAL_HMAC_KEY is
// load-fails-closed. Operators cannot accidentally run without authenticated
// WAL frames.
func TestLoadConfig_RejectsMissingWALHMACKey(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_HMAC_KEY", "")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_HMAC_KEY")
}

// TestLoadConfig_RejectsShortWALHMACKey proves the 32-byte minimum is
// enforced at startup, not only at HMAC construction.
func TestLoadConfig_RejectsShortWALHMACKey(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_HMAC_KEY", "Short1Key")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_HMAC_KEY")
}

// TestLoadConfig_RejectsWeakWALHMACKey proves denylisted placeholder values
// (e.g. "changeme", "password") are rejected regardless of length padding.
func TestLoadConfig_RejectsWeakWALHMACKey(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_HMAC_KEY", "00000000000000000000000000000000")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_HMAC_KEY")
}

// TestLoadConfig_RejectsDuplicateWALHMACPreviousKey prevents operators from
// rotating to the same key twice (which would defeat the purpose of rotation
// and leaves a visible audit trail).
func TestLoadConfig_RejectsDuplicateWALHMACPreviousKey(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_HMAC_KEY", "RotateTestHMACKey32bytes_curent1")
	t.Setenv("AUTHORIZER_WAL_HMAC_KEY_PREVIOUS", "RotateTestHMACKey32bytes_curent1")

	_, err := LoadConfig()
	require.Error(t, err)
	require.ErrorContains(t, err, "AUTHORIZER_WAL_HMAC_KEY_PREVIOUS")
}

// TestLoadConfig_AcceptsWALHMACKeyRotation proves a distinct previous key is
// accepted and surfaced on the Config.
func TestLoadConfig_AcceptsWALHMACKeyRotation(t *testing.T) {
	t.Setenv("ENV_NAME", "development")
	t.Setenv("DB_TRANSACTION_HOST", "localhost")
	t.Setenv("DB_TRANSACTION_PORT", "5432")
	t.Setenv("DB_TRANSACTION_USER", "midaz")
	t.Setenv("DB_TRANSACTION_PASSWORD", "secret")
	t.Setenv("DB_TRANSACTION_NAME", "transaction")
	t.Setenv("DB_TRANSACTION_SSLMODE", "disable")
	t.Setenv("AUTHORIZER_WAL_HMAC_KEY", "RotateTestHMACKey32bytes_curent1")
	t.Setenv("AUTHORIZER_WAL_HMAC_KEY_PREVIOUS", "RotateTestHMACKey32bytes_prev001")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Equal(t, []byte("RotateTestHMACKey32bytes_curent1"), cfg.WALHMACKey)
	require.Equal(t, []byte("RotateTestHMACKey32bytes_prev001"), cfg.WALHMACKeyPrevious)
}
