// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v2/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	grpcIn "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/grpc/in"
	grpcOut "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/grpc/out"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	brokerpkg "github.com/LerianStudio/midaz/v3/pkg/broker"
	brokersecurity "github.com/LerianStudio/midaz/v3/pkg/broker/security"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/dbpool"
	"github.com/LerianStudio/midaz/v3/pkg/fence"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// ApplicationName is the name of the transaction application.
const ApplicationName = "transaction"

var (
	// ErrSchemaMigrationsEmpty is returned when schema_migrations has no rows.
	ErrSchemaMigrationsEmpty = errors.New("schema_migrations has no rows")
	// ErrConsumerModeConflict is returned when both CONSUMER_ENABLED and DEDICATED_CONSUMER_ENABLED are true.
	ErrConsumerModeConflict = errors.New("CONSUMER_ENABLED must be false when DEDICATED_CONSUMER_ENABLED=true")
	// ErrConsumerModeNotSet is returned when neither CONSUMER_ENABLED nor DEDICATED_CONSUMER_ENABLED is true.
	ErrConsumerModeNotSet = errors.New("invalid consumer mode: set either CONSUMER_ENABLED=true or DEDICATED_CONSUMER_ENABLED=true")
	// ErrSSLDisableNotAllowed is returned when SSL is disabled in a production-like environment.
	ErrSSLDisableNotAllowed = errors.New("SSL disable is not allowed in production-like environments")
	// ErrExternalPreSplitShardMismatch is returned when EXTERNAL_PRESPLIT_SHARD_COUNT diverges
	// from REDIS_SHARD_COUNT without explicit opt-in via ALLOW_EXTERNAL_PRESPLIT_MISMATCH.
	//
	// Rationale: pre-split ceiling controls how many Redis shards external-account
	// hot keys are fanned out across. When this is smaller than the live shard count,
	// large fan-out workloads (e.g. 10K-beneficiary payouts all debiting @external/USD)
	// collapse onto a small pre-split ceiling, producing write contention ~ batch/ceiling
	// per shard key. Forcing parity at bootstrap is the only fail-closed guard available —
	// once the pre-warm has run it is too late for operators to notice.
	ErrExternalPreSplitShardMismatch = errors.New("EXTERNAL_PRESPLIT_SHARD_COUNT must equal REDIS_SHARD_COUNT")
)

var dirtyMigrationVersionPattern = regexp.MustCompile(`(?i)dirty database version\s+(\d+)`)

// initLogger initializes the logger from options or creates a new one.
func initLogger(opts *Options) (libLog.Logger, error) {
	if opts != nil && opts.Logger != nil {
		return opts.Logger, nil
	}

	logger, err := libZap.InitializeLoggerWithError()
	if err != nil {
		return nil, fmt.Errorf("initialize logger: %w", err)
	}

	return logger, nil
}

func isNilStateChangeListener(listener libCircuitBreaker.StateChangeListener) bool {
	if listener == nil {
		return true
	}

	v := reflect.ValueOf(listener)

	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func resolveCircuitBreakerStateListener(
	opts *Options,
	metricStateListener libCircuitBreaker.StateChangeListener,
) libCircuitBreaker.StateChangeListener {
	if isNilStateChangeListener(metricStateListener) {
		if opts != nil && !isNilStateChangeListener(opts.CircuitBreakerStateListener) {
			return opts.CircuitBreakerStateListener
		}

		return nil
	}

	if opts != nil && !isNilStateChangeListener(opts.CircuitBreakerStateListener) {
		return &compositeStateListener{
			listeners: []libCircuitBreaker.StateChangeListener{
				metricStateListener,
				opts.CircuitBreakerStateListener,
			},
		}
	}

	return metricStateListener
}

func resolveBrokerOperationTimeout(rawTimeout string) time.Duration {
	operationTimeout := redpanda.DefaultOperationTimeout

	if rawTimeout != "" {
		if parsed, err := time.ParseDuration(rawTimeout); err == nil && parsed > 0 {
			operationTimeout = parsed
		}
	}

	return operationTimeout
}

func enforcePostgresSSLMode(envName, sslMode, envVar string) error {
	normalizedEnv := strings.TrimSpace(envName)
	if normalizedEnv != "" && brokersecurity.IsNonProductionEnvironment(normalizedEnv) {
		return nil
	}

	if !strings.EqualFold(strings.TrimSpace(sslMode), "disable") {
		return nil
	}

	return fmt.Errorf("%w: %s=disable", ErrSSLDisableNotAllowed, envVar)
}

// defaultPostgresSSLMode returns the implicit sslmode when DB_TRANSACTION_SSLMODE
// (and the standalone DB_SSLMODE fallback) are unset. Non-production environments
// (dev/local/test/staging/...) default to "disable" to preserve developer ergonomics
// against Docker-composed Postgres images that do not enable TLS by default.
// All other environments — including unrecognised env names — default to "require"
// so accidentally dropping DB_*_SSLMODE in production cannot silently downgrade
// the connection to plaintext.
func defaultPostgresSSLMode(envName string) string {
	normalizedEnv := strings.TrimSpace(envName)
	if normalizedEnv != "" && brokersecurity.IsNonProductionEnvironment(normalizedEnv) {
		return "disable"
	}

	return "require"
}

func shouldAutoRecoverDirtyMigration(envName string) bool {
	resolved := strings.TrimSpace(envName)
	if resolved == "" {
		return false
	}

	return brokersecurity.IsNonProductionEnvironment(resolved)
}

func parseDirtyMigrationVersion(err error) (int64, bool) {
	if err == nil {
		return 0, false
	}

	matches := dirtyMigrationVersionPattern.FindStringSubmatch(err.Error())
	if len(matches) != 2 { //nolint:mnd // regex match: full match + 1 capture group = 2
		return 0, false
	}

	version, parseErr := strconv.ParseInt(matches[1], 10, 64)
	if parseErr != nil {
		return 0, false
	}

	return version, true
}

func migrationRepairStatements(version int64) []string {
	const migrationVersionWithConcurrentIndex = 13

	switch version {
	case migrationVersionWithConcurrentIndex:
		return []string{"DROP INDEX CONCURRENTLY IF EXISTS idx_operation_account"}
	default:
		return nil
	}
}

func readMigrationState(ctx context.Context, db *sql.DB) (int64, bool, error) {
	row := db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1")

	var (
		version int64
		dirty   bool
	)

	if err := row.Scan(&version, &dirty); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, ErrSchemaMigrationsEmpty
		}

		return 0, false, fmt.Errorf("failed to read schema_migrations: %w", err)
	}

	return version, dirty, nil
}

func recoverDirtyMigration(connectionString string, expectedVersion int64, logger libLog.Logger) error {
	const migrationRecoveryTimeoutSecs = 60

	ctx, cancel := context.WithTimeout(context.Background(), migrationRecoveryTimeoutSecs*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		return fmt.Errorf("failed to open PostgreSQL connection for migration recovery: %w",
			dbpool.ScrubDSNInError(err, connectionString))
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			logger.Warnf("Failed to close migration recovery connection: %v", closeErr)
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL for migration recovery: %w",
			dbpool.ScrubDSNInError(err, connectionString))
	}

	version, dirty, err := readMigrationState(ctx, db)
	if err != nil {
		return err
	}

	if !dirty {
		logger.Info("PostgreSQL migration state already clean; skipping dirty migration recovery")
		return nil
	}

	if version != expectedVersion {
		logger.Warnf("Dirty migration version changed during recovery attempt (detected=%d current=%d)", expectedVersion, version)
	}

	for _, statement := range migrationRepairStatements(version) {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("failed to execute migration recovery statement %q: %w", statement, err)
		}
	}

	targetVersion := version
	if targetVersion > 0 {
		targetVersion = version - 1
	}

	result, err := db.ExecContext(ctx,
		"UPDATE schema_migrations SET version = $1, dirty = FALSE WHERE version = $2 AND dirty = TRUE",
		targetVersion,
		version,
	)
	if err != nil {
		return fmt.Errorf("failed to force schema_migrations to version %d: %w", targetVersion, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify schema_migrations recovery update: %w", err)
	}

	if rowsAffected == 0 {
		logger.Warnf("schema_migrations was updated concurrently while recovering dirty version %d; continuing", version)

		return nil
	}

	logger.Warnf("Recovered dirty PostgreSQL migration state from version %d to version %d", version, targetVersion)

	return nil
}

func ensurePostgresConnectionReady(cfg *Config, connection *libPostgres.PostgresConnection, logger libLog.Logger) error {
	_, err := connection.GetDB()
	if err == nil {
		return nil
	}

	version, dirty := parseDirtyMigrationVersion(err)
	if !dirty {
		return fmt.Errorf("failed to initialize transaction PostgreSQL connection: %w", err)
	}

	if !shouldAutoRecoverDirtyMigration(cfg.EnvName) {
		return fmt.Errorf("dirty PostgreSQL migration detected at version %d in environment %q: %w", version, cfg.EnvName, err)
	}

	logger.Warnf("Dirty PostgreSQL migration detected at version %d. Attempting automatic recovery in environment %q", version, cfg.EnvName)

	if recoveryErr := recoverDirtyMigration(connection.ConnectionStringPrimary, version, logger); recoveryErr != nil {
		// pgx historically echoes the DSN in open/ping error messages. Scrub
		// the password before wrapping so the error chain never carries the
		// plaintext credential into logs, traces, or crash dumps.
		scrubbed := dbpool.ScrubDSNInError(recoveryErr, connection.ConnectionStringPrimary)

		return fmt.Errorf("failed to recover dirty PostgreSQL migration version %d: %w", version, scrubbed)
	}

	if _, retryErr := connection.GetDB(); retryErr != nil {
		return fmt.Errorf("failed to initialize transaction PostgreSQL connection after dirty migration recovery: %w", retryErr)
	}

	logger.Infof("PostgreSQL dirty migration recovery succeeded for version %d", version)

	return nil
}

