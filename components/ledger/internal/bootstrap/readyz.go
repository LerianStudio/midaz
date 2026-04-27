// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	libCircuitBreaker "github.com/LerianStudio/lib-commons/v4/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/LerianStudio/lib-commons/v4/commons/opentelemetry/metrics"
	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
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

	// StatusNA indicates the dependency is not applicable in the current mode (e.g., tenant-scoped in MT mode).
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

// TenantAwareDependencyChecker extends DependencyChecker for tenant-scoped probes.
// In multi-tenant mode, these checkers can resolve tenant-specific connections.
type TenantAwareDependencyChecker interface {
	DependencyChecker

	// CheckTenant probes the dependency for a specific tenant.
	// Returns StatusNA if the dependency doesn't support tenant-scoped checks.
	CheckTenant(ctx context.Context, tenantID string) DependencyCheck
}

// ReadyzHandler handles /readyz and /readyz/tenant/:id requests.
type ReadyzHandler struct {
	logger             libLog.Logger
	checkers           []DependencyChecker
	tenantCheckers     []TenantAwareDependencyChecker
	version            string
	deploymentMode     string
	multiTenantEnabled bool
	onbPGManager       *tmpostgres.Manager
	txnPGManager       *tmpostgres.Manager
	onbMongoManager    *tmmongo.Manager
	txnMongoManager    *tmmongo.Manager

	// Lifecycle state
	serverReady atomic.Bool // true after HTTP server is listening
	draining    atomic.Bool // true after SIGTERM received (graceful drain)

	// OTel metrics (nil when telemetry disabled)
	metricsFactory *metrics.MetricsFactory
}

// ReadyzHandlerConfig holds configuration for creating a ReadyzHandler.
type ReadyzHandlerConfig struct {
	Logger             libLog.Logger
	Checkers           []DependencyChecker
	TenantCheckers     []TenantAwareDependencyChecker
	Version            string
	DeploymentMode     string
	MultiTenantEnabled bool
	OnbPGManager       *tmpostgres.Manager
	TxnPGManager       *tmpostgres.Manager
	OnbMongoManager    *tmmongo.Manager
	TxnMongoManager    *tmmongo.Manager
	MetricsFactory     *metrics.MetricsFactory
}

// NewReadyzHandler creates a new ReadyzHandler with the given configuration.
func NewReadyzHandler(cfg ReadyzHandlerConfig) *ReadyzHandler {
	return &ReadyzHandler{
		logger:             cfg.Logger,
		checkers:           cfg.Checkers,
		tenantCheckers:     cfg.TenantCheckers,
		version:            cfg.Version,
		deploymentMode:     ResolveDeploymentMode(cfg.DeploymentMode),
		multiTenantEnabled: cfg.MultiTenantEnabled,
		onbPGManager:       cfg.OnbPGManager,
		txnPGManager:       cfg.TxnPGManager,
		onbMongoManager:    cfg.OnbMongoManager,
		txnMongoManager:    cfg.TxnMongoManager,
		metricsFactory:     cfg.MetricsFactory,
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

// runChecksConcurrently runs all checkers in parallel using goroutines.
// Returns a map of checker name to DependencyCheck result.
func (h *ReadyzHandler) runChecksConcurrently(ctx context.Context, checkers []DependencyChecker) map[string]DependencyCheck {
	checks := make(map[string]DependencyCheck)

	if len(checkers) == 0 {
		return checks
	}

	type checkResult struct {
		name  string
		check DependencyCheck
	}

	results := make(chan checkResult, len(checkers))

	var wg sync.WaitGroup

	for _, checker := range checkers {
		wg.Add(1)

		go func(c DependencyChecker) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, h.timeoutForChecker(c))
			defer cancel()

			start := time.Now()
			check := c.Check(checkCtx)
			durationMs := time.Since(start).Milliseconds()

			// Set TLS field
			if c.TLSEnabled() {
				tlsEnabled := true
				check.TLS = &tlsEnabled
			} else {
				tlsDisabled := false
				check.TLS = &tlsDisabled
			}

			// Record metrics
			h.recordCheckMetrics(ctx, c.Name(), check.Status, durationMs)

			// Log full error and sanitize for non-local modes
			h.logAndSanitizeCheck(ctx, c.Name(), &check)

			results <- checkResult{name: c.Name(), check: check}
		}(checker)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for result := range results {
		checks[result.name] = result.check
	}

	return checks
}

// runTenantChecksConcurrently runs tenant-aware checkers in parallel for a specific tenant.
func (h *ReadyzHandler) runTenantChecksConcurrently(ctx context.Context, tenantID string, checkers []TenantAwareDependencyChecker) map[string]DependencyCheck {
	checks := make(map[string]DependencyCheck)

	if len(checkers) == 0 {
		return checks
	}

	type checkResult struct {
		name  string
		check DependencyCheck
	}

	results := make(chan checkResult, len(checkers))

	var wg sync.WaitGroup

	for _, checker := range checkers {
		wg.Add(1)

		go func(c TenantAwareDependencyChecker) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, h.timeoutForChecker(c))
			defer cancel()

			start := time.Now()
			check := c.CheckTenant(checkCtx, tenantID)
			durationMs := time.Since(start).Milliseconds()

			// Set TLS field
			if c.TLSEnabled() {
				tlsEnabled := true
				check.TLS = &tlsEnabled
			} else {
				tlsDisabled := false
				check.TLS = &tlsDisabled
			}

			// Record metrics
			h.recordCheckMetrics(ctx, c.Name(), check.Status, durationMs)

			// Log full error and sanitize for non-local modes
			h.logAndSanitizeCheck(ctx, c.Name(), &check)

			results <- checkResult{name: c.Name(), check: check}
		}(checker)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for result := range results {
		checks[result.name] = result.check
	}

	return checks
}

