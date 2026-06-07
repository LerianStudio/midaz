// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/lib-observability/log"
)

//go:generate mockgen --destination=health-checker.mock.go --package=pkg --copyright_file=../COPYRIGHT . HealthCheckRunner

// Compile-time interface satisfaction check.
var _ HealthCheckRunner = (*HealthChecker)(nil)

// HealthCheckRunner defines the interface for managing datasource health checks.
type HealthCheckRunner interface {
	// Start begins the health check loop in a background goroutine.
	Start()
	// Stop gracefully stops the health checker.
	Stop()
	// GetHealthStatus returns the current health status of all datasources.
	GetHealthStatus() map[string]string
}

// HealthChecker performs periodic health checks on datasources and attempts reconnection
type HealthChecker struct {
	dataSources           *map[string]DataSource
	circuitBreakerManager *CircuitBreakerManager
	logger                log.Logger
	stopChan              chan struct{}
	wg                    sync.WaitGroup
	mu                    sync.RWMutex
	// metrics emits per-datasource health and ping-duration signals to the
	// configured OTel meter. nil is tolerated — emit calls are no-ops on a
	// nil receiver, which keeps NewHealthChecker callers backward-compatible.
	metrics *DatasourceMetrics
}

// NewHealthChecker creates a new health checker instance.
//
// Returns an error if any required dependency is nil. The returned checker
// has no metrics emitter wired; use SetMetrics or NewHealthCheckerWithMetrics
// to enable per-datasource emission. NewHealthChecker delegates to
// NewHealthCheckerWithMetrics with metrics=nil so both constructors share a
// single validation path — adding a check in one constructor automatically
// applies to the other.
func NewHealthChecker(
	dataSources *map[string]DataSource,
	circuitBreakerManager *CircuitBreakerManager,
	logger log.Logger,
) (*HealthChecker, error) {
	return NewHealthCheckerWithMetrics(dataSources, circuitBreakerManager, logger, nil)
}

// NewHealthCheckerWithMetrics is the metrics-aware constructor. It builds the
// same HealthChecker as NewHealthChecker but additionally wires the provided
// DatasourceMetrics emitter. Pass nil to opt out (semantics identical to
// NewHealthChecker).
//
// Validation: the constructor refuses to build a HealthChecker when any of
// dataSources, circuitBreakerManager, or logger is nil. performHealthChecks
// and GetHealthStatus dereference all three; failing fast at construction
// surfaces the misconfiguration at bootstrap rather than as a nil-pointer
// panic on the first /readyz request or background tick. metrics is the
// only nilable input — it is the documented opt-out for callers that wire
// the emitter later via SetMetrics.
func NewHealthCheckerWithMetrics(
	dataSources *map[string]DataSource,
	circuitBreakerManager *CircuitBreakerManager,
	logger log.Logger,
	metrics *DatasourceMetrics,
) (*HealthChecker, error) {
	if dataSources == nil {
		return nil, errors.New("health checker: dataSources must not be nil")
	}

	if circuitBreakerManager == nil {
		return nil, errors.New("health checker: circuitBreakerManager must not be nil")
	}

	if logger == nil {
		return nil, errors.New("health checker: logger must not be nil")
	}

	return &HealthChecker{
		dataSources:           dataSources,
		circuitBreakerManager: circuitBreakerManager,
		logger:                logger,
		stopChan:              make(chan struct{}),
		metrics:               metrics,
	}, nil
}

// SetMetrics installs (or replaces) the DatasourceMetrics emitter on an
// existing HealthChecker. Safe to call before Start. Calling after Start
// is supported but emission for the in-flight pass uses whichever pointer
// is set when each ping completes — there is no guarantee that a single
// pass uses a single emitter, so callers should treat post-Start swaps as
// best-effort.
func (hc *HealthChecker) SetMetrics(metrics *DatasourceMetrics) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.metrics = metrics
}

// Start begins the health check loop in a separate goroutine
func (hc *HealthChecker) Start() {
	hc.wg.Add(1)

	GoWithCleanup(hc.logger, func() {
		hc.healthCheckLoop()
	}, nil)
	hc.logger.Log(context.Background(), log.LevelInfo, "Health checker started - checking datasources every 30s")
}

// Stop gracefully stops the health checker
func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()
	hc.logger.Log(context.Background(), log.LevelInfo, "Health checker stopped")
}