func newBalanceSyncWorker(
	cfg *Config,
	logger libLog.Logger,
	redisConnection *libRedis.RedisConnection,
	useCase *command.UseCase,
	balanceSyncWorkerEnabled bool,
) *BalanceSyncWorker {
	const defaultBalanceSyncMaxWorkers = 5

	balanceSyncMaxWorkers := cfg.BalanceSyncMaxWorkers
	if balanceSyncMaxWorkers <= 0 {
		balanceSyncMaxWorkers = defaultBalanceSyncMaxWorkers
		logger.Infof("BalanceSyncWorker using default: BALANCE_SYNC_MAX_WORKERS=%d", defaultBalanceSyncMaxWorkers)
	}

	if balanceSyncWorkerEnabled {
		balanceSyncWorker := NewBalanceSyncWorker(redisConnection, logger, useCase, balanceSyncMaxWorkers)
		logger.Infof("BalanceSyncWorker enabled with %d max workers.", balanceSyncMaxWorkers)

		return balanceSyncWorker
	}

	logger.Info("BalanceSyncWorker disabled.")

	return nil
}

// initShardRouting initializes the shard router and manager for Redis Cluster sharding (Phase 2A).
// Returns (nil, nil) when sharding is disabled (REDIS_SHARD_COUNT=0).
func initShardRouting(
	cfg *Config,
	logger libLog.Logger,
	redisConnection *libRedis.RedisConnection,
) (*shard.Router, *internalsharding.Manager) {
	if cfg.RedisShardCount <= 0 {
		logger.Info("Redis sharding disabled (REDIS_SHARD_COUNT=0)")

		return nil, nil
	}

	shardRouter := shard.NewRouter(cfg.RedisShardCount)
	logger.Infof("Redis sharding enabled: %d shards", cfg.RedisShardCount)

	shardManager := internalsharding.NewManager(redisConnection, shardRouter, logger, internalsharding.Config{})

	return shardRouter, shardManager
}

// brokerProducerResult holds the outputs of producer + circuit breaker initialization.
type brokerProducerResult struct {
	producer              redpanda.ProducerRepository
	circuitBreakerManager *CircuitBreakerManager
}

// initProducerWithCircuitBreaker creates the producer and wraps it with a circuit breaker.
func initProducerWithCircuitBreaker(
	cfg *Config,
	opts *Options,
	logger libLog.Logger,
	brokers []string,
	telemetry *libOpentelemetry.Telemetry,
) (*brokerProducerResult, error) {
	linger := time.Duration(cfg.RedpandaProducerLingerMS) * time.Millisecond
	securityConfig := redpanda.ClientSecurityConfig{
		TLSEnabled:            cfg.RedpandaTLSEnabled,
		TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
		TLSCAFile:             cfg.RedpandaTLSCAFile,
		SASLEnabled:           cfg.RedpandaSASLEnabled,
		SASLMechanism:         cfg.RedpandaSASLMechanism,
		SASLUsername:          cfg.RedpandaSASLUsername,
		SASLPassword:          cfg.RedpandaSASLPassword,
		Environment:           cfg.EnvName,
	}

	rawProducer, err := redpanda.NewProducerRedpandaWithSecurityAndShardPartitioning(
		brokers,
		linger,
		cfg.RedpandaMaxBufferedRecords,
		cfg.TransactionAsync,
		securityConfig,
		cfg.RedisShardCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redpanda producer: %w", err)
	}

	var metricStateListener libCircuitBreaker.StateChangeListener

	switch {
	case telemetry != nil && telemetry.MetricsFactory != nil:
		metricStateListener, err = redpanda.NewMetricStateListener(telemetry.MetricsFactory)
		if err != nil {
			if closeErr := rawProducer.Close(); closeErr != nil {
				logger.Warnf("Failed to close producer during cleanup: %v", closeErr)
			}

			return nil, fmt.Errorf("failed to create metric state listener: %w", err)
		}
	default:
		logger.Warn("Telemetry metrics factory unavailable; circuit breaker metrics listener disabled")
	}

	stateListener := resolveCircuitBreakerStateListener(opts, metricStateListener)
	operationTimeout := resolveBrokerOperationTimeout(cfg.BrokerOperationTimeout)

	//nolint:mnd // circuit breaker defaults: well-known tuning constants
	cbConfig := redpanda.CircuitBreakerConfig{
		ConsecutiveFailures: utils.GetUint32FromIntWithDefault(cfg.BrokerCircuitBreakerConsecutiveFailures, 15),
		FailureRatio:        utils.GetFloat64FromIntPercentWithDefault(cfg.BrokerCircuitBreakerFailureRatio, 0.5),
		Interval:            utils.GetDurationSecondsWithDefault(cfg.BrokerCircuitBreakerInterval, 2*time.Minute),
		MaxRequests:         utils.GetUint32FromIntWithDefault(cfg.BrokerCircuitBreakerMaxRequests, 3),
		MinRequests:         utils.GetUint32FromIntWithDefault(cfg.BrokerCircuitBreakerMinRequests, 10),
		Timeout:             utils.GetDurationSecondsWithDefault(cfg.BrokerCircuitBreakerTimeout, 30*time.Second),
		HealthCheckInterval: utils.GetDurationSecondsWithDefault(cfg.BrokerCircuitBreakerHealthCheckInterval, 30*time.Second),
		OperationTimeout:    operationTimeout,
	}

	circuitBreakerManager, err := NewCircuitBreakerManager(logger, cbConfig, stateListener)
	if err != nil {
		if closeErr := rawProducer.Close(); closeErr != nil {
			logger.Warnf("Failed to close producer during cleanup: %v", closeErr)
		}

		return nil, fmt.Errorf("failed to create circuit breaker manager: %w", err)
	}

	circuitBreakerManager.SetHealthChecker(rawProducer)

	producerRepo, err := redpanda.NewCircuitBreakerProducer(
		rawProducer,
		circuitBreakerManager.Manager,
		logger,
		cbConfig.OperationTimeout,
	)
	if err != nil {
		if closeErr := rawProducer.Close(); closeErr != nil {
			logger.Warnf("Failed to close producer during cleanup: %v", closeErr)
		}

		return nil, fmt.Errorf("failed to create circuit breaker producer: %w", err)
	}

	return &brokerProducerResult{
		producer:              producerRepo,
		circuitBreakerManager: circuitBreakerManager,
	}, nil
}

func initConsumerLagChecker(
	cfg *Config,
	logger libLog.Logger,
	brokers []string,
) (fence.ConsumerLagChecker, func(), error) {
	if cfg == nil || !cfg.ConsumerLagFenceEnabled {
		return nil, nil, nil
	}

	securityConfig := redpanda.ClientSecurityConfig{
		TLSEnabled:            cfg.RedpandaTLSEnabled,
		TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
		TLSCAFile:             cfg.RedpandaTLSCAFile,
		SASLEnabled:           cfg.RedpandaSASLEnabled,
		SASLMechanism:         cfg.RedpandaSASLMechanism,
		SASLUsername:          cfg.RedpandaSASLUsername,
		SASLPassword:          cfg.RedpandaSASLPassword,
		Environment:           cfg.EnvName,
	}

	securityOptions, err := redpanda.BuildSecurityOptions(securityConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to configure consumer lag checker security: %w", err)
	}

	clientOpts := make([]kgo.Opt, 0, 1+len(securityOptions))
	clientOpts = append(clientOpts, kgo.SeedBrokers(brokers...))
	clientOpts = append(clientOpts, securityOptions...)

	client, err := kgo.NewClient(clientOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize consumer lag checker client: %w", err)
	}

	cacheTTL := time.Duration(cfg.ConsumerLagCacheTTLMS) * time.Millisecond
	checker := fence.NewFranzConsumerLagCheckerWithMode(
		client,
		cfg.RedpandaConsumerGroup,
		cacheTTL,
		cfg.ConsumerLagFenceFailOpen,
	)

	logger.Infof(
		"Consumer lag fence enabled group=%s cache_ttl=%s fail_open=%t",
		cfg.RedpandaConsumerGroup,
		cacheTTL,
		cfg.ConsumerLagFenceFailOpen,
	)

	cleanup := func() {
		client.Close()
	}

	return checker, cleanup, nil
}

