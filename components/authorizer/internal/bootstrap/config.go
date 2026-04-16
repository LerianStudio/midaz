// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	brokerpkg "github.com/LerianStudio/midaz/v3/pkg/broker"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// Sentinel errors for configuration validation.
var (
	errConfigShardCountMax              = errors.New("AUTHORIZER_SHARD_COUNT exceeds supported maximum")
	errConfigWALFlushInterval           = errors.New("AUTHORIZER_WAL_FLUSH_INTERVAL_MS must be >= 0")
	errConfigPrepareTimeout             = errors.New("AUTHORIZER_PREPARE_TIMEOUT_MS must be > 0")
	errConfigPrepareMaxPending          = errors.New("AUTHORIZER_PREPARE_MAX_PENDING out of range")
	errConfigMaxOpsPerRequest           = errors.New("AUTHORIZER_MAX_OPERATIONS_PER_REQUEST out of range")
	errConfigMaxBalancesPerRequest      = errors.New("AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST out of range")
	errConfigReplayMutations            = errors.New("AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY out of range")
	errConfigReplayBalances             = errors.New("AUTHORIZER_WAL_REPLAY_MAX_UNIQUE_BALANCES_PER_ENTRY out of range")
	errConfigCommittedRetention         = errors.New("AUTHORIZER_PREPARED_COMMITTED_RETENTION_MS must be > 0")
	errConfigCommitRetryLimit           = errors.New("AUTHORIZER_PREPARE_COMMIT_RETRY_LIMIT must be > 0")
	errConfigMaxRecvBytes               = errors.New("AUTHORIZER_MAX_RECV_BYTES out of range")
	errConfigLatencySLO                 = errors.New("AUTHORIZER_AUTHORIZE_LATENCY_SLO_MS must be > 0")
	errConfigTLSFiles                   = errors.New("AUTHORIZER_GRPC_TLS_CERT_FILE and AUTHORIZER_GRPC_TLS_KEY_FILE are required when TLS is enabled")
	errConfigMissingPostgres            = errors.New("missing postgres configuration for authorizer")
	errConfigSSLDisableProduction       = errors.New("DB_TRANSACTION_SSLMODE=disable is not allowed in production-like environments")
	errConfigCommitIntentPollTimeout    = errors.New("AUTHORIZER_COMMIT_INTENT_POLL_TIMEOUT_MS must be > 0")
	errConfigPeerAbortTimeout           = errors.New("AUTHORIZER_PEER_ABORT_TIMEOUT_MS must be > 0")
	errConfigPeerCommitTimeout          = errors.New("AUTHORIZER_PEER_COMMIT_TIMEOUT_MS must be > 0")
	errConfigPeerAuthMaxSkew            = errors.New("AUTHORIZER_PEER_AUTH_MAX_SKEW_MS must be > 0")
	errConfigPeerNonceEntries           = errors.New("AUTHORIZER_PEER_NONCE_MAX_ENTRIES out of range")
	errConfigPeerAuthTokenRequired      = errors.New("AUTHORIZER_PEER_AUTH_TOKEN is required (required for single-instance deployments too; serves as local-daemon authentication)")
	errConfigPeerInstanceAddress        = errors.New("AUTHORIZER_INSTANCE_ADDRESS must be a routable host:port when AUTHORIZER_PEER_INSTANCES is configured")
	errConfigPeerAuthTokenPrevDuplicate = errors.New("AUTHORIZER_PEER_AUTH_TOKEN_PREVIOUS must differ from AUTHORIZER_PEER_AUTH_TOKEN")
	errConfigPeerPrepareMaxInFlight     = errors.New("AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT out of range")
	errConfigPeerBoundedWait            = errors.New("AUTHORIZER_PEER_PREPARE_BOUNDED_WAIT_MS out of range")
	errConfigPeerConnPoolSize           = errors.New("AUTHORIZER_PEER_CONN_POOL_SIZE out of range")
	errConfigReconcilerInterval         = errors.New("AUTHORIZER_WAL_RECONCILER_INTERVAL_MS must be > 0")
	errConfigReconcilerLookback         = errors.New("AUTHORIZER_WAL_RECONCILER_LOOKBACK_MS must be > 0")
	errConfigReconcilerGrace            = errors.New("AUTHORIZER_WAL_RECONCILER_GRACE_MS must be > 0")
	errConfigLookbackGrace              = errors.New("AUTHORIZER_WAL_RECONCILER_LOOKBACK_MS must be > AUTHORIZER_WAL_RECONCILER_GRACE_MS")
	errConfigReconcilerCompletedTTL     = errors.New("AUTHORIZER_WAL_RECONCILER_COMPLETED_TTL_MS must be > 0")
	errConfigAsyncNeedsReconciler       = errors.New("AUTHORIZER_ASYNC_COMMIT_INTENT=true requires AUTHORIZER_WAL_RECONCILER_ENABLED=true")
	errConfigReconcilerTiming           = errors.New("reconciler grace + interval must be less than prepare timeout")
	errConfigWALBufferSize              = errors.New("AUTHORIZER_WAL_BUFFER_SIZE out of range")
	errConfigOwnedShardStart            = errors.New("AUTHORIZER_OWNED_SHARD_START must be >= 0")
	errConfigOwnedShardEnd              = errors.New("AUTHORIZER_OWNED_SHARD_END must be < AUTHORIZER_SHARD_COUNT")
	errConfigShardStartEnd              = errors.New("AUTHORIZER_OWNED_SHARD_START must be <= AUTHORIZER_OWNED_SHARD_END")
	errConfigShardIDNegative            = errors.New("shard id must be >= 0")
	errConfigShardIDOutOfRange          = errors.New("shard id out of range")
	errConfigShardIDDuplicate           = errors.New("duplicate shard id")
	errConfigPeerAuthTokenWeak          = errors.New("AUTHORIZER_PEER_AUTH_TOKEN uses a denied weak value")
	errConfigPeerAuthTokenShort         = errors.New("AUTHORIZER_PEER_AUTH_TOKEN too short")
	errConfigPeerAuthTokenClasses       = errors.New("AUTHORIZER_PEER_AUTH_TOKEN must include at least 3 character classes")
	errConfigWALHMACKeyRequired         = errors.New("AUTHORIZER_WAL_HMAC_KEY is required")
	errConfigWALHMACKeyShort            = errors.New("AUTHORIZER_WAL_HMAC_KEY must be at least 32 bytes")
	errConfigWALHMACKeyWeak             = errors.New("AUTHORIZER_WAL_HMAC_KEY uses a denied weak value")
	errConfigWALHMACKeyClasses          = errors.New("AUTHORIZER_WAL_HMAC_KEY must include at least 3 character classes")
	errConfigWALHMACKeyPrevDuplicate    = errors.New("AUTHORIZER_WAL_HMAC_KEY_PREVIOUS must differ from AUTHORIZER_WAL_HMAC_KEY")
	errConfigWALPathInTmpProduction     = errors.New("AUTHORIZER_WAL_PATH must not live under /tmp in production-like environments")
)

