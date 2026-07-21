// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v5/commons/circuitbreaker"
	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v5/commons/redis"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// PostgresChecker probes a PostgreSQL connection using SELECT 1.
type PostgresChecker struct {
	name       string
	client     *libPostgres.Client
	dsn        string
	tlsEnabled bool
}

// NewPostgresChecker creates a new PostgreSQL health checker.
func NewPostgresChecker(name string, client *libPostgres.Client, dsn string) *PostgresChecker {
	tlsEnabled := detectPostgresTLS(dsn)

	return &PostgresChecker{
		name:       name,
		client:     client,
		dsn:        dsn,
		tlsEnabled: tlsEnabled,
	}
}

// Name returns the checker identifier.
func (c *PostgresChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled for this connection.
func (c *PostgresChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes the PostgreSQL connection.
func (c *PostgresChecker) Check(ctx context.Context) DependencyCheck {
	if c.client == nil {
		return DependencyCheck{
			Status: StatusSkipped,
			Reason: "PostgreSQL client not configured",
		}
	}

	start := time.Now()

	db, err := c.client.Resolver(ctx)
	if err != nil {
		latencyMs := time.Since(start).Milliseconds()

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("failed to get database connection: %v", err),
		}
	}

	var result int

	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("SELECT 1 failed: %v", err),
		}
	}

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}
}

// MongoChecker probes a MongoDB connection using ping command.
type MongoChecker struct {
	name       string
	client     *libMongo.Client
	uri        string
	tlsEnabled bool
}

// NewMongoChecker creates a new MongoDB health checker.
func NewMongoChecker(name string, client *libMongo.Client, uri string) *MongoChecker {
	tlsEnabled, _ := detectMongoTLS(uri)

	return &MongoChecker{
		name:       name,
		client:     client,
		uri:        uri,
		tlsEnabled: tlsEnabled,
	}
}

// NewMongoCheckerWithLogger creates a new MongoDB health checker that logs TLS detection errors.
// If logger is nil, errors are silently ignored (same behavior as NewMongoChecker).
func NewMongoCheckerWithLogger(name string, client *libMongo.Client, uri string, logger libLog.Logger) *MongoChecker {
	tlsEnabled, err := detectMongoTLS(uri)
	if err != nil && logger != nil {
		logger.Log(context.Background(), libLog.LevelDebug,
			"Failed to detect MongoDB TLS configuration",
			libLog.String("checker", name),
			libLog.Err(err))
	}

	return &MongoChecker{
		name:       name,
		client:     client,
		uri:        uri,
		tlsEnabled: tlsEnabled,
	}
}

// Name returns the checker identifier.
func (c *MongoChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled for this connection.
func (c *MongoChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes the MongoDB connection.
func (c *MongoChecker) Check(ctx context.Context) DependencyCheck {
	if c.client == nil {
		return DependencyCheck{
			Status: StatusSkipped,
			Reason: "MongoDB client not configured",
		}
	}

	start := time.Now()

	err := c.client.Ping(ctx)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("ping failed: %v", err),
		}
	}

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}
}

// RedisChecker probes a Redis connection using PING command.
type RedisChecker struct {
	name       string
	client     *libRedis.Client
	host       string
	tlsEnabled bool
}

// NewRedisChecker creates a new Redis health checker.
func NewRedisChecker(name string, client *libRedis.Client, host string, tlsConfigEnabled bool) *RedisChecker {
	return &RedisChecker{
		name:       name,
		client:     client,
		host:       host,
		tlsEnabled: detectRedisTLS(host, tlsConfigEnabled),
	}
}

// Name returns the checker identifier.
func (c *RedisChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled for this connection.
func (c *RedisChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes the Redis connection.
func (c *RedisChecker) Check(ctx context.Context) DependencyCheck {
	if c.client == nil {
		return DependencyCheck{
			Status: StatusSkipped,
			Reason: "Redis client not configured",
		}
	}

	start := time.Now()

	rds, err := c.client.GetClient(ctx)
	if err != nil {
		latencyMs := time.Since(start).Milliseconds()

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("failed to get Redis client: %v", err),
		}
	}

	err = rds.Ping(ctx).Err()
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("PING failed: %v", err),
		}
	}

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}
}

// RabbitMQChecker probes RabbitMQ using the health check URL.
type RabbitMQChecker struct {
	name           string
	healthCheckURL string
	uri            string
	tlsEnabled     bool
	httpClient     *http.Client
	cbManager      libCircuitBreaker.Manager
}

// NewRabbitMQChecker creates a new RabbitMQ health checker.
// If healthCheckURL is empty, the checker returns "skipped" status.
func NewRabbitMQChecker(name, healthCheckURL, uri string, cbManager libCircuitBreaker.Manager) *RabbitMQChecker {
	tlsEnabled, _ := detectAMQPTLS(uri)

	return &RabbitMQChecker{
		name:           name,
		healthCheckURL: healthCheckURL,
		uri:            uri,
		tlsEnabled:     tlsEnabled,
		httpClient:     &http.Client{},
		cbManager:      cbManager,
	}
}