func initStaleBalanceRecoverer(
	cfg *Config,
	logger libLog.Logger,
	brokers []string,
) (query.StaleBalanceRecoverer, func(), error) {
	if cfg == nil || !cfg.ConsumerLagFenceEnabled {
		return nil, nil, nil
	}

	securityConfig := redpanda.ClientSecurityConfig{
		TLSEnabled:            cfg.RedpandaTLSEnabled,
		TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
		TLSCAFile:             cfg.RedpandaTLSCAFile,
		SASLEnabled:           cfg.RedpandaSASLEnabled,
		SASLMechanism:         cfg.RedpandaSASLMechanism,
		SASLUsername:          cfg.RedpandaSASLUsername,
		SASLPassword:          cfg.RedpandaSASLPassword,
		Environment:           cfg.EnvName,
	}

	securityOptions, err := redpanda.BuildSecurityOptions(securityConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to configure stale-balance recoverer security: %w", err)
	}

	adminOptions := make([]kgo.Opt, 0, 1+len(securityOptions))
	adminOptions = append(adminOptions, kgo.SeedBrokers(brokers...))
	adminOptions = append(adminOptions, securityOptions...)

	adminClient, err := kgo.NewClient(adminOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize stale-balance recoverer admin client: %w", err)
	}

	recoverer := query.NewFranzStaleBalanceRecoverer(adminClient, brokers, securityOptions, cfg.RedpandaConsumerGroup)

	logger.Infof("Stale-balance replay recoverer enabled group=%s", cfg.RedpandaConsumerGroup)

	cleanup := func() {
		adminClient.Close()
	}

	return recoverer, cleanup, nil
}

func newShardRebalanceWorker(
	cfg *Config,
	logger libLog.Logger,
	shardManager *internalsharding.Manager,
	shardRouter *shard.Router,
	enabled bool,
) *ShardRebalanceWorker {
	if !enabled {
		logger.Info("ShardRebalanceWorker disabled.")

		return nil
	}

	if shardManager == nil || shardRouter == nil {
		logger.Info("ShardRebalanceWorker disabled: sharding manager/router unavailable")

		return nil
	}

	const percentDivisor = 100.0

	interval := time.Duration(cfg.ShardRebalanceIntervalSeconds) * time.Second
	window := time.Duration(cfg.ShardRebalanceWindowSeconds) * time.Second
	threshold := float64(cfg.ShardRebalanceThresholdPercent) / percentDivisor
	candidateLimit := cfg.ShardRebalanceCandidateLimit
	isolationShare := float64(cfg.ShardRebalanceIsolationSharePercent) / percentDivisor
	isolationMinLoad := cfg.ShardRebalanceIsolationMinLoad

	worker := NewShardRebalanceWorker(
		logger,
		shardManager,
		shardRouter,
		interval,
		window,
		threshold,
		candidateLimit,
		isolationShare,
		isolationMinLoad,
	)
	logger.Infof(
		"ShardRebalanceWorker enabled interval=%s window=%s threshold=%.2f candidate_limit=%d isolation_share=%.2f isolation_min_load=%d",
		interval,
		window,
		threshold,
		candidateLimit,
		isolationShare,
		isolationMinLoad,
	)

	return worker
}

//nolint:gocyclo,cyclop // pre-warm setup with multiple validation steps and error paths
func preWarmExternalPreSplitBalances(
	cfg *Config,
	logger libLog.Logger,
	balanceRepo *balance.BalancePostgreSQLRepository,
	redisRepo *redis.RedisConsumerRepository,
) error {
	if cfg == nil || !cfg.ExternalPreSplitPreWarm || balanceRepo == nil || redisRepo == nil {
		return nil
	}

	if cfg.ExternalPreSplitOrganizationID == "" || cfg.ExternalPreSplitLedgerID == "" {
		logger.Info("External pre-split Redis pre-warm skipped: EXTERNAL_PRESPLIT_ORGANIZATION_ID or EXTERNAL_PRESPLIT_LEDGER_ID not set")
		return nil
	}

	organizationID, err := uuid.Parse(cfg.ExternalPreSplitOrganizationID)
	if err != nil {
		return fmt.Errorf("invalid EXTERNAL_PRESPLIT_ORGANIZATION_ID: %w", err)
	}

	ledgerID, err := uuid.Parse(cfg.ExternalPreSplitLedgerID)
	if err != nil {
		return fmt.Errorf("invalid EXTERNAL_PRESPLIT_LEDGER_ID: %w", err)
	}

	const preWarmTimeoutSecs = 10

	ctx, cancel := context.WithTimeout(context.Background(), preWarmTimeoutSecs*time.Second)
	defer cancel()

	externalBalances, err := balanceRepo.ListExternalByOrganizationLedger(ctx, organizationID, ledgerID)
	if err != nil {
		return fmt.Errorf("failed to list external balances: %w", err)
	}

	if len(externalBalances) == 0 {
		logger.Infof("External pre-split Redis pre-warm: no external balances found for organization=%s ledger=%s", organizationID, ledgerID)
		return nil
	}

	preWarmTTL := time.Duration(cfg.ExternalPreSplitPreWarmTTLSeconds) * time.Second
	if preWarmTTL < 0 {
		preWarmTTL = 0
	}

	coverageByAlias, err := redisRepo.PreWarmExternalBalances(ctx, organizationID, ledgerID, externalBalances, preWarmTTL)
	if err != nil {
		return fmt.Errorf("failed to pre-warm external balances in Redis: %w", err)
	}

	for alias, coverage := range coverageByAlias {
		expected := coverage.ExpectedShards
		if cfg.ExternalPreSplitShardCount > 0 {
			expected = cfg.ExternalPreSplitShardCount
		}

		logger.Infof("External pre-split balances: %d/%d shards covered for %s", coverage.CoveredShards, expected, alias)

		if coverage.CoveredShards < expected {
			logger.Warnf("External pre-split coverage incomplete for %s: %d/%d shard balances", alias, coverage.CoveredShards, expected)
		}
	}

	return nil
}