const minPeerAuthTokenLength = 24

// minWALHMACKeyLength matches wal.MinHMACKeyLength (32 bytes / 256 bits).
// Defined here (rather than imported) so the config package can validate
// without a circular import from wal → bootstrap.
const minWALHMACKeyLength = 32

const (
	maxConfigPrepareMaxPending               = 1_000_000
	maxConfigOperationsPerRequest            = 100_000
	maxConfigUniqueBalancesPerRequest        = 100_000
	maxConfigWALReplayMutationsPerEntry      = 100_000
	maxConfigWALReplayUniqueBalancesPerEntry = 100_000
	maxConfigWALBufferSize                   = 8 * 1024 * 1024
	maxConfigReceiveMessageSizeBytes         = 64 * 1024 * 1024
	maxConfigPeerNonceEntries                = 1_000_000
	maxConfigPeerPrepareMaxInFlight          = 1_000_000
	maxConfigPeerPrepareBoundedWaitMs        = 1000
	maxConfigPeerConnPoolSize                = 16
)

// Default configuration values for the authorizer service.
const (
	defaultShardCount                  = 8
	defaultWALBufferSize               = 65536
	defaultWALFlushIntervalMs          = 1
	defaultPrepareMaxPending           = 10000
	defaultMaxOpsPerRequest            = 2048
	defaultMaxBalancesPerRequest       = 2048
	defaultReplayMutationsPerEntry     = 2048
	defaultReplayBalancesPerEntry      = 2048
	defaultPrepareCommitRetryLimit     = 3
	defaultMaxConcurrentStreams        = 1000
	defaultMaxRecvBytes                = 4 * 1024
	defaultPostgresPoolMaxConns        = 20
	defaultPostgresPoolMinConns        = 2
	defaultRedpandaProducerLingerMs    = 5
	defaultRedpandaMaxBufferedRecords  = 10000
	defaultRedpandaRecordRetries       = 3
	defaultRedpandaDeliveryTimeoutMs   = 30000
	defaultRedpandaPublishTimeoutMs    = 5000
	defaultCommitIntentPollTimeoutMs   = 1000
	defaultPeerNonceMaxEntries         = 100000
	defaultPeerPrepareMaxInFlight      = 1024
	defaultPeerPrepareBoundedWaitMs    = 50
	defaultPeerConnPoolSize            = 4
	defaultWALReconcilerIntervalMs     = 10000
	defaultWALReconcilerLookbackMs     = 300000
	defaultWALReconcilerGraceMs        = 30000
	defaultWALReconcilerCompletedTTLMs = 600000
	minPeerAuthTokenCharacterClasses   = 3
	defaultPrepareTimeoutSec           = 30
	defaultCommittedRetentionHours     = 24
	defaultPoolMaxConnLifeMin          = 30
	defaultPoolMaxConnIdleMin          = 5
	defaultPoolHealthCheckSec          = 30
	defaultPoolConnectTimeoutSec       = 5
	defaultPeerAbortTimeoutSec         = 5
	defaultPeerCommitTimeoutSec        = 10
	defaultPeerAuthMaxSkewSec          = 30
)

var deniedPeerAuthTokens = map[string]struct{}{
	"midaz-local-peer-token": {},
	"changeme":               {},
	"password":               {},
	"secret":                 {},
	"secret-token":           {},
}

// deniedWALHMACKeys lists obvious weak or placeholder HMAC keys that MUST be
// rejected before the authorizer starts. Additions should lowercase the value
// before inserting; lookups normalize with strings.ToLower.
var deniedWALHMACKeys = map[string]struct{}{
	"changeme":                         {},
	"midaz-local-wal-hmac-key":         {},
	"00000000000000000000000000000000": {},
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": {},
	"password":                         {},
	"secret":                           {},
}

