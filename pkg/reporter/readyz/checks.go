// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"context"
	"time"

	mongoDB "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/redact"
	libRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/storage"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
)

// Per-dependency probe timeouts. The contract specifies short, bounded
// budgets so a single slow dependency cannot hold up the readiness probe
// past Kubernetes' own probe interval (typically 5s). Each value is the
// absolute upper bound on a single probe call.
const (
	// CheckTimeoutMongo is the per-probe budget for MongoDB.
	CheckTimeoutMongo = 2 * time.Second

	// CheckTimeoutRabbitMQ is the per-probe budget for RabbitMQ.
	CheckTimeoutRabbitMQ = 2 * time.Second

	// CheckTimeoutRedis is the per-probe budget for Redis/Valkey. Cache
	// dependencies should respond fastest; budget is tighter.
	CheckTimeoutRedis = 1 * time.Second

	// CheckTimeoutStorage is the per-probe budget for S3-compatible storage.
	CheckTimeoutStorage = 2 * time.Second

	// CheckTimeoutTenantManager is the per-probe budget for the Tenant
	// Manager dependency. Currently unused because the checker performs
	// only a nil-check, but defined for symmetry with other dependencies.
	CheckTimeoutTenantManager = 2 * time.Second

	// reasonMultiTenant is the reason string used for Mongo/RabbitMQ in
	// multi-tenant mode where per-tenant probing is not yet implemented.
	reasonMultiTenant = "multi-tenant mode: per-tenant probing not implemented (deferred to future Gate 6)"

	// reasonMultiTenantDisabled is the reason string when the Tenant Manager
	// dependency is skipped because MULTI_TENANT_ENABLED=false.
	reasonMultiTenantDisabled = "MULTI_TENANT_ENABLED=false"

	// reasonRedisOptionalUnused is the reason string when Redis is not
	// configured AND not required by the current service configuration. The
	// Worker uses Redis only when MULTI_TENANT_ENABLED=true (per-tenant Redis
	// client + tenant event-listener); in single-tenant mode a nil
	// RedisConnection is the expected steady state, NOT an error condition.
	reasonRedisOptionalUnused = "redis not used in this configuration (single-tenant mode, MULTI_TENANT_ENABLED=false)"

	// readinessProbeKey is the synthetic S3 key used to exercise the storage
	// API path during readiness checks. It is intentionally unlikely to
	// exist; the goal is to confirm the bucket/endpoint is reachable.
	readinessProbeKey = ".readiness-check"

	nameMongo         = "mongo"
	nameRabbitMQ      = "rabbitmq"
	nameRedis         = "redis"
	nameStorage       = "storage"
	nameTenantManager = "tenant_manager"
)

// boolPtr is a small helper used to populate the optional TLS pointer field
// on DependencyCheck. Returning a pointer rather than the bare bool lets us
// distinguish "not detected" (nil) from "explicitly false" in the wire format.
func boolPtr(b bool) *bool {
	return &b
}

// detectedTLSPtr returns a *bool only when the TLS-detection call returned a
// nil error, preserving the documented tri-state contract on
// DependencyCheck.TLS:
//
//	true  → confirmed TLS
//	false → confirmed non-TLS
//	nil   → unknown (e.g. malformed or unparseable URI)
//
// Callers MUST use this helper instead of `boolPtr(tlsOn)` whenever the
// detection function returns (bool, error). The previous pattern of
// discarding the error with `tlsOn, _ :=` and unconditionally wrapping the
// bool would misreport an unknown TLS posture as "confirmed false".
func detectedTLSPtr(detected bool, err error) *bool {
	if err != nil {
		return nil
	}

	return boolPtr(detected)
}

// ----------------------------------------------------------------------------
// MongoChecker
// ----------------------------------------------------------------------------

// MongoChecker probes a MongoDB connection by calling Ping on the underlying
// client. In multi-tenant mode the checker reports StatusNA because the
// service does not maintain a static connection — per-tenant probing is
// deferred to a future gate.
type MongoChecker struct {
	conn               *mongoDB.MongoConnection
	multiTenantEnabled bool
	uri                string
}

