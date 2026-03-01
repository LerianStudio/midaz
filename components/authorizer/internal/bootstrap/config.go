// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	brokerpkg "github.com/LerianStudio/midaz/v3/pkg/broker"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

const minPeerAuthTokenLength = 24

var deniedPeerAuthTokens = map[string]struct{}{
	"midaz-local-peer-token": {},
	"changeme":               {},
	"password":               {},
	"secret":                 {},
	"secret-token":           {},
}

// Config is the runtime configuration for the authorizer service.
type Config struct {
	EnvName                    string
	GRPCAddress                string
	InstanceAddress            string
	ShardCount                 int
	ShardIDs                   []int32
	AuthorizeLatencySLO        time.Duration
	EnableTelemetry            bool
	OtelServiceName            string
	OtelLibraryName            string
	OtelServiceVersion         string
	OtelDeploymentEnv          string
	OtelColExporterEndpoint    string
	WALPath                    string
	WALBufferSize              int
	WALFlushInterval           time.Duration
	WALSyncOnAppend            bool
	PrepareTimeout             time.Duration
	PrepareMaxPending          int
	PrepareCommittedRetention  time.Duration
	PrepareCommitRetryLimit    int
	MaxConcurrentStreams       uint32
	MaxReceiveMessageSizeBytes int
	GRPCTLSEnabled             bool
	GRPCTLSCertFile            string
	GRPCTLSKeyFile             string
	ReflectionEnabled          bool
	PostgresDSN                string
	PostgresPoolMaxConns       int32
	PostgresPoolMinConns       int32
	PostgresPoolMaxConnLife    time.Duration
	PostgresPoolMaxConnIdle    time.Duration
	PostgresPoolHealthCheck    time.Duration
	PostgresConnectTimeout     time.Duration
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
}