// healthCheckLoop runs the periodic health checks
func (hc *HealthChecker) healthCheckLoop() {
	defer hc.wg.Done()

	ticker := time.NewTicker(constant.HealthCheckInterval)
	defer ticker.Stop()

	// Run initial check after a short delay
	time.Sleep(5 * time.Second)
	hc.performHealthChecks()

	for {
		select {
		case <-ticker.C:
			hc.performHealthChecks()
		case <-hc.stopChan:
			return
		}
	}
}

// performHealthChecks checks all datasources and attempts reconnection if needed.
//
// As of Gate 5 (readyz metrics emission), this loop also emits two OTel
// metrics per datasource per pass:
//   - datasource_check_duration_ms (histogram): how long the ping took
//   - datasource_healthy (gauge): 1 if the datasource is up, 0 otherwise
//
// Emission happens for every datasource, healthy or not. needsHealing-only
// emission would leave dashboards blind to permanently-healthy datasources
// (no time series at all), so we deliberately ping every datasource on
// each pass. The ping itself is a lightweight repository-level Ping(ctx)
// — SELECT 1 for PostgreSQL, db.runCommand({ping:1}) for MongoDB — issued
// by pingDataSource.
//
// Single-probe invariant (Dispatch 2 fix):
//
// Each iteration of the loop pings a datasource AT MOST ONCE per pass —
// even when reconnection is required. Previously, attemptReconnection ran
// its own internal verification ping AND the metric emitter ran another
// ping immediately after, doubling the load against datasources that were
// already struggling. Now reconnection only re-establishes the underlying
// connection; the verification ping is folded into the metric emission
// step and its result is consumed by both observers.
func (hc *HealthChecker) performHealthChecks() {
	hc.mu.RLock()
	// Create a shallow copy of the map to iterate over, preventing a concurrent map write panic.
	dataSourcesSnapshot := make(map[string]DataSource, len(*hc.dataSources))
	for name, ds := range *hc.dataSources {
		dataSourcesSnapshot[name] = ds
	}

	metrics := hc.metrics

	hc.mu.RUnlock()
	hc.logger.Log(context.Background(), log.LevelDebug, "Performing health checks on all datasources...")

	unavailableCount := 0
	reconnectedCount := 0

	for name, ds := range dataSourcesSnapshot {
		needsHeal := hc.needsHealing(name, ds)

		reconnectAttempted := false
		reconnectErr := error(nil)

		// Re-establish underlying connection if needed. The verification
		// ping that confirms connectivity is deferred to the single ping
		// performed below (instead of being duplicated inside).
		if needsHeal {
			unavailableCount++

			hc.logger.Log(context.Background(), log.LevelDebug, "Attempting to heal datasource", log.String("datasource", name), log.String("status", ds.Status))

			reconnectAttempted = true
			reconnectErr = hc.reestablishConnection(name, &ds)
		}

		// Ping ONCE per loop iteration — only when at least one observer
		// needs the result. Observers are:
		//   - reconnectAttempted: the verification ping that resolves the
		//     reconnection outcome (Available / Degraded / Unavailable).
		//   - metrics != nil: the OTel emitter that wants a continuous
		//     time series for dashboards.
		// When neither observer is present (no healing needed and no
		// metrics configured) the loop body is a no-op for this
		// datasource — preserving the legacy "no metrics → no probe"
		// shape that callers relied on.
		var (
			healthy bool
			elapsed time.Duration
			pinged  bool
		)

		if reconnectAttempted || metrics != nil {
			healthy, elapsed = hc.timedPing(context.Background(), name, &ds)
			pinged = true
		}

		// Resolve reconnection outcome based on the single ping result.
		if reconnectAttempted {
			switch {
			case reconnectErr != nil:
				// Connection re-establishment failed outright — we never
				// got a valid repository back, so the ping above used the
				// stale (or nil) handle and almost certainly returned
				// false. Surface the connect error as the cause.
				hc.logger.Log(context.Background(), log.LevelError, "Failed to reconnect datasource - will retry",
					log.String("datasource", name),
					log.Err(reconnectErr),
					log.Any("retry_interval", constant.HealthCheckInterval),
				)

				ds.Status = libConstants.DataSourceStatusUnavailable
				ds.LastError = reconnectErr
			case !healthy:
				// Reconnect succeeded but the verification ping (the only
				// ping we run) reports failure. Marks the datasource as
				// degraded — same semantic as the previous double-ping
				// implementation, but now without the duplicate probe.
				hc.logger.Log(context.Background(), log.LevelError, "Reconnection succeeded but ping failed", log.String("datasource", name))

				ds.Status = libConstants.DataSourceStatusDegraded
			default:
				// Reconnect succeeded and ping is healthy. Same flip as
				// the legacy attemptReconnection success path.
				ds.Status = libConstants.DataSourceStatusAvailable
				ds.Initialized = true
				ds.LastError = nil
				reconnectedCount++

				hc.circuitBreakerManager.Reset(name)
				hc.logger.Log(context.Background(), log.LevelDebug, "Datasource reconnected successfully - circuit breaker reset", log.String("datasource", name))
			}

			// Persist updated datasource state so health/readiness sees current state.
			hc.mu.Lock()
			(*hc.dataSources)[name] = ds
			hc.mu.Unlock()
		}

		// Emit metrics for every datasource on every pass — including the
		// ones that did NOT need healing — so dashboards see a continuous
		// time series. We use the ping result captured above (no second
		// probe).
		if metrics != nil && pinged {
			metrics.EmitDatasourceCheckDuration(context.Background(), name, elapsed)
			metrics.EmitDatasourceHealthy(context.Background(), name, healthy)
		}

		// Periodic-ping degradation:
		//
		// When the metric-only ping path runs (reconnectAttempted=false,
		// metrics!=nil) and the ping reports failure on a previously
		// Available datasource, we MUST flip Status to Degraded so the
		// next loop's needsHealing check picks it up. Without this flip
		// ds.Status stays Available, needsHealing returns false next
		// loop, and the healing path NEVER triggers — every subsequent
		// ping fails silently.
		//
		// Mirrors the reconnectAttempted+!healthy branch above.
		if !reconnectAttempted && pinged && !healthy && ds.Status == libConstants.DataSourceStatusAvailable {
			hc.logger.Log(context.Background(), log.LevelWarn, "Periodic ping failed on healthy datasource - marking degraded",
				log.String("datasource", name),
			)

			ds.Status = libConstants.DataSourceStatusDegraded
			ds.LastError = errors.New("periodic ping reported unhealthy")

			hc.mu.Lock()
			(*hc.dataSources)[name] = ds
			hc.mu.Unlock()
		}
	}

	if unavailableCount > 0 {
		hc.logger.Log(context.Background(), log.LevelDebug, "Health check complete", log.Int("needed_healing", unavailableCount), log.Int("reconnected", reconnectedCount))
	} else {
		hc.logger.Log(context.Background(), log.LevelDebug, "All datasources healthy")
	}
}