// NewMongoChecker constructs a MongoChecker. When multiTenantEnabled is true,
// the checker reports n/a regardless of the connection state.
func NewMongoChecker(conn *mongoDB.MongoConnection, multiTenantEnabled bool) *MongoChecker {
	uri := ""
	if conn != nil {
		uri = conn.ConnectionStringSource
	}

	return &MongoChecker{conn: conn, multiTenantEnabled: multiTenantEnabled, uri: uri}
}

// Name returns the JSON key used in the /readyz response.
func (c *MongoChecker) Name() string { return nameMongo }

// Check runs a Mongo Ping with a bounded timeout.
func (c *MongoChecker) Check(ctx context.Context) DependencyCheck {
	if c.multiTenantEnabled {
		return DependencyCheck{Status: StatusNA, Reason: reasonMultiTenant}
	}

	if c.conn == nil {
		return DependencyCheck{Status: StatusDown, Error: "connection not configured"}
	}

	tlsField := detectedTLSPtr(DetectMongoTLS(c.uri))

	probeCtx, cancel := context.WithTimeout(ctx, CheckTimeoutMongo)
	defer cancel()

	start := time.Now()

	db, err := c.conn.GetDB(probeCtx)
	if err != nil {
		ms, d := latencyFields(start)

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: ms,
			Latency:   d,
			TLS:       tlsField,
			Error:     redact.Error(err),
		}
	}

	if err = db.Ping(probeCtx, nil); err != nil {
		ms, d := latencyFields(start)

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: ms,
			Latency:   d,
			TLS:       tlsField,
			Error:     redact.Error(err),
		}
	}

	ms, d := latencyFields(start)

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: ms,
		Latency:   d,
		TLS:       tlsField,
	}
}

// ----------------------------------------------------------------------------
// RabbitMQChecker
// ----------------------------------------------------------------------------

// rabbitConn captures the minimal subset of *libRabbitmq.RabbitMQConnection
// the checker needs. Defining it here lets tests inject a stub that can
// simulate a HealthCheck() call that hangs — important because the real
// type's HealthCheck() is in-memory today but a future change could add
// network I/O, and the checker MUST honor its ctx budget regardless.
type rabbitConn interface {
	connectionState() (connected bool, closed bool)
	healthCheck() (bool, error)
}

// realRabbitConn adapts *libRabbitmq.RabbitMQConnection to rabbitConn.
type realRabbitConn struct {
	inner *libRabbitmq.RabbitMQConnection
}

func (r realRabbitConn) connectionState() (bool, bool) {
	return r.inner.Connected,
		r.inner.Connection == nil || r.inner.Connection.IsClosed()
}
func (r realRabbitConn) healthCheck() (bool, error) { return r.inner.HealthCheck() }

// RabbitMQChecker probes a RabbitMQ connection by inspecting the cached
// connection state and invoking the lib-commons HealthCheck helper. In
// multi-tenant mode the checker reports StatusNA because connections are
// per-tenant and managed by tmrabbitmq.Manager.
type RabbitMQChecker struct {
	conn               rabbitConn
	hasConn            bool
	multiTenantEnabled bool
	uri                string
}

// NewRabbitMQChecker constructs a RabbitMQChecker.
func NewRabbitMQChecker(conn *libRabbitmq.RabbitMQConnection, multiTenantEnabled bool) *RabbitMQChecker {
	uri := ""
	c := &RabbitMQChecker{multiTenantEnabled: multiTenantEnabled}

	if conn != nil {
		uri = conn.ConnectionStringSource
		c.conn = realRabbitConn{inner: conn}
		c.hasConn = true
	}

	c.uri = uri

	return c
}

// Name returns the JSON key used in the /readyz response.
func (c *RabbitMQChecker) Name() string { return nameRabbitMQ }

