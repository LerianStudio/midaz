// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v5/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	"github.com/LerianStudio/lib-commons/v5/commons/opentelemetry/metrics"
	libRedis "github.com/LerianStudio/lib-commons/v5/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// DependencyStatus represents the health status of a dependency.
// This is a closed vocabulary - only these values are valid.
type DependencyStatus string

const (
	// StatusUp indicates the dependency probe succeeded.
	StatusUp DependencyStatus = "up"

	// StatusDown indicates the dependency probe failed.
	StatusDown DependencyStatus = "down"

	// StatusDegraded indicates the dependency is in a degraded state (e.g., circuit breaker half-open).
	StatusDegraded DependencyStatus = "degraded"

	// StatusSkipped indicates the dependency is optional and was not probed (e.g., disabled by config).
	StatusSkipped DependencyStatus = "skipped"

	// StatusNA indicates the dependency is not applicable in the current context.
	StatusNA DependencyStatus = "n/a"
)

const (
	// DefaultRedisTimeout is the default timeout for Redis health checks.
	DefaultRedisTimeout = 1 * time.Second

	// DefaultDatabaseTimeout is the default timeout for PostgreSQL and MongoDB health checks.
	DefaultDatabaseTimeout = 2 * time.Second

	// DefaultRabbitMQTimeout is the default timeout for RabbitMQ health checks.
	DefaultRabbitMQTimeout = 2 * time.Second

	// DefaultDrainDelay is the time to wait after starting drain before fully shutting down.
	// This gives load balancers time to stop sending traffic to the pod.
	DefaultDrainDelay = 12 * time.Second
)