// timedPing performs a single bounded-context ping against the datasource
// and returns whether the probe succeeded plus the elapsed duration. It
// is the single source of truth for "is this datasource reachable?" inside
// performHealthChecks — both the reconnection state machine and the
// OTel emitter consume the same result, so each loop iteration produces
// at most one probe per datasource.
func (hc *HealthChecker) timedPing(ctx context.Context, name string, ds *DataSource) (bool, time.Duration) {
	pingCtx, cancel := context.WithTimeout(ctx, constant.HealthCheckTimeout)
	defer cancel()

	start := time.Now()
	healthy := hc.pingDataSource(pingCtx, name, ds)
	elapsed := time.Since(start)

	return healthy, elapsed
}

// needsHealing determines if a datasource needs reconnection attempt
func (hc *HealthChecker) needsHealing(name string, ds DataSource) bool {
	// Datasource is unavailable
	if ds.Status == libConstants.DataSourceStatusUnavailable {
		return true
	}

	// Datasource has degraded after a failed periodic ping. Without
	// this branch a Degraded datasource — produced by the metric-only
	// ping path in performHealthChecks — would never be re-probed by
	// the healing path, so it would stay Degraded forever.
	if ds.Status == libConstants.DataSourceStatusDegraded {
		return true
	}

	// Datasource is not initialized
	if !ds.Initialized {
		return true
	}

	// Circuit breaker is open (datasource is unhealthy)
	if !hc.circuitBreakerManager.IsHealthy(name) {
		cbState := hc.circuitBreakerManager.GetState(name)
		if cbState == constant.CircuitBreakerStateOpen {
			return true
		}
	}

	return false
}