// initPostgresConnection creates the PostgreSQL connection with fallback support
// for prefixed env vars (unified ledger) and SSL mode enforcement.
func initPostgresConnection(cfg *Config, logger libLog.Logger) (*libPostgres.PostgresConnection, error) {
	// Apply fallback for prefixed env vars (unified ledger) to non-prefixed (standalone)
	dbHost := utils.EnvFallback(cfg.PrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := utils.EnvFallback(cfg.PrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := utils.EnvFallback(cfg.PrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := utils.EnvFallback(cfg.PrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := utils.EnvFallback(cfg.PrefixedPrimaryDBPort, cfg.PrimaryDBPort)

	dbSSLMode := strings.TrimSpace(utils.EnvFallback(cfg.PrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode))
	if dbSSLMode == "" {
		dbSSLMode = defaultPostgresSSLMode(cfg.EnvName)
	}

	if err := enforcePostgresSSLMode(cfg.EnvName, dbSSLMode, "DB_TRANSACTION_SSLMODE"); err != nil {
		return nil, err
	}

	dbReplicaHost := utils.EnvFallback(cfg.PrefixedReplicaDBHost, cfg.ReplicaDBHost)
	dbReplicaUser := utils.EnvFallback(cfg.PrefixedReplicaDBUser, cfg.ReplicaDBUser)
	dbReplicaPassword := utils.EnvFallback(cfg.PrefixedReplicaDBPassword, cfg.ReplicaDBPassword)
	dbReplicaName := utils.EnvFallback(cfg.PrefixedReplicaDBName, cfg.ReplicaDBName)
	dbReplicaPort := utils.EnvFallback(cfg.PrefixedReplicaDBPort, cfg.ReplicaDBPort)

	dbReplicaSSLMode := strings.TrimSpace(utils.EnvFallback(cfg.PrefixedReplicaDBSSLMode, cfg.ReplicaDBSSLMode))
	if dbReplicaSSLMode == "" {
		dbReplicaSSLMode = dbSSLMode
	}

	if err := enforcePostgresSSLMode(cfg.EnvName, dbReplicaSSLMode, "DB_TRANSACTION_REPLICA_SSLMODE"); err != nil {
		return nil, err
	}

	maxOpenConns := utils.EnvFallbackInt(cfg.PrefixedMaxOpenConnections, cfg.MaxOpenConnections)
	maxIdleConns := utils.EnvFallbackInt(cfg.PrefixedMaxIdleConnections, cfg.MaxIdleConnections)

	const percentDivisor = 100.0

	if err := dbpool.ValidatePoolBudget(
		cfg.DBPoolBudgetMaxConnections,
		float64(cfg.DBPoolBudgetRatioPercent)/percentDivisor,
		[]dbpool.PoolBudget{{
			Name:              "transaction",
			MaxConns:          maxOpenConns,
			ExpectedInstances: cfg.DBPoolBudgetInstances,
		}},
	); err != nil {
		return nil, err
	}

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbReplicaHost, dbReplicaUser, dbReplicaPassword, dbReplicaName, dbReplicaPort, dbReplicaSSLMode)

	return &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           dbName,
		ReplicaDBName:           dbReplicaName,
		Component:               ApplicationName,
		Logger:                  logger,
		MaxOpenConnections:      maxOpenConns,
		MaxIdleConnections:      maxIdleConns,
	}, nil
}

// initMongoConnection creates the MongoDB connection with fallback support
// for prefixed env vars (unified ledger deployment).
func initMongoConnection(cfg *Config, logger libLog.Logger) *libMongo.MongoConnection {
	// Apply fallback for MongoDB prefixed env vars
	mongoURI := utils.EnvFallback(cfg.PrefixedMongoURI, cfg.MongoURI)
	mongoHost := utils.EnvFallback(cfg.PrefixedMongoDBHost, cfg.MongoDBHost)
	mongoName := utils.EnvFallback(cfg.PrefixedMongoDBName, cfg.MongoDBName)
	mongoUser := utils.EnvFallback(cfg.PrefixedMongoDBUser, cfg.MongoDBUser)
	mongoPassword := utils.EnvFallback(cfg.PrefixedMongoDBPassword, cfg.MongoDBPassword)
	mongoPortRaw := utils.EnvFallback(cfg.PrefixedMongoDBPort, cfg.MongoDBPort)
	mongoParametersRaw := utils.EnvFallback(cfg.PrefixedMongoDBParameters, cfg.MongoDBParameters)
	mongoPoolSize := utils.EnvFallbackInt(cfg.PrefixedMaxPoolSize, cfg.MaxPoolSize)

	// Extract port and parameters for MongoDB connection (handles backward compatibility)
	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(mongoPortRaw, mongoParametersRaw, logger)

	// Build MongoDB connection string using centralized utility (ensures correct format)
	mongoSource := libMongo.BuildConnectionString(
		mongoURI, mongoUser, mongoPassword, mongoHost, mongoPort, mongoParameters, logger)

	// Safe conversion: use uint64 with default, only assign if positive
	var mongoMaxPoolSize uint64 = 100
	if mongoPoolSize > 0 {
		mongoMaxPoolSize = uint64(mongoPoolSize)
	}

	return &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               mongoName,
		Logger:                 logger,
		MaxPoolSize:            mongoMaxPoolSize,
	}
}

// initRedisConnection creates the Redis connection from configuration.
func initRedisConnection(cfg *Config, logger libLog.Logger) *libRedis.RedisConnection {
	return &libRedis.RedisConnection{
		Address:                      strings.Split(cfg.RedisHost, ","),
		Password:                     cfg.RedisPassword,
		DB:                           cfg.RedisDB,
		Protocol:                     cfg.RedisProtocol,
		MasterName:                   cfg.RedisMasterName,
		UseTLS:                       cfg.RedisTLS,
		CACert:                       cfg.RedisCACert,
		UseGCPIAMAuth:                cfg.RedisUseGCPIAM,
		ServiceAccount:               cfg.RedisServiceAccount,
		GoogleApplicationCredentials: cfg.GoogleApplicationCredentials,
		TokenLifeTime:                time.Duration(cfg.RedisTokenLifeTime) * time.Minute,
		RefreshDuration:              time.Duration(cfg.RedisTokenRefreshDuration) * time.Minute,
		Logger:                       logger,
		PoolSize:                     cfg.RedisPoolSize,
		MinIdleConns:                 cfg.RedisMinIdleConns,
		ReadTimeout:                  time.Duration(cfg.RedisReadTimeout) * time.Second,
		WriteTimeout:                 time.Duration(cfg.RedisWriteTimeout) * time.Second,
		DialTimeout:                  time.Duration(cfg.RedisDialTimeout) * time.Second,
		PoolTimeout:                  time.Duration(cfg.RedisPoolTimeout) * time.Second,
		MaxRetries:                   cfg.RedisMaxRetries,
		MinRetryBackoff:              time.Duration(cfg.RedisMinRetryBackoff) * time.Millisecond,
		MaxRetryBackoff:              time.Duration(cfg.RedisMaxRetryBackoff) * time.Second,
	}
}

// ensureMongoIndexes creates entity_id indexes on known base collections.
func ensureMongoIndexes(mongoConnection *libMongo.MongoConnection, logger libLog.Logger) {
	const ensureIndexesTimeoutSecs = 60

	ctxEnsureIndexes, cancelEnsureIndexes := context.WithTimeout(context.Background(), ensureIndexesTimeoutSecs*time.Second)
	defer cancelEnsureIndexes()

	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}},
		Options: options.Index().
			SetUnique(false),
	}

	collections := []string{"operation", "transaction", "operation_route", "transaction_route"}
	for _, collection := range collections {
		if err := mongoConnection.EnsureIndexes(ctxEnsureIndexes, collection, indexModel); err != nil {
			logger.Warnf("Failed to ensure indexes for collection %s: %v", collection, err)
		}
	}
}

// validateBrokerSecurity validates Redpanda security configuration and logs warnings.
func validateBrokerSecurity(cfg *Config, logger libLog.Logger) error {
	warnings, err := brokersecurity.ValidateRuntimeConfig(brokersecurity.RuntimeConfig{
		Environment:           cfg.EnvName,
		TLSEnabled:            cfg.RedpandaTLSEnabled,
		TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
		SASLEnabled:           cfg.RedpandaSASLEnabled,
	})
	if err != nil {
		return err
	}

	for _, warning := range warnings {
		logger.Warnf("Redpanda security warning: %s (ENV_NAME=%s)", warning, cfg.EnvName)
	}

	deprecatedBrokerEnvs := brokerpkg.DeprecatedBrokerEnvVariables(os.Environ())
	if len(deprecatedBrokerEnvs) > 0 {
		logger.Warnf(
			"Deprecated broker environment variables detected (ignored by this version): %s. Regenerate .env from .env.example and remove deprecated entries.",
			strings.Join(deprecatedBrokerEnvs, ", "),
		)
	}

	return nil
}

// brokerInfraResult holds the outputs of broker infrastructure initialization.
type brokerInfraResult struct {
	producer              redpanda.ProducerRepository
	circuitBreakerManager *CircuitBreakerManager
	consumerLagChecker    fence.ConsumerLagChecker
	staleBalanceRecoverer query.StaleBalanceRecoverer
	cleanupFuncs          []func()
}

// initBrokerInfrastructure initializes the Redpanda producer (with circuit breaker),
// consumer lag checker, and stale-balance recoverer. It returns all cleanup functions
// that the caller must invoke on shutdown.
func initBrokerInfrastructure(
	cfg *Config,
	opts *Options,
	logger libLog.Logger,
	seedBrokers []string,
	telemetry *libOpentelemetry.Telemetry,
) (*brokerInfraResult, error) {
	var cleanups []func()

	producerResult, err := initProducerWithCircuitBreaker(cfg, opts, logger, seedBrokers, telemetry)
	if err != nil {
		return nil, err
	}

	cleanups = append(cleanups, func() {
		if producerResult.circuitBreakerManager != nil {
			producerResult.circuitBreakerManager.Stop()
		}

		if producerResult.producer != nil {
			if closeErr := producerResult.producer.Close(); closeErr != nil {
				logger.Warnf("Failed to close transaction producer during cleanup: %v", closeErr)
			}
		}
	})

	consumerLagChecker, consumerLagCheckerCleanup, err := initConsumerLagChecker(cfg, logger, seedBrokers)
	if err != nil {
		return nil, err
	}

	if consumerLagCheckerCleanup != nil {
		cleanups = append(cleanups, consumerLagCheckerCleanup)
	}

	staleBalanceRecoverer, staleBalanceRecovererCleanup, err := initStaleBalanceRecoverer(cfg, logger, seedBrokers)
	if err != nil {
		return nil, err
	}

	if staleBalanceRecovererCleanup != nil {
		cleanups = append(cleanups, staleBalanceRecovererCleanup)
	}

	return &brokerInfraResult{
		producer:              producerResult.producer,
		circuitBreakerManager: producerResult.circuitBreakerManager,
		consumerLagChecker:    consumerLagChecker,
		staleBalanceRecoverer: staleBalanceRecoverer,
		cleanupFuncs:          cleanups,
	}, nil
}

// resolveProtoAddress ensures ProtoAddress has a valid value, defaulting to ":3011".
func resolveProtoAddress(cfg *Config, logger libLog.Logger) {
	if cfg.ProtoAddress == "" || cfg.ProtoAddress == ":" {
		cfg.ProtoAddress = ":3011"

		logger.Warn("PROTO_ADDRESS not set or invalid, using default: :3011")
	}
}

// resolveBalanceCacheTTL returns a non-negative cache TTL from configuration.
func resolveBalanceCacheTTL(cfg *Config) time.Duration {
	ttl := time.Duration(cfg.BalanceCacheTTLSeconds) * time.Second
	if ttl < 0 {
		return 0
	}

	return ttl
}