// Config is the runtime configuration for the authorizer service.
type Config struct {
	EnvName                 string
	GRPCAddress             string
	InstanceAddress         string
	ShardCount              int
	ShardIDs                []int32
	AuthorizeLatencySLO     time.Duration
	EnableTelemetry         bool
	OtelServiceName         string
	OtelLibraryName         string
	OtelServiceVersion      string
	OtelDeploymentEnv       string
	OtelColExporterEndpoint string
	WALPath                 string
	WALBufferSize           int
	WALFlushInterval        time.Duration
	WALSyncOnAppend         bool
	// WALHMACKey is the 32+ byte shared secret used to authenticate WAL frames
	// (HMAC-SHA256). Loaded from AUTHORIZER_WAL_HMAC_KEY; required at startup.
	WALHMACKey []byte
	// WALHMACKeyPrevious is an optional previous key accepted on verification
	// during key rotation. Loaded from AUTHORIZER_WAL_HMAC_KEY_PREVIOUS.
	WALHMACKeyPrevious                 []byte
	PrepareTimeout                     time.Duration
	PrepareMaxPending                  int
	MaxOperationsPerRequest            int
	MaxUniqueBalancesPerRequest        int
	WALReplayMaxMutationsPerEntry      int
	WALReplayMaxUniqueBalancesPerEntry int
	WALReplayStrictMode                bool
	PrepareCommittedRetention          time.Duration
	PrepareCommitRetryLimit            int
	MaxConcurrentStreams               uint32
	MaxReceiveMessageSizeBytes         int
	GRPCTLSEnabled                     bool
	GRPCTLSCertFile                    string
	GRPCTLSKeyFile                     string
	ReflectionEnabled                  bool
	PostgresDSN                        string
	PostgresPoolMaxConns               int32
	PostgresPoolMinConns               int32
	PostgresPoolMaxConnLife            time.Duration
	PostgresPoolMaxConnIdle            time.Duration
	PostgresPoolHealthCheck            time.Duration
	PostgresConnectTimeout             time.Duration
	// PostgresStatementTimeout is applied as the `statement_timeout` runtime
	// parameter on every pool connection. Bounds worst-case scan latency
	// during cold start so a degraded PG replica cannot hang bootstrap past
	// the readiness gate timeout (D1 audit finding #3).
	PostgresStatementTimeout   time.Duration
	RedpandaEnabled            bool
	RedpandaBrokers            []string
	RedpandaTLSEnabled         bool
	RedpandaTLSInsecureSkip    bool
	RedpandaTLSCAFile          string
	RedpandaSASLEnabled        bool
	RedpandaSASLMechanism      string
	RedpandaSASLUsername       string
	RedpandaSASLPassword       string
	RedpandaProducerLinger     time.Duration
	RedpandaMaxBufferedRecords int
	RedpandaRecordRetries      int
	RedpandaDeliveryTimeout    time.Duration
	RedpandaPublishTimeout     time.Duration
	RedpandaBackpressurePolicy string
	CommitIntentConsumerGroup  string
	CommitIntentPollTimeout    time.Duration
	PeerAbortTimeout           time.Duration
	PeerCommitTimeout          time.Duration
	PeerAuthMaxSkew            time.Duration
	PeerNonceMaxEntries        int

	// PeerInstances lists the gRPC addresses of other authorizer instances
	// in the cluster (e.g., "authorizer-2:50051"). Used for cross-shard
	// 2PC coordination when a transaction spans multiple authorizer instances.
	PeerInstances []string
	// OwnedShardStart is the first shard ID owned by this instance (inclusive).
	OwnedShardStart int
	// OwnedShardEnd is the last shard ID owned by this instance (inclusive).
	OwnedShardEnd int

	// PeerShardRanges optionally defines explicit shard ranges for each peer instance.
	// Format: "start-end,start-end" in the same order as AUTHORIZER_PEER_INSTANCES.
	PeerShardRanges []string

	// PeerAuthToken is required when peer instances are configured.
	// It is used as a shared secret header for 2PC peer RPCs.
	PeerAuthToken string

	// PeerAuthTokenPrevious allows zero-downtime HMAC shared-secret rotation.
	// Outbound requests are signed with PeerAuthToken; inbound verification accepts both.
	PeerAuthTokenPrevious string

	// PeerInsecureAllowed explicitly allows insecure peer RPC transport in non-production
	// environments when gRPC TLS is disabled.
	PeerInsecureAllowed bool

	// PeerTLSCAFile pins the trusted CA bundle used for peer mTLS verification.
	PeerTLSCAFile string

	// PeerPrepareMaxInFlight limits concurrent PrepareAuthorize peer RPCs.
	PeerPrepareMaxInFlight int

	// PeerPrepareBoundedWaitMs is the maximum time in milliseconds to wait for a
	// prepare semaphore slot before shedding the request. 0 means immediate shed.
	PeerPrepareBoundedWaitMs int

	// PeerConnPoolSize controls how many gRPC connections to maintain per peer.
	// Multiple connections avoid HTTP/2 TCP head-of-line blocking under high concurrency.
	PeerConnPoolSize int

	// AsyncCommitIntent enables asynchronous commit intent publishing.
	// When true, local commits proceed in parallel with the Redpanda publish,
	// reducing lock hold time. Protected by commit intent recovery on failure.
	AsyncCommitIntent bool

	// WALReconcilerEnabled enables the periodic WAL reconciler goroutine.
	// When true, the reconciler scans for WAL entries that have not been
	// confirmed by Redpanda and re-publishes them.
	WALReconcilerEnabled bool
	// WALReconcilerInterval is how often the reconciler runs.
	WALReconcilerInterval time.Duration
	// WALReconcilerLookback is how far back in the WAL the reconciler scans.
	WALReconcilerLookback time.Duration
	// WALReconcilerGrace is the minimum age of a WAL entry before the
	// reconciler considers it stale. Prevents re-publishing in-flight entries.
	WALReconcilerGrace time.Duration
	// WALReconcilerCompletedTTL is how long completed WAL entries are kept
	// before being eligible for garbage collection.
	WALReconcilerCompletedTTL time.Duration
}

