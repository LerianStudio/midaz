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

	brokerpkg "github.com/LerianStudio/midaz/v3/pkg/broker"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// Config is the runtime configuration for the authorizer service.
type Config struct {
	EnvName                    string
	GRPCAddress                string
	ShardCount                 int
	ShardIDs                   []int32
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
	MaxConcurrentStreams       uint32
	MaxReceiveMessageSizeBytes int
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
}

func LoadConfig() (*Config, error) {
	grpcAddress := getenv("AUTHORIZER_GRPC_ADDRESS", ":50051")
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
	maxConcurrentStreams := utils.GetUint32FromIntWithDefault(getenvInt("AUTHORIZER_MAX_CONCURRENT_STREAMS", 1000), 1000)
	maxReceiveBytes := getenvInt("AUTHORIZER_MAX_RECV_BYTES", 4*1024*1024)
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

	return &Config{
		EnvName:                    envName,
		GRPCAddress:                grpcAddress,
		ShardCount:                 shardCount,
		ShardIDs:                   shardIDs,
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
		MaxConcurrentStreams:       maxConcurrentStreams,
		MaxReceiveMessageSizeBytes: maxReceiveBytes,
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