// DependencyCheck represents the health check result for a single dependency.
type DependencyCheck struct {
	Status       DependencyStatus `json:"status"`
	LatencyMs    *int64           `json:"latency_ms,omitempty"`
	TLS          *bool            `json:"tls,omitempty"`
	Error        string           `json:"error,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	BreakerState string           `json:"breaker_state,omitempty"`
}

// ReadyzResponse is the response body for the /readyz endpoint.
type ReadyzResponse struct {
	Status         string                     `json:"status"`
	Checks         map[string]DependencyCheck `json:"checks"`
	Version        string                     `json:"version"`
	DeploymentMode string                     `json:"deployment_mode"`
	Reason         string                     `json:"reason,omitempty"`
}

// DependencyChecker is the interface for probing a dependency's health.
type DependencyChecker interface {
	// Name returns the dependency identifier used as the key in the checks map.
	Name() string

	// Check probes the dependency and returns the health check result.
	// The context should have a timeout applied by the caller.
	Check(ctx context.Context) DependencyCheck

	// TLSEnabled returns whether TLS is enabled for this dependency.
	TLSEnabled() bool
}

// ReadyzHandler handles /readyz requests.
type ReadyzHandler struct {
	logger         libLog.Logger
	checkers       []DependencyChecker
	version        string
	deploymentMode string

	// Lifecycle state
	serverReady atomic.Bool // true after HTTP server is listening
	draining    atomic.Bool // true after SIGTERM received (graceful drain)

	// OTel metrics (nil when telemetry disabled)
	metricsFactory *metrics.MetricsFactory
}

// ReadyzHandlerConfig holds configuration for creating a ReadyzHandler.
type ReadyzHandlerConfig struct {
	Logger         libLog.Logger
	Checkers       []DependencyChecker
	Version        string
	DeploymentMode string
	MetricsFactory *metrics.MetricsFactory
}

// NewReadyzHandler creates a new ReadyzHandler with the given configuration.
func NewReadyzHandler(cfg ReadyzHandlerConfig) *ReadyzHandler {
	return &ReadyzHandler{
		logger:         cfg.Logger,
		checkers:       cfg.Checkers,
		version:        cfg.Version,
		deploymentMode: ResolveDeploymentMode(cfg.DeploymentMode),
		metricsFactory: cfg.MetricsFactory,
	}
}

// recordCheckMetrics records OTel metrics for a health check result.
func (h *ReadyzHandler) recordCheckMetrics(ctx context.Context, checkerName string, status DependencyStatus, durationMs int64) {
	if h.metricsFactory == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("checker", checkerName),
		attribute.String("status", string(status)),
	}

	// Record check duration histogram
	if histogram, err := h.metricsFactory.Histogram(utils.ReadyzCheckDuration); err == nil {
		_ = histogram.WithAttributes(attrs...).Record(ctx, durationMs)
	}

	// Record check status counter
	if counter, err := h.metricsFactory.Counter(utils.ReadyzCheckStatus); err == nil {
		_ = counter.WithAttributes(attrs...).Add(ctx, 1)
	}
}

// recordRequestMetrics records OTel metrics for a readyz request.
func (h *ReadyzHandler) recordRequestMetrics(ctx context.Context, endpoint string, healthy bool) {
	if h.metricsFactory == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("endpoint", endpoint),
		attribute.Bool("healthy", healthy),
	}

	if counter, err := h.metricsFactory.Counter(utils.ReadyzRequestsTotal); err == nil {
		_ = counter.WithAttributes(attrs...).Add(ctx, 1)
	}
}

// SetServerReady marks the server as ready to accept traffic.
// Call this after the HTTP server has started listening.
func (h *ReadyzHandler) SetServerReady() {
	h.serverReady.Store(true)
	h.logger.Log(context.Background(), libLog.LevelInfo, "Readyz: server marked as ready")
}

// StartDrain initiates graceful drain mode.
// After calling this, readyz will return 503 to signal load balancers to stop sending traffic.
func (h *ReadyzHandler) StartDrain() {
	h.draining.Store(true)
	h.logger.Log(context.Background(), libLog.LevelInfo, "Readyz: graceful drain started")
}

// IsDraining returns true if the handler is in drain mode.
func (h *ReadyzHandler) IsDraining() bool {
	return h.draining.Load()
}

// IsServerReady returns true if the server is ready to accept traffic.
func (h *ReadyzHandler) IsServerReady() bool {
	return h.serverReady.Load()
}

// checkLifecycleState checks the server lifecycle state (self-probe and graceful drain).
// Returns (reason, ok) where ok is false if the server should return 503.
func (h *ReadyzHandler) checkLifecycleState() (string, bool) {
	if !h.serverReady.Load() {
		return "server not ready (startup in progress)", false
	}

	if h.draining.Load() {
		return "server draining (shutdown in progress)", false
	}

	return "", true
}

// HandleReadyz handles the /readyz endpoint for global health checks.
// All configured dependency checkers are probed and their status is returned.
func (h *ReadyzHandler) HandleReadyz(c *fiber.Ctx) error {
	// Check lifecycle state first (self-probe and graceful drain)
	if reason, ok := h.checkLifecycleState(); !ok {
		return c.Status(http.StatusServiceUnavailable).JSON(ReadyzResponse{
			Status:         "unhealthy",
			Checks:         map[string]DependencyCheck{},
			Version:        h.version,
			DeploymentMode: h.deploymentMode,
			Reason:         reason,
		})
	}

	checks := make(map[string]DependencyCheck)
	allHealthy := true

	// Run all checkers sequentially
	for _, checker := range h.checkers {
		checkCtx, cancel := context.WithTimeout(c.Context(), h.timeoutForChecker(checker))

		start := time.Now()
		check := checker.Check(checkCtx)
		durationMs := time.Since(start).Milliseconds()

		cancel()

		// Set TLS field
		if checker.TLSEnabled() {
			tlsEnabled := true
			check.TLS = &tlsEnabled
		} else {
			tlsDisabled := false
			check.TLS = &tlsDisabled
		}

		// Record metrics
		h.recordCheckMetrics(c.Context(), checker.Name(), check.Status, durationMs)

		// Log full error and sanitize for non-local modes
		h.logAndSanitizeCheck(c.Context(), checker.Name(), &check)

		checks[checker.Name()] = check

		if check.Status == StatusDown || check.Status == StatusDegraded {
			allHealthy = false
		}
	}

	status := "healthy"
	httpStatus := http.StatusOK

	if !allHealthy {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	// Record request metrics
	h.recordRequestMetrics(c.Context(), "/readyz", allHealthy)

	response := ReadyzResponse{
		Status:         status,
		Checks:         checks,
		Version:        h.version,
		DeploymentMode: h.deploymentMode,
	}

	return c.Status(httpStatus).JSON(response)
}

// timeoutForChecker returns the appropriate timeout for a checker based on its name.
// Database probes (postgres, mongo) get DefaultDatabaseTimeout, Redis gets DefaultRedisTimeout,
// RabbitMQ gets DefaultRabbitMQTimeout.
func (h *ReadyzHandler) timeoutForChecker(checker DependencyChecker) time.Duration {
	name := checker.Name()

	switch {
	case contains(name, "redis"):
		return DefaultRedisTimeout
	case contains(name, "postgres"), contains(name, "mongo"):
		return DefaultDatabaseTimeout
	case contains(name, "rabbitmq"):
		return DefaultRabbitMQTimeout
	default:
		return DefaultDatabaseTimeout
	}
}

// contains checks if substr is found in s (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsLower(s, substr)))
}

// containsLower performs a case-insensitive substring search.
func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true

		for j := range len(substr) {
			sc := s[i+j]
			uc := substr[j]

			// Convert to lowercase for comparison
			if sc >= 'A' && sc <= 'Z' {
				sc += 'a' - 'A'
			}

			if uc >= 'A' && uc <= 'Z' {
				uc += 'a' - 'A'
			}

			if sc != uc {
				match = false
				break
			}
		}

		if match {
			return true
		}
	}

	return false
}

// sanitizeError returns a sanitized error message for non-local deployment modes.
// In local mode, the full error is returned for debugging.
// In saas/byoc modes, the error is sanitized to prevent leaking internal details.
func (h *ReadyzHandler) sanitizeError(checkerName, originalError string) string {
	if h.deploymentMode == DeploymentModeLocal {
		return originalError
	}

	return fmt.Sprintf("%s check failed", checkerName)
}

// logAndSanitizeCheck logs the full error server-side and sanitizes it in the response.
// This ensures operators can debug issues while preventing information disclosure to clients.
func (h *ReadyzHandler) logAndSanitizeCheck(ctx context.Context, checkerName string, check *DependencyCheck) {
	if check.Error == "" {
		return
	}

	// Always log the full error server-side for debugging
	h.logger.Log(ctx, libLog.LevelWarn, "Health check failed",
		libLog.String("checker", checkerName),
		libLog.String("status", string(check.Status)),
		libLog.String("error", check.Error))

	// Sanitize the error in the response for non-local modes
	check.Error = h.sanitizeError(checkerName, check.Error)
}

// buildReadyzHandler creates the ReadyzHandler with appropriate checkers.
// All checkers return actual status for single-tenant mode.
func buildReadyzHandler(
	cfg *Config,
	logger libLog.Logger,
	redisConnection *libRedis.Client,
	onbPG *onboardingPostgresComponents,
	txnPG *transactionPostgresComponents,
	onbMgo *onboardingMongoComponents,
	txnMgo *transactionMongoComponents,
	rmq *rabbitMQComponents,
	metricsFactory *metrics.MetricsFactory,
) (*ReadyzHandler, error) {
	// Build DSN strings for TLS detection
	onbPGDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.OnbPrefixedPrimaryDBHost, cfg.OnbPrefixedPrimaryDBUser, cfg.OnbPrefixedPrimaryDBPassword,
		cfg.OnbPrefixedPrimaryDBName, cfg.OnbPrefixedPrimaryDBPort, cfg.OnbPrefixedPrimaryDBSSLMode)

	txnPGDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.TxnPrefixedPrimaryDBHost, cfg.TxnPrefixedPrimaryDBUser, cfg.TxnPrefixedPrimaryDBPassword,
		cfg.TxnPrefixedPrimaryDBName, cfg.TxnPrefixedPrimaryDBPort, cfg.TxnPrefixedPrimaryDBSSLMode)

	// Build Mongo URIs for TLS detection
	onbMongoURI := fmt.Sprintf("%s://%s:%s@%s:%s/%s?%s",
		cfg.OnbPrefixedMongoURI, cfg.OnbPrefixedMongoDBUser, cfg.OnbPrefixedMongoDBPassword,
		cfg.OnbPrefixedMongoDBHost, cfg.OnbPrefixedMongoDBPort, cfg.OnbPrefixedMongoDBName,
		cfg.OnbPrefixedMongoDBParameters)

	txnMongoURI := fmt.Sprintf("%s://%s:%s@%s:%s/%s?%s",
		cfg.TxnPrefixedMongoURI, cfg.TxnPrefixedMongoDBUser, cfg.TxnPrefixedMongoDBPassword,
		cfg.TxnPrefixedMongoDBHost, cfg.TxnPrefixedMongoDBPort, cfg.TxnPrefixedMongoDBName,
		cfg.TxnPrefixedMongoDBParameters)

	// Build RabbitMQ URI for TLS detection
	rmqURI := buildRabbitMQConnectionString(
		cfg.RabbitURI, cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost, cfg.RabbitMQVHost)

	var checkers []DependencyChecker

	// PostgreSQL checkers
	if onbPG.connection != nil {
		checkers = append(checkers,
			NewPostgresChecker("postgres_onboarding", onbPG.connection, onbPGDSN))
	}

	if txnPG.connection != nil {
		checkers = append(checkers,
			NewPostgresChecker("postgres_transaction", txnPG.connection, txnPGDSN))
	}

	// MongoDB checkers
	if onbMgo.connection != nil {
		checkers = append(checkers,
			NewMongoChecker("mongo_onboarding", onbMgo.connection, onbMongoURI))
	}

	if txnMgo.connection != nil {
		checkers = append(checkers,
			NewMongoChecker("mongo_transaction", txnMgo.connection, txnMongoURI))
	}

	// Redis checker
	if redisConnection != nil {
		checkers = append(checkers,
			NewRedisChecker("redis", redisConnection, cfg.RedisHost, cfg.RedisTLS))
	}

	// RabbitMQ checker
	var cbManager libCircuitBreaker.Manager

	if rmq != nil && rmq.circuitBreakerManager != nil {
		cbManager = rmq.circuitBreakerManager.Manager
	}

	checkers = append(checkers,
		NewRabbitMQChecker("rabbitmq", cfg.RabbitMQHealthCheckURL, rmqURI, cbManager))

	// Build TLS validation results from already-created checkers.
	tlsResults := make([]TLSValidationResult, 0, len(checkers))
	for _, checker := range checkers {
		tlsResults = append(tlsResults, TLSValidationResult{
			Name:       checker.Name(),
			TLSEnabled: checker.TLSEnabled(),
		})
	}

	// ValidateSaaSTLS returns error ONLY for DEPLOYMENT_MODE=saas with insecure deps.
	if err := ValidateSaaSTLS(cfg.DeploymentMode, tlsResults); err != nil {
		return nil, err
	}

	// For BYOC mode, log a warning for insecure dependencies
	if IsTLSRecommended(cfg.DeploymentMode) {
		for _, result := range tlsResults {
			if !result.TLSEnabled {
				logger.Log(context.Background(), libLog.LevelWarn,
					"TLS recommended but not configured for dependency",
					libLog.String("dependency", result.Name),
					libLog.String("deployment_mode", cfg.DeploymentMode))
			}
		}
	}

	return NewReadyzHandler(ReadyzHandlerConfig{
		Logger:         logger,
		Checkers:       checkers,
		Version:        cfg.Version,
		DeploymentMode: cfg.DeploymentMode,
		MetricsFactory: metricsFactory,
	}), nil
}