// LoadConfig reads environment variables and returns a validated Config.
func LoadConfig() (*Config, error) {
	cfg, err := loadCoreConfig()
	if err != nil {
		return nil, err
	}

	if err := loadPeerConfig(cfg); err != nil {
		return nil, err
	}

	if err := loadReconcilerConfig(cfg); err != nil {
		return nil, err
	}

	if err := validateWALPath(cfg); err != nil {
		return nil, err
	}

	if err := validateOwnedShards(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

//nolint:gocyclo,cyclop // loadCoreConfig reads all core env vars; splitting further harms readability.
func loadCoreConfig() (*Config, error) {
	grpcAddress := getenv("AUTHORIZER_GRPC_ADDRESS", ":50051")
	instanceAddress := strings.TrimSpace(getenv("AUTHORIZER_INSTANCE_ADDRESS", grpcAddress))
	envName := getenv("ENV_NAME", "development")
	shardCount := getenvInt("AUTHORIZER_SHARD_COUNT", defaultShardCount)
	maxShardID := int32(-1)

	if shardCount > 0 {
		if shardCount > int(math.MaxInt32)+1 {
			return nil, fmt.Errorf("AUTHORIZER_SHARD_COUNT=%d: %w", shardCount, errConfigShardCountMax)
		}

		maxShardID = int32(shardCount - 1)
	}

	shardIDs, err := parseInt32CSV(getenv("AUTHORIZER_SHARD_IDS", ""), maxShardID)
	if err != nil {
		return nil, fmt.Errorf("invalid AUTHORIZER_SHARD_IDS: %w", err)
	}

	enableTelemetry := utils.IsTruthyString(getenv("ENABLE_TELEMETRY", "false"))
	otelServiceName := getenv("OTEL_RESOURCE_SERVICE_NAME", "authorizer")
	otelLibraryName := getenv("OTEL_LIBRARY_NAME", "midaz-authorizer")
	otelServiceVersion := getenv("OTEL_RESOURCE_SERVICE_VERSION", "v3")
	otelDeploymentEnv := getenv("OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT", envName)
	otelCollectorEndpoint := getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")

	// Default WAL path is explicitly outside /tmp because /tmp is world-writable
	// on POSIX systems and a common target for symlink-swap attacks. Operators
	// may still override via AUTHORIZER_WAL_PATH, but validateWALPath() rejects
	// /tmp-rooted paths in production-like environments.
	walPath := getenv("AUTHORIZER_WAL_PATH", "/var/lib/midaz/authorizer/wal.log")
	walBufferSize := getenvInt("AUTHORIZER_WAL_BUFFER_SIZE", defaultWALBufferSize)

	walHMACKeyRaw, walHMACKeyPreviousRaw, err := loadWALHMACKeys()
	if err != nil {
		return nil, err
	}

	walFlushIntervalMs := getenvInt("AUTHORIZER_WAL_FLUSH_INTERVAL_MS", defaultWALFlushIntervalMs)
	if walFlushIntervalMs < 0 {
		return nil, fmt.Errorf("value=%d: %w", walFlushIntervalMs, errConfigWALFlushInterval)
	}

	walSyncOnAppend := utils.IsTruthyString(getenv("AUTHORIZER_WAL_SYNC_ON_APPEND", "true"))

	prepareTimeoutMs := getenvInt("AUTHORIZER_PREPARE_TIMEOUT_MS", int((defaultPrepareTimeoutSec * time.Second).Milliseconds()))
	if prepareTimeoutMs <= 0 {
		return nil, fmt.Errorf("value=%d: %w", prepareTimeoutMs, errConfigPrepareTimeout)
	}

	prepareMaxPending := getenvInt("AUTHORIZER_PREPARE_MAX_PENDING", defaultPrepareMaxPending)
	if err := validateIntRange(prepareMaxPending, 1, maxConfigPrepareMaxPending, errConfigPrepareMaxPending); err != nil {
		return nil, err
	}

	maxOperationsPerRequest := getenvInt("AUTHORIZER_MAX_OPERATIONS_PER_REQUEST", defaultMaxOpsPerRequest)
	if err := validateIntRange(maxOperationsPerRequest, 1, maxConfigOperationsPerRequest, errConfigMaxOpsPerRequest); err != nil {
		return nil, err
	}

	maxUniqueBalancesPerRequest := getenvInt("AUTHORIZER_MAX_UNIQUE_BALANCES_PER_REQUEST", defaultMaxBalancesPerRequest)
	if err := validateIntRange(maxUniqueBalancesPerRequest, 1, maxConfigUniqueBalancesPerRequest, errConfigMaxBalancesPerRequest); err != nil {
		return nil, err
	}

	walReplayMaxMutationsPerEntry := getenvInt("AUTHORIZER_WAL_REPLAY_MAX_MUTATIONS_PER_ENTRY", defaultReplayMutationsPerEntry)
	if err := validateIntRange(walReplayMaxMutationsPerEntry, 1, maxConfigWALReplayMutationsPerEntry, errConfigReplayMutations); err != nil {
		return nil, err
	}

	walReplayMaxUniqueBalancesPerEntry := getenvInt("AUTHORIZER_WAL_REPLAY_MAX_UNIQUE_BALANCES_PER_ENTRY", defaultReplayBalancesPerEntry)
	if err := validateIntRange(walReplayMaxUniqueBalancesPerEntry, 1, maxConfigWALReplayUniqueBalancesPerEntry, errConfigReplayBalances); err != nil {
		return nil, err
	}

	walReplayStrictMode := utils.IsTruthyString(getenv("AUTHORIZER_WAL_REPLAY_STRICT_MODE", "true"))

	prepareCommittedRetentionMs := getenvInt("AUTHORIZER_PREPARED_COMMITTED_RETENTION_MS", int((defaultCommittedRetentionHours * time.Hour).Milliseconds()))
	if prepareCommittedRetentionMs <= 0 {
		return nil, fmt.Errorf("value=%d: %w", prepareCommittedRetentionMs, errConfigCommittedRetention)
	}

	prepareCommitRetryLimit := getenvInt("AUTHORIZER_PREPARE_COMMIT_RETRY_LIMIT", defaultPrepareCommitRetryLimit)
	if prepareCommitRetryLimit <= 0 {
		return nil, fmt.Errorf("value=%d: %w", prepareCommitRetryLimit, errConfigCommitRetryLimit)
	}

	maxConcurrentStreams := utils.GetUint32FromIntWithDefault(getenvInt("AUTHORIZER_MAX_CONCURRENT_STREAMS", defaultMaxConcurrentStreams), defaultMaxConcurrentStreams)

	maxReceiveBytes := getenvInt("AUTHORIZER_MAX_RECV_BYTES", defaultMaxRecvBytes)
	if err := validateIntRange(maxReceiveBytes, 1, maxConfigReceiveMessageSizeBytes, errConfigMaxRecvBytes); err != nil {
		return nil, err
	}

	authorizeLatencySLOMs := getenvInt("AUTHORIZER_AUTHORIZE_LATENCY_SLO_MS", defaultAuthorizeLatencySLOMs)
	if authorizeLatencySLOMs <= 0 {
		return nil, fmt.Errorf("value=%d: %w", authorizeLatencySLOMs, errConfigLatencySLO)
	}

	grpcTLSEnabled := utils.IsTruthyString(getenv("AUTHORIZER_GRPC_TLS_ENABLED", "false"))
	grpcTLSCertFile := strings.TrimSpace(getenv("AUTHORIZER_GRPC_TLS_CERT_FILE", ""))
	grpcTLSKeyFile := strings.TrimSpace(getenv("AUTHORIZER_GRPC_TLS_KEY_FILE", ""))

	if grpcTLSEnabled && (grpcTLSCertFile == "" || grpcTLSKeyFile == "") {
		return nil, errConfigTLSFiles
	}

	reflectionEnabled := utils.IsTruthyString(getenv("AUTHORIZER_GRPC_REFLECTION_ENABLED", "false"))

	postgresPoolMaxConns := getenvInt32("AUTHORIZER_DB_MAX_CONNS", defaultPostgresPoolMaxConns)
	postgresPoolMinConns := getenvInt32("AUTHORIZER_DB_MIN_CONNS", defaultPostgresPoolMinConns)
	postgresPoolMaxConnLifeMs := getenvInt("AUTHORIZER_DB_MAX_CONN_LIFETIME_MS", int((defaultPoolMaxConnLifeMin * time.Minute).Milliseconds()))
	postgresPoolMaxConnIdleMs := getenvInt("AUTHORIZER_DB_MAX_CONN_IDLE_MS", int((defaultPoolMaxConnIdleMin * time.Minute).Milliseconds()))
	postgresPoolHealthCheckMs := getenvInt("AUTHORIZER_DB_HEALTHCHECK_MS", int((defaultPoolHealthCheckSec * time.Second).Milliseconds()))
	postgresConnectTimeoutMs := getenvInt("AUTHORIZER_DB_CONNECT_TIMEOUT_MS", int((defaultPoolConnectTimeoutSec * time.Second).Milliseconds()))
	// 0 = no statement_timeout. Production deployments should set this to
	// something in the 30s-120s range; tests and local dev leave it unset.
	postgresStatementTimeoutMs := getenvInt("AUTHORIZER_DB_STATEMENT_TIMEOUT_MS", 0)
	redpandaEnabled := utils.IsTruthyString(getenv("AUTHORIZER_REDPANDA_ENABLED", "true"))

	postgresDSN, err := buildPostgresDSN(envName)
	if err != nil {
		return nil, err
	}

	rpCfg := loadRedpandaConfig()

	commitIntentConsumerGroup := strings.TrimSpace(getenv("AUTHORIZER_COMMIT_INTENT_CONSUMER_GROUP", defaultCommitIntentConsumerGroup))

	commitIntentPollTimeoutMs := getenvInt("AUTHORIZER_COMMIT_INTENT_POLL_TIMEOUT_MS", defaultCommitIntentPollTimeoutMs)
	if commitIntentPollTimeoutMs <= 0 {
		return nil, fmt.Errorf("value=%d: %w", commitIntentPollTimeoutMs, errConfigCommitIntentPollTimeout)
	}

	peerAbortTimeoutMs := getenvInt("AUTHORIZER_PEER_ABORT_TIMEOUT_MS", int((defaultPeerAbortTimeoutSec * time.Second).Milliseconds()))
	if peerAbortTimeoutMs <= 0 {
		return nil, fmt.Errorf("value=%d: %w", peerAbortTimeoutMs, errConfigPeerAbortTimeout)
	}

	peerCommitTimeoutMs := getenvInt("AUTHORIZER_PEER_COMMIT_TIMEOUT_MS", int((defaultPeerCommitTimeoutSec * time.Second).Milliseconds()))
	if peerCommitTimeoutMs <= 0 {
		return nil, fmt.Errorf("value=%d: %w", peerCommitTimeoutMs, errConfigPeerCommitTimeout)
	}

	peerAuthMaxSkewMs := getenvInt("AUTHORIZER_PEER_AUTH_MAX_SKEW_MS", int((defaultPeerAuthMaxSkewSec * time.Second).Milliseconds()))
	if peerAuthMaxSkewMs <= 0 {
		return nil, fmt.Errorf("value=%d: %w", peerAuthMaxSkewMs, errConfigPeerAuthMaxSkew)
	}

	peerNonceMaxEntries := getenvInt("AUTHORIZER_PEER_NONCE_MAX_ENTRIES", defaultPeerNonceMaxEntries)
	if err := validateIntRange(peerNonceMaxEntries, 1, maxConfigPeerNonceEntries, errConfigPeerNonceEntries); err != nil {
		return nil, err
	}

	if walBufferSize <= 0 {
		return nil, fmt.Errorf("value=%d must be > 0: %w", walBufferSize, errConfigWALBufferSize)
	}

	if walBufferSize > maxConfigWALBufferSize {
		return nil, fmt.Errorf("value=%d must be <= %d: %w", walBufferSize, maxConfigWALBufferSize, errConfigWALBufferSize)
	}

	return &Config{
		EnvName:                            envName,
		GRPCAddress:                        grpcAddress,
		InstanceAddress:                    instanceAddress,
		ShardCount:                         shardCount,
		ShardIDs:                           shardIDs,
		AuthorizeLatencySLO:                time.Duration(authorizeLatencySLOMs) * time.Millisecond,
		EnableTelemetry:                    enableTelemetry,
		OtelServiceName:                    otelServiceName,
		OtelLibraryName:                    otelLibraryName,
		OtelServiceVersion:                 otelServiceVersion,
		OtelDeploymentEnv:                  otelDeploymentEnv,
		OtelColExporterEndpoint:            otelCollectorEndpoint,
		WALPath:                            walPath,
		WALBufferSize:                      walBufferSize,
		WALFlushInterval:                   time.Duration(walFlushIntervalMs) * time.Millisecond,
		WALSyncOnAppend:                    walSyncOnAppend,
		WALHMACKey:                         []byte(walHMACKeyRaw),
		WALHMACKeyPrevious:                 []byte(walHMACKeyPreviousRaw),
		PrepareTimeout:                     time.Duration(prepareTimeoutMs) * time.Millisecond,
		PrepareMaxPending:                  prepareMaxPending,
		MaxOperationsPerRequest:            maxOperationsPerRequest,
		MaxUniqueBalancesPerRequest:        maxUniqueBalancesPerRequest,
		WALReplayMaxMutationsPerEntry:      walReplayMaxMutationsPerEntry,
		WALReplayMaxUniqueBalancesPerEntry: walReplayMaxUniqueBalancesPerEntry,
		WALReplayStrictMode:                walReplayStrictMode,
		PrepareCommittedRetention:          time.Duration(prepareCommittedRetentionMs) * time.Millisecond,
		PrepareCommitRetryLimit:            prepareCommitRetryLimit,
		MaxConcurrentStreams:               maxConcurrentStreams,
		MaxReceiveMessageSizeBytes:         maxReceiveBytes,
		GRPCTLSEnabled:                     grpcTLSEnabled,
		GRPCTLSCertFile:                    grpcTLSCertFile,
		GRPCTLSKeyFile:                     grpcTLSKeyFile,
		ReflectionEnabled:                  reflectionEnabled,
		PostgresDSN:                        postgresDSN,
		PostgresPoolMaxConns:               postgresPoolMaxConns,
		PostgresPoolMinConns:               postgresPoolMinConns,
		PostgresPoolMaxConnLife:            time.Duration(postgresPoolMaxConnLifeMs) * time.Millisecond,
		PostgresPoolMaxConnIdle:            time.Duration(postgresPoolMaxConnIdleMs) * time.Millisecond,
		PostgresPoolHealthCheck:            time.Duration(postgresPoolHealthCheckMs) * time.Millisecond,
		PostgresConnectTimeout:             time.Duration(postgresConnectTimeoutMs) * time.Millisecond,
		PostgresStatementTimeout:           time.Duration(postgresStatementTimeoutMs) * time.Millisecond,
		RedpandaEnabled:                    redpandaEnabled,
		RedpandaBrokers:                    rpCfg.brokers,
		RedpandaTLSEnabled:                 rpCfg.tlsEnabled,
		RedpandaTLSInsecureSkip:            rpCfg.tlsInsecureSkip,
		RedpandaTLSCAFile:                  rpCfg.tlsCAFile,
		RedpandaSASLEnabled:                rpCfg.saslEnabled,
		RedpandaSASLMechanism:              rpCfg.saslMechanism,
		RedpandaSASLUsername:               rpCfg.saslUsername,
		RedpandaSASLPassword:               rpCfg.saslPassword,
		RedpandaProducerLinger:             time.Duration(rpCfg.producerLingerMs) * time.Millisecond,
		RedpandaMaxBufferedRecords:         rpCfg.maxBufferedRecords,
		RedpandaRecordRetries:              rpCfg.recordRetries,
		RedpandaDeliveryTimeout:            time.Duration(rpCfg.deliveryTimeoutMs) * time.Millisecond,
		RedpandaPublishTimeout:             time.Duration(rpCfg.publishTimeoutMs) * time.Millisecond,
		RedpandaBackpressurePolicy:         rpCfg.backpressurePolicy,
		CommitIntentConsumerGroup:          commitIntentConsumerGroup,
		CommitIntentPollTimeout:            time.Duration(commitIntentPollTimeoutMs) * time.Millisecond,
		PeerAbortTimeout:                   time.Duration(peerAbortTimeoutMs) * time.Millisecond,
		PeerCommitTimeout:                  time.Duration(peerCommitTimeoutMs) * time.Millisecond,
		PeerAuthMaxSkew:                    time.Duration(peerAuthMaxSkewMs) * time.Millisecond,
		PeerNonceMaxEntries:                peerNonceMaxEntries,
	}, nil
}

// redpandaEnvConfig holds Redpanda-specific configuration read from environment variables.
type redpandaEnvConfig struct {
	brokers            []string
	tlsEnabled         bool
	tlsInsecureSkip    bool
	tlsCAFile          string
	saslEnabled        bool
	saslMechanism      string
	saslUsername       string
	saslPassword       string
	producerLingerMs   int
	maxBufferedRecords int
	recordRetries      int
	deliveryTimeoutMs  int
	publishTimeoutMs   int
	backpressurePolicy string
}

func loadRedpandaConfig() redpandaEnvConfig {
	redpandaBrokersRaw := strings.TrimSpace(os.Getenv("AUTHORIZER_REDPANDA_BROKERS"))
	if redpandaBrokersRaw == "" {
		redpandaBrokersRaw = strings.TrimSpace(os.Getenv("REDPANDA_BROKERS"))
	}

	if redpandaBrokersRaw == "" {
		redpandaBrokersRaw = "127.0.0.1:9092"
	}

	saslMechanism := utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_SASL_MECHANISM"), os.Getenv("REDPANDA_SASL_MECHANISM"))

	if strings.TrimSpace(saslMechanism) == "" {
		saslMechanism = "SCRAM-SHA-256"
	}

	return redpandaEnvConfig{
		brokers:            brokerpkg.ParseSeedBrokers(redpandaBrokersRaw),
		tlsEnabled:         utils.IsTruthyString(utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_TLS_ENABLED"), os.Getenv("REDPANDA_TLS_ENABLED"))),
		tlsInsecureSkip:    utils.IsTruthyString(utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_TLS_INSECURE_SKIP_VERIFY"), os.Getenv("REDPANDA_TLS_INSECURE_SKIP_VERIFY"))),
		tlsCAFile:          utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_TLS_CA_FILE"), os.Getenv("REDPANDA_TLS_CA_FILE")),
		saslEnabled:        utils.IsTruthyString(utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_SASL_ENABLED"), os.Getenv("REDPANDA_SASL_ENABLED"))),
		saslMechanism:      saslMechanism,
		saslUsername:       utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_SASL_USERNAME"), os.Getenv("REDPANDA_SASL_USERNAME")),
		saslPassword:       utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_SASL_PASSWORD"), os.Getenv("REDPANDA_SASL_PASSWORD")),
		producerLingerMs:   getenvInt("AUTHORIZER_REDPANDA_PRODUCER_LINGER_MS", defaultRedpandaProducerLingerMs),
		maxBufferedRecords: getenvInt("AUTHORIZER_REDPANDA_MAX_BUFFERED_RECORDS", defaultRedpandaMaxBufferedRecords),
		recordRetries:      getenvInt("AUTHORIZER_REDPANDA_RECORD_RETRIES", defaultRedpandaRecordRetries),
		deliveryTimeoutMs:  getenvInt("AUTHORIZER_REDPANDA_DELIVERY_TIMEOUT_MS", defaultRedpandaDeliveryTimeoutMs),
		publishTimeoutMs:   getenvInt("AUTHORIZER_REDPANDA_PUBLISH_TIMEOUT_MS", defaultRedpandaPublishTimeoutMs),
		backpressurePolicy: strings.ToLower(strings.TrimSpace(getenv("AUTHORIZER_REDPANDA_BACKPRESSURE_POLICY", "bounded_wait"))),
	}
}

func buildPostgresDSN(envName string) (string, error) {
	host := utils.EnvFallback(os.Getenv("DB_TRANSACTION_HOST"), os.Getenv("DB_HOST"))
	port := utils.EnvFallback(os.Getenv("DB_TRANSACTION_PORT"), os.Getenv("DB_PORT"))
	user := utils.EnvFallback(os.Getenv("DB_TRANSACTION_USER"), os.Getenv("DB_USER"))
	password := utils.EnvFallback(os.Getenv("DB_TRANSACTION_PASSWORD"), os.Getenv("DB_PASSWORD"))
	dbName := utils.EnvFallback(os.Getenv("DB_TRANSACTION_NAME"), os.Getenv("DB_NAME"))
	sslMode := utils.EnvFallback(os.Getenv("DB_TRANSACTION_SSLMODE"), os.Getenv("DB_SSLMODE"))

	if host == "" || port == "" || user == "" || dbName == "" {
		return "", errConfigMissingPostgres
	}

	if sslMode == "" {
		sslMode = "require"
	}

	if !brokersecurity.IsNonProductionEnvironment(envName) && strings.EqualFold(strings.TrimSpace(sslMode), "disable") {
		return "", errConfigSSLDisableProduction
	}

	postgresURL := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   dbName,
	}

	if password != "" {
		postgresURL.User = url.UserPassword(user, password)
	} else {
		postgresURL.User = url.User(user)
	}

	query := url.Values{}
	query.Set("sslmode", sslMode)
	postgresURL.RawQuery = query.Encode()

	return postgresURL.String(), nil
}

func loadPeerConfig(cfg *Config) error {
	peerInstancesRaw := getenv("AUTHORIZER_PEER_INSTANCES", "")

	var peerInstances []string

	if peerInstancesRaw != "" {
		for _, addr := range strings.Split(peerInstancesRaw, ",") {
			if trimmed := strings.TrimSpace(addr); trimmed != "" {
				peerInstances = append(peerInstances, trimmed)
			}
		}
	}

	peerShardRangesRaw := getenv("AUTHORIZER_PEER_SHARD_RANGES", "")
	peerShardRanges := make([]string, 0)

	if peerShardRangesRaw != "" {
		for _, rng := range strings.Split(peerShardRangesRaw, ",") {
			if trimmed := strings.TrimSpace(rng); trimmed != "" {
				peerShardRanges = append(peerShardRanges, trimmed)
			}
		}
	}

	peerAuthToken := strings.TrimSpace(getenv("AUTHORIZER_PEER_AUTH_TOKEN", ""))
	peerAuthTokenPrevious := strings.TrimSpace(getenv("AUTHORIZER_PEER_AUTH_TOKEN_PREVIOUS", ""))

	if err := validatePeerAuthTokensAndAddress(peerAuthToken, peerAuthTokenPrevious, cfg.InstanceAddress, len(peerInstances) > 0); err != nil {
		return err
	}

	peerInsecureAllowed := utils.IsTruthyString(getenv("AUTHORIZER_PEER_INSECURE_ALLOWED", "false"))
	peerTLSCAFile := strings.TrimSpace(getenv("AUTHORIZER_PEER_TLS_CA_FILE", ""))

	peerPrepareMaxInFlight := getenvInt("AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT", defaultPeerPrepareMaxInFlight)
	if err := validateIntRange(peerPrepareMaxInFlight, 0, maxConfigPeerPrepareMaxInFlight, errConfigPeerPrepareMaxInFlight); err != nil {
		return err
	}

	peerPrepareBoundedWaitMs := getenvInt("AUTHORIZER_PEER_PREPARE_BOUNDED_WAIT_MS", defaultPeerPrepareBoundedWaitMs)
	if err := validateIntRange(peerPrepareBoundedWaitMs, 0, maxConfigPeerPrepareBoundedWaitMs, errConfigPeerBoundedWait); err != nil {
		return err
	}

	peerConnPoolSize := getenvInt("AUTHORIZER_PEER_CONN_POOL_SIZE", defaultPeerConnPoolSize)
	if err := validateIntRange(peerConnPoolSize, 1, maxConfigPeerConnPoolSize, errConfigPeerConnPoolSize); err != nil {
		return err
	}

	asyncCommitIntent := utils.IsTruthyString(getenv("AUTHORIZER_ASYNC_COMMIT_INTENT", "false"))

	cfg.PeerInstances = peerInstances
	cfg.PeerShardRanges = peerShardRanges
	cfg.PeerAuthToken = peerAuthToken
	cfg.PeerAuthTokenPrevious = peerAuthTokenPrevious
	cfg.PeerInsecureAllowed = peerInsecureAllowed
	cfg.PeerTLSCAFile = peerTLSCAFile
	cfg.PeerPrepareMaxInFlight = peerPrepareMaxInFlight
	cfg.PeerPrepareBoundedWaitMs = peerPrepareBoundedWaitMs
	cfg.PeerConnPoolSize = peerConnPoolSize
	cfg.AsyncCommitIntent = asyncCommitIntent

	return nil
}

func loadReconcilerConfig(cfg *Config) error {
	walReconcilerEnabled := utils.IsTruthyString(getenv("AUTHORIZER_WAL_RECONCILER_ENABLED", "false"))

	walReconcilerIntervalMs := getenvInt("AUTHORIZER_WAL_RECONCILER_INTERVAL_MS", defaultWALReconcilerIntervalMs)
	if walReconcilerIntervalMs <= 0 {
		return fmt.Errorf("value=%d: %w", walReconcilerIntervalMs, errConfigReconcilerInterval)
	}

	walReconcilerLookbackMs := getenvInt("AUTHORIZER_WAL_RECONCILER_LOOKBACK_MS", defaultWALReconcilerLookbackMs)
	if walReconcilerLookbackMs <= 0 {
		return fmt.Errorf("value=%d: %w", walReconcilerLookbackMs, errConfigReconcilerLookback)
	}

	walReconcilerGraceMs := getenvInt("AUTHORIZER_WAL_RECONCILER_GRACE_MS", defaultWALReconcilerGraceMs)
	if walReconcilerGraceMs <= 0 {
		return fmt.Errorf("value=%d: %w", walReconcilerGraceMs, errConfigReconcilerGrace)
	}

	if walReconcilerLookbackMs <= walReconcilerGraceMs {
		return fmt.Errorf("lookback=%d grace=%d: %w", walReconcilerLookbackMs, walReconcilerGraceMs, errConfigLookbackGrace)
	}

	walReconcilerCompletedTTLMs := getenvInt("AUTHORIZER_WAL_RECONCILER_COMPLETED_TTL_MS", defaultWALReconcilerCompletedTTLMs)
	if walReconcilerCompletedTTLMs <= 0 {
		return fmt.Errorf("value=%d: %w", walReconcilerCompletedTTLMs, errConfigReconcilerCompletedTTL)
	}

	if cfg.AsyncCommitIntent && !walReconcilerEnabled {
		return errConfigAsyncNeedsReconciler
	}

	// Reconciler must be able to act before peers auto-abort.
	// grace + interval gives the worst-case time before first reconciler action on a stale entry.
	prepareTimeoutMs := int(cfg.PrepareTimeout.Milliseconds())

	if walReconcilerEnabled && cfg.AsyncCommitIntent && walReconcilerGraceMs+walReconcilerIntervalMs >= prepareTimeoutMs {
		return fmt.Errorf(
			"grace=%d interval=%d prepareTimeout=%d: %w",
			walReconcilerGraceMs, walReconcilerIntervalMs, prepareTimeoutMs, errConfigReconcilerTiming,
		)
	}

	cfg.WALReconcilerEnabled = walReconcilerEnabled
	cfg.WALReconcilerInterval = time.Duration(walReconcilerIntervalMs) * time.Millisecond
	cfg.WALReconcilerLookback = time.Duration(walReconcilerLookbackMs) * time.Millisecond
	cfg.WALReconcilerGrace = time.Duration(walReconcilerGraceMs) * time.Millisecond
	cfg.WALReconcilerCompletedTTL = time.Duration(walReconcilerCompletedTTLMs) * time.Millisecond

	return nil
}

func validateWALPath(cfg *Config) error {
	resolvedWALPath, err := filepath.Abs(filepath.Clean(cfg.WALPath))
	if err != nil {
		return fmt.Errorf("invalid AUTHORIZER_WAL_PATH=%q: %w", cfg.WALPath, err)
	}

	// In production-like environments, disallow WAL paths rooted at /tmp.
	// /tmp is world-writable and a common target for symlink-swap attacks,
	// and persisting durable state there risks loss on reboot under tmpfs.
	if !brokersecurity.IsNonProductionEnvironment(cfg.EnvName) {
		if strings.HasPrefix(resolvedWALPath, "/tmp/") || resolvedWALPath == "/tmp" {
			return fmt.Errorf("path=%q env=%q: %w", resolvedWALPath, cfg.EnvName, errConfigWALPathInTmpProduction)
		}
	}

	cfg.WALPath = resolvedWALPath

	return nil
}

func validateOwnedShards(cfg *Config) error {
	ownedShardStart := getenvInt("AUTHORIZER_OWNED_SHARD_START", 0)
	ownedShardEnd := getenvInt("AUTHORIZER_OWNED_SHARD_END", cfg.ShardCount-1)

	if ownedShardStart < 0 {
		return fmt.Errorf("value=%d: %w", ownedShardStart, errConfigOwnedShardStart)
	}

	if ownedShardEnd >= cfg.ShardCount {
		return fmt.Errorf("end=%d shardCount=%d: %w", ownedShardEnd, cfg.ShardCount, errConfigOwnedShardEnd)
	}

	if ownedShardStart > ownedShardEnd {
		return fmt.Errorf("start=%d end=%d: %w", ownedShardStart, ownedShardEnd, errConfigShardStartEnd)
	}

	cfg.OwnedShardStart = ownedShardStart
	cfg.OwnedShardEnd = ownedShardEnd

	return nil
}

// validateIntRange checks that val is between lower and upper (inclusive), wrapping sentinel on failure.
func validateIntRange(val, lower, upper int, sentinel error) error {
	if val < lower || val > upper {
		return fmt.Errorf("value=%d min=%d max=%d: %w", val, lower, upper, sentinel)
	}

	return nil
}

func parseInt32CSV(raw string, maxValue int32) ([]int32, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	out := make([]int32, 0, len(parts))
	seen := make(map[int32]struct{}, len(parts))

	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}

		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("parsing shard id %q: %w", value, err)
		}

		id := int32(parsed)

		if id < 0 {
			return nil, fmt.Errorf("id=%d: %w", id, errConfigShardIDNegative)
		}

		if maxValue >= 0 && id > maxValue {
			return nil, fmt.Errorf("id=%d max=%d: %w", id, maxValue, errConfigShardIDOutOfRange)
		}

		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("id=%d: %w", id, errConfigShardIDDuplicate)
		}

		seen[id] = struct{}{}

		out = append(out, id)
	}

	return out, nil
}

