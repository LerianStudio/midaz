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

	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// Config is the runtime configuration for the authorizer service.
type Config struct {
	GRPCAddress                string
	ShardCount                 int
	WALEnabled                 bool
	WALPath                    string
	WALBufferSize              int
	WALFlushInterval           time.Duration
	MaxConcurrentStreams       uint32
	MaxReceiveMessageSizeBytes int
	PostgresDSN                string
	RabbitMQEnabled            bool
	RabbitMQURL                string
}

func LoadConfig() (*Config, error) {
	grpcAddress := getenv("AUTHORIZER_GRPC_ADDRESS", ":50051")
	shardCount := getenvInt("AUTHORIZER_SHARD_COUNT", 8)
	walEnabled := utils.IsTruthyString(getenv("AUTHORIZER_WAL_ENABLED", "true"))
	walPath := getenv("AUTHORIZER_WAL_PATH", "/tmp/midaz-authorizer-wal.log")
	walBufferSize := getenvInt("AUTHORIZER_WAL_BUFFER_SIZE", 65536)
	walFlushIntervalMs := getenvInt("AUTHORIZER_WAL_FLUSH_INTERVAL_MS", 1)
	maxConcurrentStreams := uint32(getenvInt("AUTHORIZER_MAX_CONCURRENT_STREAMS", 1000))
	maxReceiveBytes := getenvInt("AUTHORIZER_MAX_RECV_BYTES", 4*1024*1024)
	rabbitEnabled := utils.IsTruthyString(getenv("AUTHORIZER_RABBITMQ_ENABLED", "true"))

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

	postgresDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", host, port, user, password, dbName, sslMode)

	rabbitURL := strings.TrimSpace(os.Getenv("AUTHORIZER_RABBITMQ_URL"))
	if rabbitURL == "" {
		rabbitScheme := getenv("RABBITMQ_URI", "amqp")
		rabbitHost := getenv("RABBITMQ_HOST", "127.0.0.1")
		rabbitPort := getenv("RABBITMQ_PORT_HOST", "5672")
		rabbitUser := getenv("RABBITMQ_DEFAULT_USER", "guest")
		rabbitPass := getenv("RABBITMQ_DEFAULT_PASS", "guest")
		rabbitVHost := strings.TrimSpace(os.Getenv("RABBITMQ_VHOST"))

		if rabbitVHost == "/" || rabbitVHost == "" {
			rabbitURL = fmt.Sprintf("%s://%s:%s@%s:%s/", rabbitScheme, rabbitUser, rabbitPass, rabbitHost, rabbitPort)
		} else {
			rabbitURL = fmt.Sprintf("%s://%s:%s@%s:%s/%s", rabbitScheme, rabbitUser, rabbitPass, rabbitHost, rabbitPort, rabbitVHost)
		}
	}

	return &Config{
		GRPCAddress:                grpcAddress,
		ShardCount:                 shardCount,
		WALEnabled:                 walEnabled,
		WALPath:                    walPath,
		WALBufferSize:              walBufferSize,
		WALFlushInterval:           time.Duration(walFlushIntervalMs) * time.Millisecond,
		MaxConcurrentStreams:       maxConcurrentStreams,
		MaxReceiveMessageSizeBytes: maxReceiveBytes,
		PostgresDSN:                postgresDSN,
		RabbitMQEnabled:            rabbitEnabled,
		RabbitMQURL:                rabbitURL,
	}, nil
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