// configureConsumerRoutes creates and configures Redpanda consumer routes.
func configureConsumerRoutes(cfg *Config, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry, seedBrokers []string) *redpanda.ConsumerRoutes {
	routes := redpanda.NewConsumerRoutesWithSecurity(
		seedBrokers,
		cfg.RedpandaConsumerGroup,
		cfg.RedpandaNumbersOfWorkers,
		cfg.RedpandaFetchMaxBytes,
		logger,
		telemetry,
		redpanda.ClientSecurityConfig{
			TLSEnabled:            cfg.RedpandaTLSEnabled,
			TLSInsecureSkipVerify: cfg.RedpandaTLSInsecureSkipVerify,
			TLSCAFile:             cfg.RedpandaTLSCAFile,
			SASLEnabled:           cfg.RedpandaSASLEnabled,
			SASLMechanism:         cfg.RedpandaSASLMechanism,
			SASLUsername:          cfg.RedpandaSASLUsername,
			SASLPassword:          cfg.RedpandaSASLPassword,
			Environment:           cfg.EnvName,
		},
		cfg.RedpandaMaxRetryAttempts,
	)

	if cfg.RedpandaPartitionCount > 0 && cfg.RedpandaNumbersOfWorkers > cfg.RedpandaPartitionCount {
		logger.Warnf(
			"REDPANDA_NUMBERS_OF_WORKERS=%d exceeds REDPANDA_PARTITION_COUNT=%d; extra workers stay idle because work is partition-bound",
			cfg.RedpandaNumbersOfWorkers,
			cfg.RedpandaPartitionCount,
		)
	}

	routes.SetPartitionWorkerHint(cfg.RedpandaPartitionCount)

	commitInterval := time.Duration(cfg.RedpandaCommitIntervalMS) * time.Millisecond
	if commitInterval > 0 {
		routes.SetCommitInterval(commitInterval)
	}

	if cfg.ConsumerBackpressureEnabled && cfg.ConsumerMaxDBTPS > 0 {
		routes.SetDBRateLimiter(cfg.ConsumerMaxDBTPS, cfg.ConsumerMaxDBBurst)
		logger.Infof("Consumer DB backpressure enabled: max_tps=%d burst=%d", cfg.ConsumerMaxDBTPS, cfg.ConsumerMaxDBBurst)
	}

	routes.SetBatchConfig(
		cfg.ConsumerBatchEnabled,
		cfg.ConsumerBatchSize,
		time.Duration(cfg.ConsumerBatchWindowMS)*time.Millisecond,
		time.Duration(cfg.ConsumerIdleFlushMS)*time.Millisecond,
	)
	routes.SetBatchImmediateCommit(cfg.ConsumerBatchImmediateCommit)

	if cfg.ConsumerBatchEnabled {
		logger.Infof(
			"Consumer micro-batching enabled: size=%d window_ms=%d idle_flush_ms=%d immediate_commit=%t",
			cfg.ConsumerBatchSize,
			cfg.ConsumerBatchWindowMS,
			cfg.ConsumerIdleFlushMS,
			cfg.ConsumerBatchImmediateCommit,
		)
	}

	return routes
}