// HandleReadyz handles the /readyz endpoint for global health checks.
// In multi-tenant mode, database checkers return "n/a" since connections are tenant-scoped.
// Redis always returns actual status (shared infrastructure).
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

	// Run all checkers concurrently
	checks := h.runChecksConcurrently(c.Context(), h.checkers)

	// Determine overall health status
	allHealthy := true

	for _, check := range checks {
		if check.Status == StatusDown || check.Status == StatusDegraded {
			allHealthy = false

			break
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

// HandleReadyzTenant handles the /readyz/tenant/:id endpoint for tenant-specific health checks.
// This endpoint resolves tenant-specific connections and probes them directly.
func (h *ReadyzHandler) HandleReadyzTenant(c *fiber.Ctx) error {
	tenantID := c.Params("id")
	if tenantID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "tenant ID is required",
		})
	}

	if !h.multiTenantEnabled {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "tenant-scoped readyz is only available in multi-tenant mode",
		})
	}

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

	// Set tenant ID in context for downstream managers
	ctx := tmcore.ContextWithTenantID(c.Context(), tenantID)

	// Run tenant-aware checkers concurrently
	tenantChecks := h.runTenantChecksConcurrently(ctx, tenantID, h.tenantCheckers)

	// Get non-tenant-aware checkers (like Redis which is shared)
	nonTenantCheckers := h.getNonTenantAwareCheckers()

	// Run non-tenant-aware checkers concurrently
	sharedChecks := h.runChecksConcurrently(ctx, nonTenantCheckers)

	// Merge results
	checks := make(map[string]DependencyCheck)
	for name, check := range tenantChecks {
		checks[name] = check
	}

	for name, check := range sharedChecks {
		checks[name] = check
	}

	// Determine overall health status
	allHealthy := true

	for _, check := range checks {
		if check.Status == StatusDown || check.Status == StatusDegraded {
			allHealthy = false

			break
		}
	}

	status := "healthy"
	httpStatus := http.StatusOK

	if !allHealthy {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	// Record request metrics
	h.recordRequestMetrics(ctx, "/readyz/tenant", allHealthy)

	response := ReadyzResponse{
		Status:         status,
		Checks:         checks,
		Version:        h.version,
		DeploymentMode: h.deploymentMode,
	}

	return c.Status(httpStatus).JSON(response)
}

// getNonTenantAwareCheckers returns checkers that don't have a tenant-aware version.
// These are shared infrastructure components like Redis.
func (h *ReadyzHandler) getNonTenantAwareCheckers() []DependencyChecker {
	var result []DependencyChecker

	for _, checker := range h.checkers {
		if !h.hasTenantChecker(checker.Name()) {
			result = append(result, checker)
		}
	}

	return result
}