func getenv(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value
	}

	return fallback
}

// getenvInt parses an integer environment variable, falling back to a default.
// NOTE: stdlib log.Printf is used intentionally because the structured logger
// (OpenTelemetry / zap) is not yet initialized at bootstrap time.
func getenvInt(name string, fallback int) int {
	value := getenv(name, "")
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("authorizer bootstrap: invalid numeric env %s, using default %d", name, fallback)

		return fallback
	}

	return parsed
}

// getenvInt32 parses a 32-bit integer environment variable, falling back to a default.
// NOTE: stdlib log.Printf is used intentionally because the structured logger
// (OpenTelemetry / zap) is not yet initialized at bootstrap time.
func getenvInt32(name string, fallback int32) int32 {
	value := getenv(name, "")
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		log.Printf("authorizer bootstrap: invalid numeric env %s, using default %d", name, fallback)

		return fallback
	}

	return int32(parsed)
}

// validatePeerAuthTokensAndAddress enforces the B4 peer-auth contract at
// bootstrap:
//   - AUTHORIZER_PEER_AUTH_TOKEN is mandatory regardless of peer count. The
//     token authenticates callers (transaction service, sidecars, operator
//     tooling) for every internal RPC — single-instance deployments included.
//     See components/authorizer/internal/bootstrap/grpc.go#Authorize SECURITY
//     notes for the full threat model.
//   - InstanceAddress must be a routable host:port only when peers are
//     configured; token strength/rotation pairing applies unconditionally.
func validatePeerAuthTokensAndAddress(token, previousToken, instanceAddress string, peersConfigured bool) error {
	if token == "" {
		return errConfigPeerAuthTokenRequired
	}

	if peersConfigured && strings.HasPrefix(instanceAddress, ":") {
		return errConfigPeerInstanceAddress
	}

	if err := validatePeerAuthToken(token); err != nil {
		return err
	}

	if previousToken == "" {
		return nil
	}

	if err := validatePeerAuthToken(previousToken); err != nil {
		return fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN_PREVIOUS is invalid: %w", err)
	}

	if previousToken == token {
		return errConfigPeerAuthTokenPrevDuplicate
	}

	return nil
}