// Config is the top level configuration struct for the entire application.
// Supports prefixed env vars (DB_TRANSACTION_*) with fallback to non-prefixed (DB_*) for backward compatibility.
type Config struct {
	EnvName  string `env:"ENV_NAME"`
	LogLevel string `env:"LOG_LEVEL"`
	Version  string `env:"VERSION" default:"v3"`

	// Server address - prefixed for unified ledger deployment
	PrefixedServerAddress string `env:"SERVER_ADDRESS_TRANSACTION"`
	ServerAddress         string `env:"SERVER_ADDRESS"`

	// PostgreSQL Primary - prefixed vars for unified ledger deployment
	PrefixedPrimaryDBHost     string `env:"DB_TRANSACTION_HOST"`
	PrefixedPrimaryDBUser     string `env:"DB_TRANSACTION_USER"`
	PrefixedPrimaryDBPassword string `env:"DB_TRANSACTION_PASSWORD"`
	PrefixedPrimaryDBName     string `env:"DB_TRANSACTION_NAME"`
	PrefixedPrimaryDBPort     string `env:"DB_TRANSACTION_PORT"`
	PrefixedPrimaryDBSSLMode  string `env:"DB_TRANSACTION_SSLMODE"`

	// PostgreSQL Primary - fallback vars for standalone deployment
	PrimaryDBHost     string `env:"DB_HOST"`
	PrimaryDBUser     string `env:"DB_USER"`
	PrimaryDBPassword string `env:"DB_PASSWORD"`
	PrimaryDBName     string `env:"DB_NAME"`
	PrimaryDBPort     string `env:"DB_PORT"`
	PrimaryDBSSLMode  string `env:"DB_SSLMODE"`

	// PostgreSQL Replica - prefixed vars for unified ledger deployment
	PrefixedReplicaDBHost     string `env:"DB_TRANSACTION_REPLICA_HOST"`
	PrefixedReplicaDBUser     string `env:"DB_TRANSACTION_REPLICA_USER"`
	PrefixedReplicaDBPassword string `env:"DB_TRANSACTION_REPLICA_PASSWORD"`
	PrefixedReplicaDBName     string `env:"DB_TRANSACTION_REPLICA_NAME"`
	PrefixedReplicaDBPort     string `env:"DB_TRANSACTION_REPLICA_PORT"`
	PrefixedReplicaDBSSLMode  string `env:"DB_TRANSACTION_REPLICA_SSLMODE"`

	// PostgreSQL Replica - fallback vars for standalone deployment
	ReplicaDBHost     string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser     string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName     string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort     string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode  string `env:"DB_REPLICA_SSLMODE"`

	// PostgreSQL connection pool - prefixed with fallback
	PrefixedMaxOpenConnections int `env:"DB_TRANSACTION_MAX_OPEN_CONNS"`
	PrefixedMaxIdleConnections int `env:"DB_TRANSACTION_MAX_IDLE_CONNS"`
	MaxOpenConnections         int `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections         int `env:"DB_MAX_IDLE_CONNS"`

	// Pool budget validation (D7). These bound the aggregate
	// MaxOpenConnections * DBPoolBudgetInstances against the declared
	// PostgreSQL max_connections ceiling so no single deploy can exhaust
	// the server. Operators MUST set DB_POOL_BUDGET_MAX_CONNECTIONS in
	// production to match the `max_connections` on the PG cluster; the
	// default of 0 disables fail-closed validation for local development.
	// Ratio is expressed as an integer percent (0..100) because
	// lib-commons/v2 SetConfigFromEnvVars does not support float64
	// fields — it reflect.Value.SetString-panics on them. Convert at
	// use-site to a float.
	DBPoolBudgetMaxConnections int `env:"DB_POOL_BUDGET_MAX_CONNECTIONS" default:"0"`
	DBPoolBudgetInstances      int `env:"DB_POOL_BUDGET_INSTANCES" default:"1"`
	DBPoolBudgetRatioPercent   int `env:"DB_POOL_BUDGET_RATIO_PERCENT" default:"80"`

	// MongoDB - prefixed vars for unified ledger deployment
	PrefixedMongoURI          string `env:"MONGO_TRANSACTION_URI"`
	PrefixedMongoDBHost       string `env:"MONGO_TRANSACTION_HOST"`
	PrefixedMongoDBName       string `env:"MONGO_TRANSACTION_NAME"`
	PrefixedMongoDBUser       string `env:"MONGO_TRANSACTION_USER"`
	PrefixedMongoDBPassword   string `env:"MONGO_TRANSACTION_PASSWORD"`
	PrefixedMongoDBPort       string `env:"MONGO_TRANSACTION_PORT"`
	PrefixedMongoDBParameters string `env:"MONGO_TRANSACTION_PARAMETERS"`
	PrefixedMaxPoolSize       int    `env:"MONGO_TRANSACTION_MAX_POOL_SIZE"`

	// MongoDB - fallback vars for standalone deployment
	MongoURI                       string `env:"MONGO_URI"`
	MongoDBHost                    string `env:"MONGO_HOST"`
	MongoDBName                    string `env:"MONGO_NAME"`
	MongoDBUser                    string `env:"MONGO_USER"`
	MongoDBPassword                string `env:"MONGO_PASSWORD"`
	MongoDBPort                    string `env:"MONGO_PORT"`
	MongoDBParameters              string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                    int    `env:"MONGO_MAX_POOL_SIZE"`
	RedpandaBrokers                string `env:"REDPANDA_BROKERS" default:"127.0.0.1:9092"`
	RedpandaBalanceCreateTopic     string `env:"REDPANDA_BALANCE_CREATE_TOPIC" default:"ledger.balance.create"`
	RedpandaBalanceOperationsTopic string `env:"REDPANDA_BALANCE_OPS_TOPIC" default:"ledger.balance.operations"`
	RedpandaEventsTopic            string `env:"REDPANDA_EVENTS_TOPIC" default:"ledger.transaction.events"`
	RedpandaDecisionEventsTopic    string `env:"REDPANDA_DECISION_EVENTS_TOPIC"`
	RedpandaAuditTopic             string `env:"REDPANDA_AUDIT_TOPIC" default:"ledger.audit.log"`
	RedpandaConsumerGroup          string `env:"REDPANDA_CONSUMER_GROUP" default:"midaz-balance-projector"`
	RedpandaNumbersOfWorkers       int    `env:"REDPANDA_NUMBERS_OF_WORKERS" default:"5"`
	RedpandaPartitionCount         int    `env:"REDPANDA_PARTITION_COUNT" default:"8"`
	RedpandaCommitIntervalMS       int    `env:"REDPANDA_COMMIT_INTERVAL_MS" default:"1000"`
	RedpandaFetchMaxBytes          int    `env:"REDPANDA_FETCH_MAX_BYTES" default:"50000000"`
	RedpandaMaxRetryAttempts       int    `env:"REDPANDA_MAX_RETRY_ATTEMPTS" default:"3"`
	RedpandaProducerLingerMS       int    `env:"REDPANDA_PRODUCER_LINGER_MS" default:"5"`
	RedpandaMaxBufferedRecords     int    `env:"REDPANDA_MAX_BUFFERED_RECORDS" default:"10000"`
	RedpandaTLSEnabled             bool   `env:"REDPANDA_TLS_ENABLED" default:"false"`
	RedpandaTLSInsecureSkipVerify  bool   `env:"REDPANDA_TLS_INSECURE_SKIP_VERIFY" default:"false"`
	RedpandaTLSCAFile              string `env:"REDPANDA_TLS_CA_FILE"`
	RedpandaSASLEnabled            bool   `env:"REDPANDA_SASL_ENABLED" default:"false"`
	RedpandaSASLMechanism          string `env:"REDPANDA_SASL_MECHANISM" default:"SCRAM-SHA-256"`
	RedpandaSASLUsername           string `env:"REDPANDA_SASL_USERNAME"`
	RedpandaSASLPassword           string `env:"REDPANDA_SASL_PASSWORD"`
	TransactionEventsEnabled       bool   `env:"TRANSACTION_EVENTS_ENABLED" default:"true"`
	AuditLogEnabled                bool   `env:"AUDIT_LOG_ENABLED" default:"true"`
	OtelServiceName                string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName                string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion             string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv              string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint        string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry                bool   `env:"ENABLE_TELEMETRY"`
	RedisHost                      string `env:"REDIS_HOST"`
	RedisMasterName                string `env:"REDIS_MASTER_NAME" default:""`
	RedisPassword                  string `env:"REDIS_PASSWORD"`
	RedisDB                        int    `env:"REDIS_DB" default:"0"`
	RedisProtocol                  int    `env:"REDIS_PROTOCOL" default:"3"`
	RedisTLS                       bool   `env:"REDIS_TLS" default:"false"`
	RedisCACert                    string `env:"REDIS_CA_CERT"`
	RedisUseGCPIAM                 bool   `env:"REDIS_USE_GCP_IAM" default:"false"`
	RedisServiceAccount            string `env:"REDIS_SERVICE_ACCOUNT" default:""`
	GoogleApplicationCredentials   string `env:"GOOGLE_APPLICATION_CREDENTIALS" default:""`
	RedisTokenLifeTime             int    `env:"REDIS_TOKEN_LIFETIME" default:"60"`
	RedisTokenRefreshDuration      int    `env:"REDIS_TOKEN_REFRESH_DURATION" default:"45"`
	RedisPoolSize                  int    `env:"REDIS_POOL_SIZE" default:"10"`
	RedisMinIdleConns              int    `env:"REDIS_MIN_IDLE_CONNS" default:"0"`
	RedisReadTimeout               int    `env:"REDIS_READ_TIMEOUT" default:"3"`
	RedisWriteTimeout              int    `env:"REDIS_WRITE_TIMEOUT" default:"3"`
	RedisDialTimeout               int    `env:"REDIS_DIAL_TIMEOUT" default:"5"`
	RedisPoolTimeout               int    `env:"REDIS_POOL_TIMEOUT" default:"2"`
	RedisMaxRetries                int    `env:"REDIS_MAX_RETRIES" default:"3"`
	RedisMinRetryBackoff           int    `env:"REDIS_MIN_RETRY_BACKOFF" default:"8"`
	RedisMaxRetryBackoff           int    `env:"REDIS_MAX_RETRY_BACKOFF" default:"1"`
	AuthEnabled                    bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost                       string `env:"PLUGIN_AUTH_HOST"`
	ProtoAddress                   string `env:"PROTO_ADDRESS"`
	AuthorizerEnabled              bool   `env:"AUTHORIZER_ENABLED" default:"false"`
	AuthorizerHost                 string `env:"AUTHORIZER_HOST" default:"127.0.0.1"`
	AuthorizerPort                 string `env:"AUTHORIZER_PORT" default:"50051"`
	// AuthorizerTimeoutMS bounds a single authorizer gRPC call. The default budget
	// covers cross-shard 2PC (peer Prepare + peer Commit round-trips), the WAL
	// fsync before acknowledging Prepare, plus headroom for GC pauses at the
	// 99th percentile. Operators can still override when running single-shard
	// deployments with lower latency targets.
	AuthorizerTimeoutMS                 int    `env:"AUTHORIZER_TIMEOUT_MS" default:"250"`
	AuthorizerUseStreaming              bool   `env:"AUTHORIZER_USE_STREAMING" default:"false"`
	AuthorizerGRPCTLSEnabled            bool   `env:"AUTHORIZER_GRPC_TLS_ENABLED" default:"false"`
	AuthorizerPeerAuthToken             string `env:"AUTHORIZER_PEER_AUTH_TOKEN" default:""`
	AuthorizerRoutingMode               string `env:"AUTHORIZER_ROUTING_MODE" default:"single"`
	AuthorizerInstances                 string `env:"AUTHORIZER_INSTANCES" default:""`
	AuthorizerShardRanges               string `env:"AUTHORIZER_SHARD_RANGES" default:""`
	AuthorizerPoolSize                  int    `env:"AUTHORIZER_POOL_SIZE" default:"4"`
	BalanceSyncWorkerEnabled            bool   `env:"BALANCE_SYNC_WORKER_ENABLED" default:"true"`
	BalanceSyncMaxWorkers               int    `env:"BALANCE_SYNC_MAX_WORKERS"`
	ShardRebalanceWorkerEnabled         bool   `env:"SHARD_REBALANCE_WORKER_ENABLED" default:"false"`
	ShardRebalanceIntervalSeconds       int    `env:"SHARD_REBALANCE_INTERVAL_SECONDS" default:"5"`
	ShardRebalanceWindowSeconds         int    `env:"SHARD_REBALANCE_WINDOW_SECONDS" default:"60"`
	ShardRebalanceThresholdPercent      int    `env:"SHARD_REBALANCE_THRESHOLD_PERCENT" default:"150"`
	ShardRebalanceCandidateLimit        int    `env:"SHARD_REBALANCE_CANDIDATE_LIMIT" default:"8"`
	ShardRebalanceIsolationSharePercent int    `env:"SHARD_REBALANCE_ISOLATION_SHARE_PERCENT" default:"70"`
	ShardRebalanceIsolationMinLoad      int64  `env:"SHARD_REBALANCE_ISOLATION_MIN_LOAD" default:"250"`

	// Transaction async mode - when true, transactions are published to Redpanda for async processing.
	// Resolved once at startup and injected into UseCase to avoid per-request os.Getenv overhead.
	TransactionAsync bool `env:"TRANSACTION_ASYNC" default:"false"`

	// ConsumerEnabled controls whether this process starts Redpanda consumers and background workers.
	// Set to false when running the dedicated consumer service separately (components/consumer).
	// Default true for backward compatibility with standalone and unified ledger modes.
	ConsumerEnabled bool `env:"CONSUMER_ENABLED" default:"true"`

	// DedicatedConsumerEnabled is a deployment guardrail indicating that a standalone
	// consumer service is running. When true, API processes must set CONSUMER_ENABLED=false.
	DedicatedConsumerEnabled bool `env:"DEDICATED_CONSUMER_ENABLED" default:"false"`

	// Redis Cluster sharding (Phase 2A)
	// Set REDIS_SHARD_COUNT > 0 to enable per-shard Lua execution.
	// Default 0 = sharding disabled (legacy single-slot mode).
	RedisShardCount int `env:"REDIS_SHARD_COUNT" default:"0"`

	// ShardRoutingAllowFallback controls how the service reacts when the shard
	// manager errors but still hands back a valid router-based fallback
	// shardID. Default false is fail-closed — operators must explicitly opt in
	// to the legacy swallow-error behaviour.
	ShardRoutingAllowFallback bool `env:"SHARD_ROUTING_ALLOW_FALLBACK" default:"false"`

	ConsumerBackpressureEnabled bool `env:"CONSUMER_BACKPRESSURE_ENABLED" default:"false"`
	ConsumerMaxDBTPS            int  `env:"CONSUMER_MAX_DB_TPS" default:"0"`
	ConsumerMaxDBBurst          int  `env:"CONSUMER_MAX_DB_BURST" default:"100"`
	ConsumerLagFenceEnabled     bool `env:"CONSUMER_LAG_FENCE_ENABLED" default:"true"`
	ConsumerLagFenceFailOpen    bool `env:"CONSUMER_LAG_FENCE_FAIL_OPEN" default:"false"`
	ConsumerLagCacheTTLMS       int  `env:"CONSUMER_LAG_CACHE_TTL_MS" default:"500"`

	ConsumerBatchEnabled         bool `env:"CONSUMER_BATCH_ENABLED" default:"false"`
	ConsumerBatchSize            int  `env:"CONSUMER_BATCH_SIZE" default:"50"`
	ConsumerBatchWindowMS        int  `env:"CONSUMER_BATCH_WINDOW_MS" default:"10"`
	ConsumerIdleFlushMS          int  `env:"CONSUMER_IDLE_FLUSH_MS" default:"100"`
	ConsumerBatchImmediateCommit bool `env:"CONSUMER_BATCH_IMMEDIATE_COMMIT" default:"true"`

	// Timeout for post-commit side effects in batch processing (ms).
	// After DB commit, Redis cleanup and event publishing run with this budget.
	BatchSideEffectsTimeoutMS int `env:"BATCH_SIDE_EFFECTS_TIMEOUT_MS" default:"2000"`

	// Timeout for polling an in-flight idempotency value from Redis (ms).
	// Increase for high-TPS scenarios where 75ms is too tight.
	IdempotencyReplayTimeoutMS int `env:"IDEMPOTENCY_REPLAY_TIMEOUT_MS" default:"200"`

	ExternalPreSplitShardCount        int    `env:"EXTERNAL_PRESPLIT_SHARD_COUNT" default:"8"`
	ExternalPreSplitPreWarm           bool   `env:"EXTERNAL_PRESPLIT_PREWARM" default:"true"`
	ExternalPreSplitPreWarmTTLSeconds int    `env:"EXTERNAL_PRESPLIT_PREWARM_TTL_SECONDS" default:"3600"`
	ExternalPreSplitOrganizationID    string `env:"EXTERNAL_PRESPLIT_ORGANIZATION_ID"`
	ExternalPreSplitLedgerID          string `env:"EXTERNAL_PRESPLIT_LEDGER_ID"`
	// AllowExternalPreSplitMismatch is an explicit opt-out for the parity check between
	// EXTERNAL_PRESPLIT_SHARD_COUNT and REDIS_SHARD_COUNT. Defaults to false so a silent
	// ceiling mismatch cannot reach production; operators must acknowledge the skew.
	AllowExternalPreSplitMismatch bool `env:"ALLOW_EXTERNAL_PRESPLIT_MISMATCH" default:"false"`
	BalanceCacheTTLSeconds        int  `env:"BALANCE_CACHE_TTL_SECONDS" default:"0"`

	// Circuit Breaker configuration for producer
	BrokerCircuitBreakerConsecutiveFailures int    `env:"BROKER_CIRCUIT_BREAKER_CONSECUTIVE_FAILURES" default:"15"`
	BrokerCircuitBreakerFailureRatio        int    `env:"BROKER_CIRCUIT_BREAKER_FAILURE_RATIO" default:"50"`
	BrokerCircuitBreakerInterval            int    `env:"BROKER_CIRCUIT_BREAKER_INTERVAL" default:"120"`
	BrokerCircuitBreakerMaxRequests         int    `env:"BROKER_CIRCUIT_BREAKER_MAX_REQUESTS" default:"3"`
	BrokerCircuitBreakerMinRequests         int    `env:"BROKER_CIRCUIT_BREAKER_MIN_REQUESTS" default:"10"`
	BrokerCircuitBreakerTimeout             int    `env:"BROKER_CIRCUIT_BREAKER_TIMEOUT" default:"30"`
	BrokerCircuitBreakerHealthCheckInterval int    `env:"BROKER_CIRCUIT_BREAKER_HEALTH_CHECK_INTERVAL" default:"30"`
	BrokerOperationTimeout                  string `env:"BROKER_OPERATION_TIMEOUT" default:"3s"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding double
	// initialization when the cmd/app wants to handle bootstrap errors.
	Logger libLog.Logger

	// CircuitBreakerStateListener receives notifications when circuit breaker state changes.
	// This is optional - pass nil if you don't need state change notifications.
	CircuitBreakerStateListener libCircuitBreaker.StateChangeListener
}

func validateConsumerModeConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	if cfg.DedicatedConsumerEnabled && cfg.ConsumerEnabled {
		return ErrConsumerModeConflict
	}

	if !cfg.DedicatedConsumerEnabled && !cfg.ConsumerEnabled {
		return ErrConsumerModeNotSet
	}

	return nil
}

// validateExternalPreSplitShardCount enforces parity between the external-account
// pre-split ceiling (EXTERNAL_PRESPLIT_SHARD_COUNT) and the live shard count
// (REDIS_SHARD_COUNT). When the pre-split ceiling is smaller, large fan-out
// workloads collapse onto a subset of shards and generate contention at
// ~(batch_size / ceiling) contending writes per shard key. When the ceiling is
// larger than live shards, pre-warm wastes Redis keys that route out of range.
//
// The check is fail-closed: if REDIS_SHARD_COUNT > 0 and the ceiling is set
// (>0), the two MUST match, unless the operator has acknowledged the skew via
// ALLOW_EXTERNAL_PRESPLIT_MISMATCH=true. A ceiling of 0 disables the check —
// pre-split is effectively off.
func validateExternalPreSplitShardCount(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	if cfg.AllowExternalPreSplitMismatch {
		return nil
	}

	if cfg.ExternalPreSplitShardCount <= 0 || cfg.RedisShardCount <= 0 {
		return nil
	}

	if cfg.ExternalPreSplitShardCount == cfg.RedisShardCount {
		return nil
	}

	return fmt.Errorf("%w: EXTERNAL_PRESPLIT_SHARD_COUNT=%d REDIS_SHARD_COUNT=%d (set ALLOW_EXTERNAL_PRESPLIT_MISMATCH=true to override)",
		ErrExternalPreSplitShardMismatch,
		cfg.ExternalPreSplitShardCount,
		cfg.RedisShardCount,
	)
}