// reestablishConnection re-runs ConnectToDataSource without the inner
// verification ping. Returns nil on success (the underlying repository
// handle is rebuilt and the dependency is ready to be probed by the
// caller's single timedPing call). Returns the connect error on failure;
// the caller resolves the resulting datasource state.
//
// This helper exists so performHealthChecks can run AT MOST ONE ping per
// loop iteration: connection re-establishment is decoupled from the
// readiness probe that follows it.
func (hc *HealthChecker) reestablishConnection(name string, ds *DataSource) error {
	ctx, cancel := context.WithTimeout(context.Background(), constant.HealthCheckTimeout)
	defer cancel()

	hc.logger.Log(ctx, log.LevelDebug, "Attempting reconnection to datasource", log.String("datasource", name))

	// Create a temporary map for ConnectToDataSource
	tempMap := make(map[string]DataSource)
	tempMap[name] = *ds

	// Reset retry count before attempting reconnection
	ds.RetryCount = 0
	ds.LastAttempt = time.Now()

	// Attempt connection (single attempt, no retry loop)
	if err := ConnectToDataSource(ctx, name, ds, hc.logger, tempMap); err != nil {
		hc.logger.Log(ctx, log.LevelError, "Failed to reconnect datasource", log.String("datasource", name), log.Err(err))
		return err
	}

	return nil
}

// attemptReconnection tries to reconnect to a datasource and verify the
// connection with a follow-up ping. Returns true only if both the
// reconnection AND the verification ping succeed.
//
// As of Dispatch 2 (single-probe-per-iteration fix), performHealthChecks
// no longer calls this method — the loop body runs reestablishConnection
// + a single timedPing instead, removing the duplicate probe. This method
// is preserved for tests that rely on the legacy "did we reconnect AND
// verify?" semantics, and as the documented compatibility path for any
// future caller that needs the combined operation.
//
//nolint:unused // exercised by health-checker_test.go (legacy semantics)
func (hc *HealthChecker) attemptReconnection(name string, ds *DataSource) bool {
	if err := hc.reestablishConnection(name, ds); err != nil {
		ds.Status = libConstants.DataSourceStatusUnavailable
		ds.LastError = err

		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), constant.HealthCheckTimeout)
	defer cancel()

	// Verify connectivity with a ping
	if !hc.pingDataSource(ctx, name, ds) {
		hc.logger.Log(ctx, log.LevelError, "Reconnection succeeded but ping failed", log.String("datasource", name))

		ds.Status = libConstants.DataSourceStatusDegraded

		return false
	}

	// Update datasource status
	ds.Status = libConstants.DataSourceStatusAvailable
	ds.Initialized = true
	ds.LastError = nil

	return true
}

// pingDataSource performs a real lightweight ping to verify datasource
// connectivity (SELECT 1 for PostgreSQL, db.runCommand({ping:1}) for
// MongoDB).
//
// Replaces the previous implementation which called GetDatabaseSchema —
// that was misleadingly commented as "lightweight" but actually performed
// full information_schema scans (PostgreSQL) and per-collection schema
// inference (MongoDB), causing ~5s of per-DS load per 30s health-check
// cycle. The schema-fetch is still used elsewhere (report generation,
// schema validation), but is no longer invoked from this hot loop.
//
// Latency target: ~5ms per datasource (down from ~5s for MongoDB).
func (hc *HealthChecker) pingDataSource(ctx context.Context, name string, ds *DataSource) bool {
	switch ds.DatabaseType {
	case PostgreSQLType:
		if ds.PostgresRepository == nil {
			return false
		}

		if err := ds.PostgresRepository.Ping(ctx); err != nil {
			hc.logger.Log(ctx, log.LevelDebug, "PostgreSQL ping failed",
				log.String("datasource", name), log.Err(err))

			return false
		}

		return true

	case MongoDBType:
		if ds.MongoDBRepository == nil {
			return false
		}

		if err := ds.MongoDBRepository.Ping(ctx); err != nil {
			hc.logger.Log(ctx, log.LevelDebug, "MongoDB ping failed",
				log.String("datasource", name), log.Err(err))

			return false
		}

		return true

	default:
		hc.logger.Log(ctx, log.LevelWarn, "Unknown database type for datasource", log.String("datasource", name), log.String("type", ds.DatabaseType))
		return false
	}
}

// GetHealthStatus returns the current health status of all datasources
func (hc *HealthChecker) GetHealthStatus() map[string]string {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	status := make(map[string]string)

	for name, ds := range *hc.dataSources {
		cbState := hc.circuitBreakerManager.GetState(name)
		status[name] = ds.Status + " (CB: " + cbState + ")"
	}

	return status
}