// Check runs the RabbitMQ readiness probe.
//
// As of Dispatch 2 (defensive ctx-aware timeout), the HealthCheck call is
// wrapped in a goroutine that respects the per-probe budget defined by
// CheckTimeoutRabbitMQ. lib-commons HealthCheck is in-memory today but
// could grow network I/O in a future release; the wrapper guarantees that
// /readyz never exceeds its own SLA regardless.
func (c *RabbitMQChecker) Check(ctx context.Context) DependencyCheck {
	if c.multiTenantEnabled {
		return DependencyCheck{Status: StatusNA, Reason: reasonMultiTenant}
	}

	if !c.hasConn {
		return DependencyCheck{Status: StatusDown, Error: "connection not configured"}
	}

	tlsField := detectedTLSPtr(DetectAMQPTLS(c.uri))

	start := time.Now()

	connected, closed := c.conn.connectionState()
	if !connected || closed {
		ms, d := latencyFields(start)

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: ms,
			Latency:   d,
			TLS:       tlsField,
			Error:     "connection is closed",
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, CheckTimeoutRabbitMQ)
	defer cancel()

	type healthResult struct {
		ok  bool
		err error
	}

	done := make(chan healthResult, 1)

	go func() {
		ok, err := c.conn.healthCheck()
		done <- healthResult{ok: ok, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			ms, d := latencyFields(start)

			return DependencyCheck{
				Status:    StatusDown,
				LatencyMs: ms,
				Latency:   d,
				TLS:       tlsField,
				Error:     redact.Error(r.err),
			}
		}

		if !r.ok {
			ms, d := latencyFields(start)

			return DependencyCheck{
				Status:    StatusDown,
				LatencyMs: ms,
				Latency:   d,
				TLS:       tlsField,
				Error:     "rabbitmq health check returned not-ok",
			}
		}

		ms, d := latencyFields(start)

		return DependencyCheck{
			Status:    StatusUp,
			LatencyMs: ms,
			Latency:   d,
			TLS:       tlsField,
		}

	case <-probeCtx.Done():
		// HealthCheck hung past the per-probe budget. The spawned
		// goroutine remains live until HealthCheck returns; this is
		// acceptable because the buffered done channel ensures it can
		// always finish without blocking on the receiver.
		ms, d := latencyFields(start)

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: ms,
			Latency:   d,
			TLS:       tlsField,
			Error:     "rabbitmq health check timed out",
		}
	}
}

// ----------------------------------------------------------------------------
// RedisChecker
// ----------------------------------------------------------------------------

// RedisChecker probes a Redis/Valkey connection by issuing a PING command.
//
// The required flag distinguishes services where Redis is mandatory (the
// Manager — used by rate-limiter, idempotency, multi-tenant cache) from
// services where Redis is conditional on feature flags (the Worker — used
// only when MULTI_TENANT_ENABLED=true for the per-tenant Redis client and
// tenant event-listener).
//
// When required=false and conn=nil the checker reports StatusSkipped with
// a reason instead of StatusDown, so the canonical aggregation rule
// (skipped counts as healthy) does not falsely fail /readyz, the startup
// self-probe, or the Docker healthcheck for a Worker that is correctly
// configured to run without Redis.
type RedisChecker struct {
	conn     *libRedis.RedisConnection
	required bool
}

// NewRedisChecker constructs a RedisChecker. Pass required=true on services
// that always need Redis (Manager). Pass required=false on services where
// Redis is conditional on feature flags (Worker).
func NewRedisChecker(conn *libRedis.RedisConnection, required bool) *RedisChecker {
	return &RedisChecker{conn: conn, required: required}
}

// Name returns the JSON key used in the /readyz response.
func (c *RedisChecker) Name() string { return nameRedis }

// Check runs the Redis readiness probe.
func (c *RedisChecker) Check(ctx context.Context) DependencyCheck {
	if c.conn == nil {
		if c.required {
			return DependencyCheck{Status: StatusDown, Error: "connection not configured"}
		}

		return DependencyCheck{Status: StatusSkipped, Reason: reasonRedisOptionalUnused}
	}

	tlsOn := c.conn.UseTLS

	probeCtx, cancel := context.WithTimeout(ctx, CheckTimeoutRedis)
	defer cancel()

	start := time.Now()

	client, err := c.conn.GetClient(probeCtx)
	if err != nil {
		ms, d := latencyFields(start)

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: ms,
			Latency:   d,
			TLS:       boolPtr(tlsOn),
			Error:     redact.Error(err),
		}
	}

	if _, err = client.Ping(probeCtx).Result(); err != nil {
		ms, d := latencyFields(start)

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: ms,
			Latency:   d,
			TLS:       boolPtr(tlsOn),
			Error:     redact.Error(err),
		}
	}

	ms, d := latencyFields(start)

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: ms,
		Latency:   d,
		TLS:       boolPtr(tlsOn),
	}
}