// InitServers initiate http and grpc servers.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initiates http and grpc servers with optional dependency injection.
//
//nolint:gocognit,gocyclo,cyclop // This function initializes all service dependencies; complexity is inherent.
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	if err := validateConsumerModeConfig(cfg); err != nil {
		return nil, err
	}

	if err := validateExternalPreSplitShardCount(cfg); err != nil {
		return nil, err
	}

	if cfg.AuthorizerEnabled && strings.TrimSpace(cfg.AuthorizerPeerAuthToken) == "" {
		return nil, fmt.Errorf("AUTHORIZER_PEER_AUTH_TOKEN is required when authorizer integration is enabled (AUTHORIZER_ENABLED=true): %w", constant.ErrAuthorizerPeerAuthTokenRequired)
	}

	const maxCleanupFuncs = 8

	cleanupFuncs := make([]func(), 0, maxCleanupFuncs)

	success := false

	defer func() {
		if success {
			return
		}

		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	}()

	logger, err := initLogger(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	if err := validateBrokerSecurity(cfg, logger); err != nil {
		return nil, err
	}

	// BalanceSyncWorkerEnabled defaults to true via struct tag
	balanceSyncWorkerEnabled := cfg.BalanceSyncWorkerEnabled
	logger.Infof("BalanceSyncWorker: BALANCE_SYNC_WORKER_ENABLED=%v", balanceSyncWorkerEnabled)

	shardRebalanceWorkerEnabled := cfg.ShardRebalanceWorkerEnabled
	logger.Infof("ShardRebalanceWorker: SHARD_REBALANCE_WORKER_ENABLED=%v", shardRebalanceWorkerEnabled)

	telemetry, err := libOpentelemetry.InitializeTelemetryWithError(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		telemetry.ShutdownTelemetry()
	})

	postgresConnection, err := initPostgresConnection(cfg, logger)
	if err != nil {
		return nil, err
	}

	if err := ensurePostgresConnectionReady(cfg, postgresConnection, logger); err != nil {
		return nil, err
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if postgresConnection.ConnectionDB != nil {
			if closeErr := (*postgresConnection.ConnectionDB).Close(); closeErr != nil {
				logger.Warnf("Failed to close transaction PostgreSQL connection during cleanup: %v", closeErr)
			}
		}
	})

	mongoConnection := initMongoConnection(cfg, logger)

	cleanupFuncs = append(cleanupFuncs, func() {
		if mongoConnection.DB != nil {
			if closeErr := mongoConnection.DB.Disconnect(context.Background()); closeErr != nil {
				logger.Warnf("Failed to disconnect transaction MongoDB client during cleanup: %v", closeErr)
			}
		}
	})

	redisConnection := initRedisConnection(cfg, logger)

	cleanupFuncs = append(cleanupFuncs, func() {
		if closeErr := redisConnection.Close(); closeErr != nil {
			logger.Warnf("Failed to close transaction Redis connection during cleanup: %v", closeErr)
		}
	})

	shardRouter, shardManager := initShardRouting(cfg, logger, redisConnection)

	redisConsumerRepository, err := redis.NewConsumerRedis(redisConnection, balanceSyncWorkerEnabled, shardRouter)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis: %w", err)
	}

	transactionPostgreSQLRepository, err := transaction.NewTransactionPostgreSQLRepository(postgresConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transaction repository: %w", err)
	}

	// Partition cutover wiring (see partition_wiring.go). A shared Reader
	// backs both the balance and operation wrappers so phase transitions
	// apply atomically. Primary repositories are kept alongside the wrapped
	// forms because pre-warm helpers need the concrete struct for methods
	// outside the Repository interface.
	wired, err := wirePartitionAwareRepos(postgresConnection, logger)
	if err != nil {
		return nil, err
	}

	balancePrimary := wired.balancePrimary
	operationPostgreSQLRepository := wired.operationRepo
	balancePostgreSQLRepository := wired.balanceRepo

	assetRatePostgreSQLRepository, err := assetrate.NewAssetRatePostgreSQLRepository(postgresConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize asset rate repository: %w", err)
	}

	operationRoutePostgreSQLRepository, err := operationroute.NewOperationRoutePostgreSQLRepository(postgresConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize operation route repository: %w", err)
	}

	transactionRoutePostgreSQLRepository, err := transactionroute.NewTransactionRoutePostgreSQLRepository(postgresConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transaction route repository: %w", err)
	}

	metadataMongoDBRepository, err := mongodb.NewMetadataMongoDBRepository(mongoConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metadata MongoDB repository: %w", err)
	}

	ensureMongoIndexes(mongoConnection, logger)

	seedBrokers := redpanda.ParseSeedBrokers(cfg.RedpandaBrokers)

	brokerInfra, err := initBrokerInfrastructure(cfg, opts, logger, seedBrokers, telemetry)
	if err != nil {
		return nil, err
	}

	cleanupFuncs = append(cleanupFuncs, brokerInfra.cleanupFuncs...)

	useCase := &command.UseCase{
		TransactionRepo:           transactionPostgreSQLRepository,
		OperationRepo:             operationPostgreSQLRepository,
		AssetRateRepo:             assetRatePostgreSQLRepository,
		BalanceRepo:               balancePostgreSQLRepository,
		OperationRouteRepo:        operationRoutePostgreSQLRepository,
		TransactionRouteRepo:      transactionRoutePostgreSQLRepository,
		MetadataRepo:              metadataMongoDBRepository,
		BrokerRepo:                brokerInfra.producer,
		RedisRepo:                 redisConsumerRepository,
		ShardRouter:               shardRouter,
		ShardManager:              shardManager,
		AllowShardRoutingFallback: cfg.ShardRoutingAllowFallback,
		BalanceOperationsTopic:    cfg.RedpandaBalanceOperationsTopic,
		BalanceCreateTopic:        cfg.RedpandaBalanceCreateTopic,
		EventsTopic:               cfg.RedpandaEventsTopic,
		DecisionEventsTopic:       cfg.RedpandaDecisionEventsTopic,
		EventsEnabled:             cfg.TransactionEventsEnabled,
		AuditTopic:                cfg.RedpandaAuditTopic,
		AuditLogEnabled:           cfg.AuditLogEnabled,
		TransactionAsync:          cfg.TransactionAsync,
		Version:                   cfg.Version,
		BatchSideEffectsTimeout:   time.Duration(cfg.BatchSideEffectsTimeoutMS) * time.Millisecond,
		IdempotencyReplayTimeout:  time.Duration(cfg.IdempotencyReplayTimeoutMS) * time.Millisecond,
	}

	balanceCacheTTL := resolveBalanceCacheTTL(cfg)

	queryUseCase := &query.UseCase{
		TransactionRepo:           transactionPostgreSQLRepository,
		OperationRepo:             operationPostgreSQLRepository,
		AssetRateRepo:             assetRatePostgreSQLRepository,
		BalanceRepo:               balancePostgreSQLRepository,
		OperationRouteRepo:        operationRoutePostgreSQLRepository,
		TransactionRouteRepo:      transactionRoutePostgreSQLRepository,
		MetadataRepo:              metadataMongoDBRepository,
		RedisRepo:                 redisConsumerRepository,
		ShardRouter:               shardRouter,
		ShardManager:              shardManager,
		AllowShardRoutingFallback: cfg.ShardRoutingAllowFallback,
		LagChecker:                brokerInfra.consumerLagChecker,
		ConsumerLagFenceEnabled:   cfg.ConsumerLagFenceEnabled,
		BalanceOperationsTopic:    cfg.RedpandaBalanceOperationsTopic,
		StaleBalanceRecoverer:     brokerInfra.staleBalanceRecoverer,
		BalanceCacheTTL:           balanceCacheTTL,
	}

	authorizerClient, err := grpcOut.NewAuthorizerClient(
		grpcOut.AuthorizerConfig{
			Enabled:       cfg.AuthorizerEnabled,
			Host:          cfg.AuthorizerHost,
			Port:          cfg.AuthorizerPort,
			Timeout:       time.Duration(cfg.AuthorizerTimeoutMS) * time.Millisecond,
			Streaming:     cfg.AuthorizerUseStreaming,
			TLSEnabled:    cfg.AuthorizerGRPCTLSEnabled,
			PeerAuthToken: cfg.AuthorizerPeerAuthToken,
			Environment:   cfg.EnvName,
			RoutingMode:   cfg.AuthorizerRoutingMode,
			Instances:     cfg.AuthorizerInstances,
			ShardRanges:   cfg.AuthorizerShardRanges,
			ShardCount:    cfg.RedisShardCount,
			PoolSize:      cfg.AuthorizerPoolSize,
		},
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authorizer client: %w", err)
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if closeErr := authorizerClient.Close(); closeErr != nil {
			logger.Warnf("Failed to close transaction authorizer connection during cleanup: %v", closeErr)
		}
	})

	queryUseCase.Authorizer = authorizerClient
	useCase.Authorizer = authorizerClient

	transactionHandler := &in.TransactionHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	operationHandler := &in.OperationHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	assetRateHandler := &in.AssetRateHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	balanceHandler := &in.BalanceHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	operationRouteHandler := &in.OperationRouteHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	transactionRouteHandler := &in.TransactionRouteHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	var (
		multiQueueConsumer *MultiQueueConsumer
		redisConsumer      *RedisQueueConsumer
		balanceSyncWorker  *BalanceSyncWorker
	)

	resolvedBalanceSyncWorkerEnabled := false

	var shardRebalanceWorker *ShardRebalanceWorker

	resolvedShardRebalanceWorkerEnabled := false

	if cfg.ConsumerEnabled {
		routes := configureConsumerRoutes(cfg, logger, telemetry, seedBrokers)
		multiQueueConsumer = NewMultiQueueConsumer(routes, useCase)
		redisConsumer = NewRedisQueueConsumer(logger, *transactionHandler)

		balanceSyncWorker = newBalanceSyncWorker(cfg, logger, redisConnection, useCase, balanceSyncWorkerEnabled)
		resolvedBalanceSyncWorkerEnabled = balanceSyncWorkerEnabled && balanceSyncWorker != nil

		shardRebalanceWorker = newShardRebalanceWorker(cfg, logger, shardManager, shardRouter, shardRebalanceWorkerEnabled)
		resolvedShardRebalanceWorkerEnabled = shardRebalanceWorker != nil
	} else {
		logger.Info("Skipping consumer and worker initialization because CONSUMER_ENABLED=false")
	}

	// The routing subscriber is always wired when sharding is enabled, even in
	// API-only mode, so every pod learns about routing overrides issued by its
	// peers. Without it, route-cache entries for hot accounts stay stuck on the
	// source shard across a migration boundary.
	shardRoutingSubscriber := NewShardRoutingSubscriber(logger, shardManager)

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	app := in.NewRouter(logger, telemetry, auth, transactionHandler, operationHandler, assetRateHandler, balanceHandler, operationRouteHandler, transactionRouteHandler)

	server := NewServer(cfg, app, logger, telemetry)

	resolveProtoAddress(cfg, logger)

	grpcApp := grpcIn.NewRouterGRPC(logger, telemetry, auth, useCase, queryUseCase)
	serverGRPC := NewServerGRPC(cfg, grpcApp, logger, telemetry)

	if err := preWarmExternalPreSplitBalances(cfg, logger, balancePrimary, redisConsumerRepository); err != nil {
		logger.Warnf("External pre-split Redis pre-warm failed: %v", err)
	}

	service := &Service{
		Server:                      server,
		ServerGRPC:                  serverGRPC,
		MultiQueueConsumer:          multiQueueConsumer,
		RedisQueueConsumer:          redisConsumer,
		BalanceSyncWorker:           balanceSyncWorker,
		BalanceSyncWorkerEnabled:    resolvedBalanceSyncWorkerEnabled,
		ShardRebalanceWorker:        shardRebalanceWorker,
		ShardRebalanceWorkerEnabled: resolvedShardRebalanceWorkerEnabled,
		ShardRoutingSubscriber:      shardRoutingSubscriber,
		CircuitBreakerManager:       brokerInfra.circuitBreakerManager,
		ConsumerEnabled:             cfg.ConsumerEnabled,
		Logger:                      logger,
		Ports: Ports{
			BalancePort:  useCase,
			MetadataPort: metadataMongoDBRepository,
		},
		authorizerCloser:        authorizerClient,
		auth:                    auth,
		transactionHandler:      transactionHandler,
		operationHandler:        operationHandler,
		assetRateHandler:        assetRateHandler,
		balanceHandler:          balanceHandler,
		operationRouteHandler:   operationRouteHandler,
		transactionRouteHandler: transactionRouteHandler,
		brokerProducer:          brokerInfra.producer,
		telemetry:               telemetry,
		postgresConnection:      postgresConnection,
		mongoConnection:         mongoConnection,
		redisConnection:         redisConnection,
	}

	success = true

	return service, nil
}

// compositeStateListener fans out state change notifications to multiple listeners.
type compositeStateListener struct {
	listeners []libCircuitBreaker.StateChangeListener
}

// OnStateChange notifies all registered listeners of the state change.
func (c *compositeStateListener) OnStateChange(serviceName string, from, to libCircuitBreaker.State) {
	for _, listener := range c.listeners {
		if isNilStateChangeListener(listener) {
			continue
		}

		listener.OnStateChange(serviceName, from, to)
	}
}