func validatePeerAuthToken(token string) error {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return errConfigPeerAuthTokenRequired
	}

	if _, denied := deniedPeerAuthTokens[strings.ToLower(trimmed)]; denied {
		return errConfigPeerAuthTokenWeak
	}

	if len(trimmed) < minPeerAuthTokenLength {
		return fmt.Errorf("length=%d minimum=%d: %w", len(trimmed), minPeerAuthTokenLength, errConfigPeerAuthTokenShort)
	}

	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSymbol := false

	for _, r := range trimmed {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}

	classes := 0

	for _, present := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if present {
			classes++
		}
	}

	if classes < minPeerAuthTokenCharacterClasses {
		return errConfigPeerAuthTokenClasses
	}

	return nil
}

// loadWALHMACKeys reads AUTHORIZER_WAL_HMAC_KEY and its optional _PREVIOUS
// companion from the environment and runs the hygiene checks. Extracted from
// loadCoreConfig to keep that function's cognitive complexity within budget.
func loadWALHMACKeys() (current, previous string, err error) {
	current = strings.TrimSpace(getenv("AUTHORIZER_WAL_HMAC_KEY", ""))
	previous = strings.TrimSpace(getenv("AUTHORIZER_WAL_HMAC_KEY_PREVIOUS", ""))

	if keyErr := validateWALHMACKey(current); keyErr != nil {
		return "", "", keyErr
	}

	if previous == "" {
		return current, "", nil
	}

	if keyErr := validateWALHMACKey(previous); keyErr != nil {
		return "", "", fmt.Errorf("AUTHORIZER_WAL_HMAC_KEY_PREVIOUS is invalid: %w", keyErr)
	}

	if previous == current {
		return "", "", errConfigWALHMACKeyPrevDuplicate
	}

	return current, previous, nil
}

// validateWALHMACKey enforces the same hygiene we apply to peer auth tokens
// (denylist, minimum length, character-class diversity) for the WAL HMAC key.
// The minimum length is 32 bytes so there is at least 256 bits of entropy
// available, matching the block size of SHA-256.
func validateWALHMACKey(key string) error {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return errConfigWALHMACKeyRequired
	}

	if _, denied := deniedWALHMACKeys[strings.ToLower(trimmed)]; denied {
		return errConfigWALHMACKeyWeak
	}

	if len(trimmed) < minWALHMACKeyLength {
		return fmt.Errorf("length=%d minimum=%d: %w", len(trimmed), minWALHMACKeyLength, errConfigWALHMACKeyShort)
	}

	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSymbol := false

	for _, r := range trimmed {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}

	classes := 0

	for _, present := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if present {
			classes++
		}
	}

	if classes < minPeerAuthTokenCharacterClasses {
		return errConfigWALHMACKeyClasses
	}

	return nil
}