// hasTenantChecker returns true if a tenant-aware checker with the given name exists.
func (h *ReadyzHandler) hasTenantChecker(name string) bool {
	for _, tc := range h.tenantCheckers {
		if tc.Name() == name {
			return true
		}
	}

	return false
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

// buildReadyzHandler creates the ReadyzHandler with appropriate checkers based on mode.
// In multi-tenant mode, database checkers return "n/a" globally; use /readyz/tenant/:id for tenant-specific checks.
// Redis is shared infrastructure and always returns actual status.
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

	var tenantCheckers []TenantAwareDependencyChecker

	if cfg.MultiTenantEnabled {
		// Multi-tenant mode: database checkers return n/a globally
		// Tenant-specific checks are done via /readyz/tenant/:id

		// PostgreSQL - n/a globally, tenant-aware checkers for /readyz/tenant/:id
		onbPGTLSEnabled := detectPostgresTLS(onbPGDSN)
		txnPGTLSEnabled := detectPostgresTLS(txnPGDSN)

		checkers = append(checkers,
			NewNAChecker("postgres_onboarding", "tenant-scoped; use /readyz/tenant/:id", onbPGTLSEnabled),
			NewNAChecker("postgres_transaction", "tenant-scoped; use /readyz/tenant/:id", txnPGTLSEnabled),
		)

		if onbPG.pgManager != nil {
			tenantCheckers = append(tenantCheckers,
				NewTenantPostgresChecker("postgres_onboarding", onbPG.pgManager, onbPGDSN))
		}

		if txnPG.pgManager != nil {
			tenantCheckers = append(tenantCheckers,
				NewTenantPostgresChecker("postgres_transaction", txnPG.pgManager, txnPGDSN))
		}

		// MongoDB - n/a globally, tenant-aware checkers for /readyz/tenant/:id
		onbMongoTLSEnabled, _ := detectMongoTLS(onbMongoURI)
		txnMongoTLSEnabled, _ := detectMongoTLS(txnMongoURI)

		checkers = append(checkers,
			NewNAChecker("mongo_onboarding", "tenant-scoped; use /readyz/tenant/:id", onbMongoTLSEnabled),
			NewNAChecker("mongo_transaction", "tenant-scoped; use /readyz/tenant/:id", txnMongoTLSEnabled),
		)

		if onbMgo.mongoManager != nil {
			tenantCheckers = append(tenantCheckers,
				NewTenantMongoChecker("mongo_onboarding", onbMgo.mongoManager, onbMongoURI))
		}

		if txnMgo.mongoManager != nil {
			tenantCheckers = append(tenantCheckers,
				NewTenantMongoChecker("mongo_transaction", txnMgo.mongoManager, txnMongoURI))
		}
	} else {
		// Single-tenant mode: all checkers return actual status

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
	}

	// Redis - always returns actual status (shared infrastructure)
	if redisConnection != nil {
		checkers = append(checkers,
			NewRedisChecker("redis", redisConnection, cfg.RedisHost, cfg.RedisTLS))
	}

	// RabbitMQ - check via health URL if configured
	var cbManager libCircuitBreaker.Manager

	if rmq != nil && rmq.circuitBreakerManager != nil {
		cbManager = rmq.circuitBreakerManager.Manager
	}

	checkers = append(checkers,
		NewRabbitMQChecker("rabbitmq", cfg.RabbitMQHealthCheckURL, rmqURI, cbManager))

	// Build TLS validation results from already-created checkers.
	// Each checker detected TLS during construction, so we reuse those results
	// instead of calling detect*TLS() functions again.
	tlsResults := make([]TLSValidationResult, 0, len(checkers))
	for _, checker := range checkers {
		tlsResults = append(tlsResults, TLSValidationResult{
			Name:       checker.Name(),
			TLSEnabled: checker.TLSEnabled(),
		})
	}

	// ValidateSaaSTLS returns error ONLY for DEPLOYMENT_MODE=saas with insecure deps.
	// The error is returned to the caller to fail startup.
	if err := ValidateSaaSTLS(cfg.DeploymentMode, tlsResults); err != nil {
		return nil, err
	}

	// For BYOC mode, log a warning for insecure dependencies (recommended but not enforced)
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
		Logger:             logger,
		Checkers:           checkers,
		TenantCheckers:     tenantCheckers,
		Version:            cfg.Version,
		DeploymentMode:     cfg.DeploymentMode,
		MultiTenantEnabled: cfg.MultiTenantEnabled,
		OnbPGManager:       onbPG.pgManager,
		TxnPGManager:       txnPG.pgManager,
		OnbMongoManager:    onbMgo.mongoManager,
		TxnMongoManager:    txnMgo.mongoManager,
		MetricsFactory:     metricsFactory,
	}), nil
}