func LoadConfig() (*Config, error) {
	grpcAddress := getenv("AUTHORIZER_GRPC_ADDRESS", ":50051")
	instanceAddress := strings.TrimSpace(getenv("AUTHORIZER_INSTANCE_ADDRESS", grpcAddress))
	envName := getenv("ENV_NAME", "development")
	shardCount := getenvInt("AUTHORIZER_SHARD_COUNT", 8)
	maxShardID := int32(shardCount - 1)
	if shardCount <= 0 {
		maxShardID = -1
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
	walPath := getenv("AUTHORIZER_WAL_PATH", "/tmp/midaz-authorizer-wal.log")
	walBufferSize := getenvInt("AUTHORIZER_WAL_BUFFER_SIZE", 65536)
	walFlushIntervalMs := getenvInt("AUTHORIZER_WAL_FLUSH_INTERVAL_MS", 1)
	walSyncOnAppend := utils.IsTruthyString(getenv("AUTHORIZER_WAL_SYNC_ON_APPEND", "true"))
	prepareTimeoutMs := getenvInt("AUTHORIZER_PREPARE_TIMEOUT_MS", int((30 * time.Second).Milliseconds()))
	if prepareTimeoutMs <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_PREPARE_TIMEOUT_MS=%d must be > 0", prepareTimeoutMs)
	}

	prepareMaxPending := getenvInt("AUTHORIZER_PREPARE_MAX_PENDING", 10000)
	if prepareMaxPending <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_PREPARE_MAX_PENDING=%d must be > 0", prepareMaxPending)
	}

	prepareCommittedRetentionMs := getenvInt("AUTHORIZER_PREPARED_COMMITTED_RETENTION_MS", int((24 * time.Hour).Milliseconds()))
	if prepareCommittedRetentionMs <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_PREPARED_COMMITTED_RETENTION_MS=%d must be > 0", prepareCommittedRetentionMs)
	}

	prepareCommitRetryLimit := getenvInt("AUTHORIZER_PREPARE_COMMIT_RETRY_LIMIT", 3)
	if prepareCommitRetryLimit <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_PREPARE_COMMIT_RETRY_LIMIT=%d must be > 0", prepareCommitRetryLimit)
	}

	maxConcurrentStreams := utils.GetUint32FromIntWithDefault(getenvInt("AUTHORIZER_MAX_CONCURRENT_STREAMS", 1000), 1000)
	maxReceiveBytes := getenvInt("AUTHORIZER_MAX_RECV_BYTES", 4*1024*1024)
	authorizeLatencySLOMs := getenvInt("AUTHORIZER_AUTHORIZE_LATENCY_SLO_MS", 150)
	if authorizeLatencySLOMs <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_AUTHORIZE_LATENCY_SLO_MS=%d must be > 0", authorizeLatencySLOMs)
	}

	grpcTLSEnabled := utils.IsTruthyString(getenv("AUTHORIZER_GRPC_TLS_ENABLED", "false"))
	grpcTLSCertFile := strings.TrimSpace(getenv("AUTHORIZER_GRPC_TLS_CERT_FILE", ""))
	grpcTLSKeyFile := strings.TrimSpace(getenv("AUTHORIZER_GRPC_TLS_KEY_FILE", ""))

	if grpcTLSEnabled && (grpcTLSCertFile == "" || grpcTLSKeyFile == "") {
		return nil, fmt.Errorf("AUTHORIZER_GRPC_TLS_CERT_FILE and AUTHORIZER_GRPC_TLS_KEY_FILE are required when AUTHORIZER_GRPC_TLS_ENABLED=true")
	}

	reflectionEnabled := utils.IsTruthyString(getenv("AUTHORIZER_GRPC_REFLECTION_ENABLED", "false"))
	postgresPoolMaxConns := getenvInt32("AUTHORIZER_DB_MAX_CONNS", 20)
	postgresPoolMinConns := getenvInt32("AUTHORIZER_DB_MIN_CONNS", 2)
	postgresPoolMaxConnLifeMs := getenvInt("AUTHORIZER_DB_MAX_CONN_LIFETIME_MS", int((30 * time.Minute).Milliseconds()))
	postgresPoolMaxConnIdleMs := getenvInt("AUTHORIZER_DB_MAX_CONN_IDLE_MS", int((5 * time.Minute).Milliseconds()))
	postgresPoolHealthCheckMs := getenvInt("AUTHORIZER_DB_HEALTHCHECK_MS", int((30 * time.Second).Milliseconds()))
	postgresConnectTimeoutMs := getenvInt("AUTHORIZER_DB_CONNECT_TIMEOUT_MS", int((5 * time.Second).Milliseconds()))
	redpandaEnabled := utils.IsTruthyString(getenv("AUTHORIZER_REDPANDA_ENABLED", "true"))

	host := utils.EnvFallback(os.Getenv("DB_TRANSACTION_HOST"), os.Getenv("DB_HOST"))
	port := utils.EnvFallback(os.Getenv("DB_TRANSACTION_PORT"), os.Getenv("DB_PORT"))
	user := utils.EnvFallback(os.Getenv("DB_TRANSACTION_USER"), os.Getenv("DB_USER"))
	password := utils.EnvFallback(os.Getenv("DB_TRANSACTION_PASSWORD"), os.Getenv("DB_PASSWORD"))
	dbName := utils.EnvFallback(os.Getenv("DB_TRANSACTION_NAME"), os.Getenv("DB_NAME"))
	sslMode := utils.EnvFallback(os.Getenv("DB_TRANSACTION_SSLMODE"), os.Getenv("DB_SSLMODE"))

	if host == "" || port == "" || user == "" || dbName == "" {
		return nil, fmt.Errorf("missing postgres configuration for authorizer")
	}

	if sslMode == "" {
		sslMode = "disable"
	}

	if !brokersecurity.IsNonProductionEnvironment(envName) && strings.EqualFold(strings.TrimSpace(sslMode), "disable") {
		return nil, fmt.Errorf("DB_TRANSACTION_SSLMODE=disable is not allowed in production-like environments")
	}

	postgresDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", host, port, user, password, dbName, sslMode)

	redpandaBrokersRaw := strings.TrimSpace(os.Getenv("AUTHORIZER_REDPANDA_BROKERS"))
	if redpandaBrokersRaw == "" {
		redpandaBrokersRaw = strings.TrimSpace(os.Getenv("REDPANDA_BROKERS"))
	}

	if redpandaBrokersRaw == "" {
		redpandaBrokersRaw = "127.0.0.1:9092"
	}

	redpandaBrokers := brokerpkg.ParseSeedBrokers(redpandaBrokersRaw)
	redpandaTLSEnabled := utils.IsTruthyString(utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_TLS_ENABLED"), os.Getenv("REDPANDA_TLS_ENABLED")))
	redpandaTLSInsecureSkip := utils.IsTruthyString(utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_TLS_INSECURE_SKIP_VERIFY"), os.Getenv("REDPANDA_TLS_INSECURE_SKIP_VERIFY")))
	redpandaTLSCAFile := utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_TLS_CA_FILE"), os.Getenv("REDPANDA_TLS_CA_FILE"))
	redpandaSASLEnabled := utils.IsTruthyString(utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_SASL_ENABLED"), os.Getenv("REDPANDA_SASL_ENABLED")))
	redpandaSASLMechanism := utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_SASL_MECHANISM"), os.Getenv("REDPANDA_SASL_MECHANISM"))
	if strings.TrimSpace(redpandaSASLMechanism) == "" {
		redpandaSASLMechanism = "SCRAM-SHA-256"
	}
	redpandaSASLUsername := utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_SASL_USERNAME"), os.Getenv("REDPANDA_SASL_USERNAME"))
	redpandaSASLPassword := utils.EnvFallback(os.Getenv("AUTHORIZER_REDPANDA_SASL_PASSWORD"), os.Getenv("REDPANDA_SASL_PASSWORD"))
	redpandaProducerLingerMs := getenvInt("AUTHORIZER_REDPANDA_PRODUCER_LINGER_MS", 5)
	redpandaMaxBufferedRecords := getenvInt("AUTHORIZER_REDPANDA_MAX_BUFFERED_RECORDS", 10000)
	redpandaRecordRetries := getenvInt("AUTHORIZER_REDPANDA_RECORD_RETRIES", 3)
	redpandaDeliveryTimeoutMs := getenvInt("AUTHORIZER_REDPANDA_DELIVERY_TIMEOUT_MS", 30000)
	redpandaPublishTimeoutMs := getenvInt("AUTHORIZER_REDPANDA_PUBLISH_TIMEOUT_MS", 5000)
	redpandaBackpressurePolicy := strings.ToLower(strings.TrimSpace(getenv("AUTHORIZER_REDPANDA_BACKPRESSURE_POLICY", "bounded_wait")))
	commitIntentConsumerGroup := strings.TrimSpace(getenv("AUTHORIZER_COMMIT_INTENT_CONSUMER_GROUP", defaultCommitIntentConsumerGroup))
	commitIntentPollTimeoutMs := getenvInt("AUTHORIZER_COMMIT_INTENT_POLL_TIMEOUT_MS", 1000)
	if commitIntentPollTimeoutMs <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_COMMIT_INTENT_POLL_TIMEOUT_MS=%d must be > 0", commitIntentPollTimeoutMs)
	}

	peerAbortTimeoutMs := getenvInt("AUTHORIZER_PEER_ABORT_TIMEOUT_MS", int((5 * time.Second).Milliseconds()))
	if peerAbortTimeoutMs <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_PEER_ABORT_TIMEOUT_MS=%d must be > 0", peerAbortTimeoutMs)
	}

	peerCommitTimeoutMs := getenvInt("AUTHORIZER_PEER_COMMIT_TIMEOUT_MS", int((10 * time.Second).Milliseconds()))
	if peerCommitTimeoutMs <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_PEER_COMMIT_TIMEOUT_MS=%d must be > 0", peerCommitTimeoutMs)
	}

	peerAuthMaxSkewMs := getenvInt("AUTHORIZER_PEER_AUTH_MAX_SKEW_MS", int((30 * time.Second).Milliseconds()))
	if peerAuthMaxSkewMs <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_PEER_AUTH_MAX_SKEW_MS=%d must be > 0", peerAuthMaxSkewMs)
	}

	peerNonceMaxEntries := getenvInt("AUTHORIZER_PEER_NONCE_MAX_ENTRIES", 100000)
	if peerNonceMaxEntries <= 0 {
		return nil, fmt.Errorf("AUTHORIZER_PEER_NONCE_MAX_ENTRIES=%d must be > 0", peerNonceMaxEntries)
	}

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
	if len(peerInstances) > 0 && peerAuthToken == "" {
		return nil, fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN is required when AUTHORIZER_PEER_INSTANCES is configured")
	}

	if len(peerInstances) > 0 {
		if strings.HasPrefix(instanceAddress, ":") {
			return nil, fmt.Errorf("AUTHORIZER_INSTANCE_ADDRESS must be a routable host:port when AUTHORIZER_PEER_INSTANCES is configured")
		}

		if err := validatePeerAuthToken(peerAuthToken); err != nil {
			return nil, err
		}

		if peerAuthTokenPrevious != "" {
			if err := validatePeerAuthToken(peerAuthTokenPrevious); err != nil {
				return nil, fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN_PREVIOUS is invalid: %w", err)
			}

			if peerAuthTokenPrevious == peerAuthToken {
				return nil, fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN_PREVIOUS must differ from AUTHORIZER_PEER_AUTH_TOKEN")
			}
		}
	}

	peerInsecureAllowed := utils.IsTruthyString(getenv("AUTHORIZER_PEER_INSECURE_ALLOWED", "false"))
	peerTLSCAFile := strings.TrimSpace(getenv("AUTHORIZER_PEER_TLS_CA_FILE", ""))
	peerPrepareMaxInFlight := getenvInt("AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT", 1024)
	if peerPrepareMaxInFlight < 0 {
		return nil, fmt.Errorf("AUTHORIZER_PEER_PREPARE_MAX_INFLIGHT=%d must be >= 0", peerPrepareMaxInFlight)
	}

	ownedShardStart := getenvInt("AUTHORIZER_OWNED_SHARD_START", 0)
	ownedShardEnd := getenvInt("AUTHORIZER_OWNED_SHARD_END", shardCount-1)

	if ownedShardStart < 0 {
		return nil, fmt.Errorf("AUTHORIZER_OWNED_SHARD_START=%d must be >= 0", ownedShardStart)
	}

	if ownedShardEnd >= shardCount {
		return nil, fmt.Errorf("AUTHORIZER_OWNED_SHARD_END=%d must be < AUTHORIZER_SHARD_COUNT=%d", ownedShardEnd, shardCount)
	}

	if ownedShardStart > ownedShardEnd {
		return nil, fmt.Errorf("AUTHORIZER_OWNED_SHARD_START=%d must be <= AUTHORIZER_OWNED_SHARD_END=%d", ownedShardStart, ownedShardEnd)
	}

	return &Config{
		EnvName:                    envName,
		GRPCAddress:                grpcAddress,
		InstanceAddress:            instanceAddress,
		ShardCount:                 shardCount,
		ShardIDs:                   shardIDs,
		AuthorizeLatencySLO:        time.Duration(authorizeLatencySLOMs) * time.Millisecond,
		EnableTelemetry:            enableTelemetry,
		OtelServiceName:            otelServiceName,
		OtelLibraryName:            otelLibraryName,
		OtelServiceVersion:         otelServiceVersion,
		OtelDeploymentEnv:          otelDeploymentEnv,
		OtelColExporterEndpoint:    otelCollectorEndpoint,
		WALPath:                    walPath,
		WALBufferSize:              walBufferSize,
		WALFlushInterval:           time.Duration(walFlushIntervalMs) * time.Millisecond,
		WALSyncOnAppend:            walSyncOnAppend,
		PrepareTimeout:             time.Duration(prepareTimeoutMs) * time.Millisecond,
		PrepareMaxPending:          prepareMaxPending,
		PrepareCommittedRetention:  time.Duration(prepareCommittedRetentionMs) * time.Millisecond,
		PrepareCommitRetryLimit:    prepareCommitRetryLimit,
		MaxConcurrentStreams:       maxConcurrentStreams,
		MaxReceiveMessageSizeBytes: maxReceiveBytes,
		GRPCTLSEnabled:             grpcTLSEnabled,
		GRPCTLSCertFile:            grpcTLSCertFile,
		GRPCTLSKeyFile:             grpcTLSKeyFile,
		ReflectionEnabled:          reflectionEnabled,
		PostgresDSN:                postgresDSN,
		PostgresPoolMaxConns:       postgresPoolMaxConns,
		PostgresPoolMinConns:       postgresPoolMinConns,
		PostgresPoolMaxConnLife:    time.Duration(postgresPoolMaxConnLifeMs) * time.Millisecond,
		PostgresPoolMaxConnIdle:    time.Duration(postgresPoolMaxConnIdleMs) * time.Millisecond,
		PostgresPoolHealthCheck:    time.Duration(postgresPoolHealthCheckMs) * time.Millisecond,
		PostgresConnectTimeout:     time.Duration(postgresConnectTimeoutMs) * time.Millisecond,
		RedpandaEnabled:            redpandaEnabled,
		RedpandaBrokers:            redpandaBrokers,
		RedpandaTLSEnabled:         redpandaTLSEnabled,
		RedpandaTLSInsecureSkip:    redpandaTLSInsecureSkip,
		RedpandaTLSCAFile:          redpandaTLSCAFile,
		RedpandaSASLEnabled:        redpandaSASLEnabled,
		RedpandaSASLMechanism:      redpandaSASLMechanism,
		RedpandaSASLUsername:       redpandaSASLUsername,
		RedpandaSASLPassword:       redpandaSASLPassword,
		RedpandaProducerLinger:     time.Duration(redpandaProducerLingerMs) * time.Millisecond,
		RedpandaMaxBufferedRecords: redpandaMaxBufferedRecords,
		RedpandaRecordRetries:      redpandaRecordRetries,
		RedpandaDeliveryTimeout:    time.Duration(redpandaDeliveryTimeoutMs) * time.Millisecond,
		RedpandaPublishTimeout:     time.Duration(redpandaPublishTimeoutMs) * time.Millisecond,
		RedpandaBackpressurePolicy: redpandaBackpressurePolicy,
		CommitIntentConsumerGroup:  commitIntentConsumerGroup,
		CommitIntentPollTimeout:    time.Duration(commitIntentPollTimeoutMs) * time.Millisecond,
		PeerAbortTimeout:           time.Duration(peerAbortTimeoutMs) * time.Millisecond,
		PeerCommitTimeout:          time.Duration(peerCommitTimeoutMs) * time.Millisecond,
		PeerAuthMaxSkew:            time.Duration(peerAuthMaxSkewMs) * time.Millisecond,
		PeerNonceMaxEntries:        peerNonceMaxEntries,
		PeerInstances:              peerInstances,
		OwnedShardStart:            ownedShardStart,
		OwnedShardEnd:              ownedShardEnd,
		PeerShardRanges:            peerShardRanges,
		PeerAuthToken:              peerAuthToken,
		PeerAuthTokenPrevious:      peerAuthTokenPrevious,
		PeerInsecureAllowed:        peerInsecureAllowed,
		PeerTLSCAFile:              peerTLSCAFile,
		PeerPrepareMaxInFlight:     peerPrepareMaxInFlight,
	}, nil
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
			return nil, err
		}

		id := int32(parsed)
		if id < 0 {
			return nil, fmt.Errorf("shard id %d must be >= 0", id)
		}

		if maxValue >= 0 && id > maxValue {
			return nil, fmt.Errorf("shard id %d out of range (max=%d)", id, maxValue)
		}

		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate shard id %d", id)
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

func getenvInt(name string, fallback int) int {
	value := getenv(name, "")
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getenvInt32(name string, fallback int32) int32 {
	value := getenv(name, "")
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return fallback
	}

	return int32(parsed)
}

func validatePeerAuthToken(token string) error {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN is required when AUTHORIZER_PEER_INSTANCES is configured")
	}

	if _, denied := deniedPeerAuthTokens[strings.ToLower(trimmed)]; denied {
		return fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN uses a denied weak value")
	}

	if len(trimmed) < minPeerAuthTokenLength {
		return fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN must be at least %d characters", minPeerAuthTokenLength)
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

	if classes < 3 {
		return fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN must include at least 3 character classes (lower, upper, digit, symbol)")
	}

	return nil
}