// ----------------------------------------------------------------------------
// StorageChecker
// ----------------------------------------------------------------------------

// StorageChecker probes an S3-compatible bucket by issuing an Exists call
// against a synthetic key. The key is unlikely to exist; we only care that
// the API path is reachable.
type StorageChecker struct {
	client   storage.ObjectStorage
	endpoint string
}

// NewStorageChecker constructs a StorageChecker. The endpoint argument is
// used only to populate the TLS field via DetectS3TLS — it does not affect
// the probe itself.
func NewStorageChecker(client storage.ObjectStorage, endpoint string) *StorageChecker {
	return &StorageChecker{client: client, endpoint: endpoint}
}

// Name returns the JSON key used in the /readyz response.
func (c *StorageChecker) Name() string { return nameStorage }

// Check runs the storage readiness probe.
func (c *StorageChecker) Check(ctx context.Context) DependencyCheck {
	if c.client == nil {
		return DependencyCheck{Status: StatusDown, Error: "storage client not configured"}
	}

	tlsField := detectedTLSPtr(DetectS3TLS(c.endpoint))

	probeCtx, cancel := context.WithTimeout(ctx, CheckTimeoutStorage)
	defer cancel()

	start := time.Now()

	if _, err := c.client.Exists(probeCtx, readinessProbeKey); err != nil {
		ms, d := latencyFields(start)

		return DependencyCheck{
			Status:    StatusDown,
			LatencyMs: ms,
			Latency:   d,
			TLS:       tlsField,
			Error:     redact.Error(err),
		}
	}

	ms, d := latencyFields(start)

	return DependencyCheck{
		Status:    StatusUp,
		LatencyMs: ms,
		Latency:   d,
		TLS:       tlsField,
	}
}

// ----------------------------------------------------------------------------
// TenantManagerChecker
// ----------------------------------------------------------------------------

// TenantManagerChecker reports whether the Tenant Manager client is
// configured. Per the existing decision recorded at
// init_tenant.go:117-132, this is a nil-check rather than an HTTP probe —
// an HTTP probe caused WARN-level "tenant not found" log spam every probe
// cycle. If the Tenant Manager becomes unavailable at runtime the circuit
// breaker on the client will trip and fail-fast on real requests.
//
// When MULTI_TENANT_ENABLED=false the checker reports StatusSkipped.
type TenantManagerChecker struct {
	multiTenantEnabled bool
	clientConfigured   func() bool
}

// NewTenantManagerChecker constructs a TenantManagerChecker. The
// clientConfigured predicate is consulted only when multiTenantEnabled=true;
// it should report true when the Tenant Manager client was successfully
// constructed at bootstrap. A nil predicate is treated as "not configured".
func NewTenantManagerChecker(multiTenantEnabled bool, clientConfigured func() bool) *TenantManagerChecker {
	return &TenantManagerChecker{
		multiTenantEnabled: multiTenantEnabled,
		clientConfigured:   clientConfigured,
	}
}

// Name returns the JSON key used in the /readyz response.
func (c *TenantManagerChecker) Name() string { return nameTenantManager }

// Check returns the readiness state of the Tenant Manager dependency.
func (c *TenantManagerChecker) Check(_ context.Context) DependencyCheck {
	if !c.multiTenantEnabled {
		return DependencyCheck{Status: StatusSkipped, Reason: reasonMultiTenantDisabled}
	}

	if c.clientConfigured == nil || !c.clientConfigured() {
		return DependencyCheck{Status: StatusDown, Error: "tenant manager client not configured"}
	}

	return DependencyCheck{Status: StatusUp}
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// latencyFields returns the (LatencyMs, Latency) pair derived from start.
// LatencyMs is the rounded ms value used in the JSON wire format; Latency
// is the unrounded time.Duration used by metric emission so sub-ms probes
// don't bottom out the histogram (test-reviewer H2). Computed in a single
// time.Since call to ensure the two fields are perfectly consistent.
func latencyFields(start time.Time) (int64, time.Duration) {
	d := time.Since(start)

	return d.Milliseconds(), d
}