// Name returns the checker identifier.
func (c *RabbitMQChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled for this connection.
func (c *RabbitMQChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes RabbitMQ via the health check URL.
func (c *RabbitMQChecker) Check(ctx context.Context) DependencyCheck {
	// Include circuit breaker state if available
	var breakerState string

	if c.cbManager != nil {
		state := c.cbManager.GetState(rabbitmq.CircuitBreakerServiceName)
		breakerState = mapCircuitBreakerState(state)

		// If circuit breaker is open, report as degraded
		if state == libCircuitBreaker.StateOpen {
			return DependencyCheck{
				Status:       StatusDegraded,
				Reason:       "circuit breaker is open",
				BreakerState: breakerState,
			}
		}

		// If half-open, report as degraded
		if state == libCircuitBreaker.StateHalfOpen {
			return DependencyCheck{
				Status:       StatusDegraded,
				Reason:       "circuit breaker is half-open",
				BreakerState: breakerState,
			}
		}
	}

	if c.healthCheckURL == "" {
		check := DependencyCheck{
			Status: StatusSkipped,
			Reason: "RABBITMQ_HEALTH_CHECK_URL not configured",
		}

		if breakerState != "" {
			check.BreakerState = breakerState
		}

		return check
	}

	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.healthCheckURL, nil)
	if err != nil {
		latencyMs := time.Since(start).Milliseconds()

		check := DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("failed to create request: %v", err),
		}

		if breakerState != "" {
			check.BreakerState = breakerState
		}

		return check
	}

	resp, err := c.httpClient.Do(req)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		check := DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("health check request failed: %v", err),
		}

		if breakerState != "" {
			check.BreakerState = breakerState
		}

		return check
	}

	defer func() {
		// Drain and close response body
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		check := DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("health check returned status %d", resp.StatusCode),
		}

		if breakerState != "" {
			check.BreakerState = breakerState
		}

		return check
	}

	check := DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}

	if breakerState != "" {
		check.BreakerState = breakerState
	}

	return check
}

// mapCircuitBreakerState maps lib-commons circuit breaker state to string.
func mapCircuitBreakerState(state libCircuitBreaker.State) string {
	switch state {
	case libCircuitBreaker.StateClosed:
		return "closed"
	case libCircuitBreaker.StateHalfOpen:
		return "half-open"
	case libCircuitBreaker.StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// SQLDBChecker probes a raw *sql.DB connection.
type SQLDBChecker struct {
	name       string
	db         *sql.DB
	tlsEnabled bool
}

// NewSQLDBChecker creates a checker for a raw *sql.DB connection.
func NewSQLDBChecker(name string, db *sql.DB, tlsEnabled bool) *SQLDBChecker {
	return &SQLDBChecker{
		name:       name,
		db:         db,
		tlsEnabled: tlsEnabled,
	}
}

// Name returns the checker identifier.
func (c *SQLDBChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled.
func (c *SQLDBChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes the SQL database connection.
func (c *SQLDBChecker) Check(ctx context.Context) DependencyCheck {
	if c.db == nil {
		return DependencyCheck{
			Status: StatusSkipped,
			Reason: "database not configured",
		}
	}

	start := time.Now()

	var result int

	err := c.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("SELECT 1 failed: %v", err),
		}
	}

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}
}

// MongoDatabaseChecker probes a MongoDB database using runCommand.
type MongoDatabaseChecker struct {
	name     string
	database interface {
		RunCommand(context.Context, any) error
	}
	tlsEnabled bool
}

// NewMongoDatabaseChecker creates a checker for a MongoDB database.
func NewMongoDatabaseChecker(name string, database interface {
	RunCommand(context.Context, any) error
}, tlsEnabled bool,
) *MongoDatabaseChecker {
	return &MongoDatabaseChecker{
		name:       name,
		database:   database,
		tlsEnabled: tlsEnabled,
	}
}

// Name returns the checker identifier.
func (c *MongoDatabaseChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled.
func (c *MongoDatabaseChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes the MongoDB database.
func (c *MongoDatabaseChecker) Check(ctx context.Context) DependencyCheck {
	if c.database == nil {
		return DependencyCheck{
			Status: StatusSkipped,
			Reason: "database not configured",
		}
	}

	start := time.Now()

	err := c.database.RunCommand(ctx, bson.D{{Key: "ping", Value: 1}})
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("ping failed: %v", err),
		}
	}

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}
}

// VaultHealthChecker is the interface for checking Vault health status, allowing
// testing without a real vault.Client.
type VaultHealthChecker interface {
	// HealthCheck verifies Vault server availability via the sys/health endpoint.
	// Returns nil if Vault is healthy, an error otherwise.
	HealthCheck(ctx context.Context) error
}

// VaultChecker probes Vault availability via the sys/health endpoint. It is wired
// only in envelope encryption mode (KMS_VENDOR=hashicorp-vault); in legacy mode no
// Vault client exists and the checker is not registered.
type VaultChecker struct {
	name       string
	client     VaultHealthChecker
	addr       string
	tlsEnabled bool
}

// NewVaultChecker creates a new Vault health checker. The addr is used only for
// TLS detection.
func NewVaultChecker(name string, client VaultHealthChecker, addr string) *VaultChecker {
	return &VaultChecker{
		name:       name,
		client:     client,
		addr:       addr,
		tlsEnabled: detectVaultTLS(addr),
	}
}

// Name returns the checker identifier.
func (c *VaultChecker) Name() string {
	return c.name
}

// TLSEnabled returns whether TLS is enabled for this connection.
func (c *VaultChecker) TLSEnabled() bool {
	return c.tlsEnabled
}

// Check probes Vault availability via the sys/health endpoint.
func (c *VaultChecker) Check(ctx context.Context) DependencyCheck {
	if c.client == nil {
		return DependencyCheck{
			Status: StatusSkipped,
			Reason: "Vault client not configured",
		}
	}

	start := time.Now()

	err := c.client.HealthCheck(ctx)
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: &latencyMs,
			Error:     fmt.Sprintf("health check failed: %v", err),
		}
	}

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: &latencyMs,
	}
}

// detectVaultTLS reports whether the Vault address uses TLS.
func detectVaultTLS(addr string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(addr)), "https://")
}
