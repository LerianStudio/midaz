// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	mongoDB "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	libRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// MongoChecker
// ----------------------------------------------------------------------------

func TestMongoChecker_Name(t *testing.T) {
	t.Parallel()

	c := NewMongoChecker(nil, false)
	assert.Equal(t, "mongo", c.Name())
}

func TestMongoChecker_MultiTenantEnabled_ReportsNA(t *testing.T) {
	t.Parallel()

	c := NewMongoChecker(&mongoDB.MongoConnection{}, true)

	got := c.Check(context.Background())
	assert.Equal(t, StatusNA, got.Status)
	assert.Contains(t, got.Reason, "multi-tenant")
	assert.Equal(t, int64(0), got.LatencyMs, "no probe should run when n/a")
}

func TestMongoChecker_NilConn_ReportsDown(t *testing.T) {
	t.Parallel()

	c := NewMongoChecker(nil, false)

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "connection not configured")
}

// TestMongoChecker_FailedConnection_ReportsDown verifies the Mongo failure
// path without performing real network I/O. The Mongo driver's
// options.ApplyURI("") returns a synchronous "error parsing uri" failure
// from base.NewClient — no DNS lookup, no TCP dial — so the test is
// deterministic across all environments (CI, Docker-in-Docker, restrictive
// firewalls). This is intentionally relied upon as a stub-equivalent: the
// driver contract for empty URIs is stable across mongo-driver versions.
//
// Documenting per test-reviewer H1: the mongo-driver synchronously rejects
// empty URIs before any network call, so this test is reliable without an
// explicit stub.
func TestMongoChecker_FailedConnection_ReportsDown(t *testing.T) {
	t.Parallel()

	// MongoConnection with no URI fails URI parsing — exercises the error
	// path synchronously without dialing.
	conn := &mongoDB.MongoConnection{ConnectionStringSource: ""}
	c := NewMongoChecker(conn, false)

	start := time.Now()
	got := c.Check(context.Background())
	elapsed := time.Since(start)

	assert.Equal(t, StatusDown, got.Status)
	assert.NotEmpty(t, got.Error)
	// Deterministic: must complete well under the 2s mongo budget.
	assert.Less(t, elapsed, 500*time.Millisecond,
		"empty-URI parse failure must not approach the dial timeout, got %s", elapsed)
}

// ----------------------------------------------------------------------------
// RabbitMQChecker
// ----------------------------------------------------------------------------

func TestRabbitMQChecker_Name(t *testing.T) {
	t.Parallel()

	c := NewRabbitMQChecker(nil, false)
	assert.Equal(t, "rabbitmq", c.Name())
}

func TestRabbitMQChecker_MultiTenantEnabled_ReportsNA(t *testing.T) {
	t.Parallel()

	c := NewRabbitMQChecker(&libRabbitmq.RabbitMQConnection{}, true)

	got := c.Check(context.Background())
	assert.Equal(t, StatusNA, got.Status)
	assert.Contains(t, got.Reason, "multi-tenant")
}

func TestRabbitMQChecker_NilConn_ReportsDown(t *testing.T) {
	t.Parallel()

	c := NewRabbitMQChecker(nil, false)

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "connection not configured")
}

func TestRabbitMQChecker_DisconnectedConn_ReportsDown(t *testing.T) {
	t.Parallel()

	// Connected=false signals the connection is not active.
	c := NewRabbitMQChecker(&libRabbitmq.RabbitMQConnection{Connected: false}, false)

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "connection is closed")
}

// TestStorageChecker_ExistsReturnsRedactedError verifies the StorageChecker
// surfaces the underlying error message (redacted) rather than a generic
// placeholder. This is the operator-visibility fix from Dispatch 2 / HIGH
// findings: replacing "storage connectivity check failed" with the actual
// reason ("connection refused", "tls: failed to verify certificate", etc).
func TestStorageChecker_ExistsReturnsRedactedError(t *testing.T) {
	t.Parallel()

	c := NewStorageChecker(&stubStorage{existsErr: errors.New("connection refused")}, "http://s3.local:8080")

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "connection refused",
		"underlying error must reach the operator-facing field")
	assert.NotContains(t, got.Error, "storage connectivity check failed",
		"the generic placeholder must be gone")
}

// stubRabbitConn lets us inject a controlled HealthCheck outcome — and
// timing — without instantiating a real *libRabbitmq.RabbitMQConnection.
// The checker treats any rabbitConn with connected=true and closed=false
// as a probe-eligible target.
//
// release is closed by the test to unblock any in-flight healthCheck
// call; this lets the goleak guard succeed on tests that exercise the
// timeout path (the goroutine spawned by RabbitMQChecker.Check needs a
// way to exit cleanly before the test binary terminates).
type stubRabbitConn struct {
	connected bool
	closed    bool
	release   <-chan struct{}
	hcOK      bool
	hcErr     error
}

func (s *stubRabbitConn) connectionState() (bool, bool) {
	return s.connected, s.closed
}

func (s *stubRabbitConn) healthCheck() (bool, error) {
	if s.release != nil {
		<-s.release
	}

	return s.hcOK, s.hcErr
}

// TestRabbitMQChecker_HealthCheckHangsBeyondBudget_ReportsTimeout verifies
// that when the underlying HealthCheck hangs past CheckTimeoutRabbitMQ,
// the checker reports StatusDown with a timeout error rather than
// blocking the /readyz handler. This is the defensive ctx-aware timeout
// added in Dispatch 2 — lib-commons HealthCheck is in-memory today but
// could grow network I/O in the future, and /readyz must honor its own
// budget regardless.
//
// We use a release channel so the test can unblock the spawned goroutine
// before TestMain's goleak guard runs. This is intentional: the
// production guarantee (the buffered done channel in Check ensures the
// goroutine can always finish writing its result) means the same pattern
// is leak-free in production once HealthCheck eventually returns; here
// we accelerate that with the release channel for deterministic tests.
func TestRabbitMQChecker_HealthCheckHangsBeyondBudget_ReportsTimeout(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	defer close(release)

	stub := &stubRabbitConn{
		connected: true,
		closed:    false,
		release:   release,
		hcOK:      true,
	}

	c := &RabbitMQChecker{conn: stub, hasConn: true, uri: ""}

	// Use a short ctx deadline so the timeout path is exercised quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	got := c.Check(ctx)
	elapsed := time.Since(start)

	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "timed out")
	// CheckTimeoutRabbitMQ is the upper bound; ctx deadline (50ms) wins
	// here because context.WithTimeout in Check() composes them and the
	// shorter wins. Allow a generous bound to avoid flakes.
	assert.Less(t, elapsed, 1*time.Second,
		"checker must return within the per-probe budget, got %s", elapsed)
}

// TestRabbitMQChecker_HealthCheckSucceeds_ReportsUp verifies the happy
// path with the new injection point still works.
func TestRabbitMQChecker_HealthCheckSucceeds_ReportsUp(t *testing.T) {
	t.Parallel()

	stub := &stubRabbitConn{connected: true, closed: false, hcOK: true}
	c := &RabbitMQChecker{conn: stub, hasConn: true, uri: "amqp://localhost:5672"}

	got := c.Check(context.Background())
	assert.Equal(t, StatusUp, got.Status)
	assert.Empty(t, got.Error)
}

// TestRabbitMQChecker_HealthCheckErrorRedacted verifies that a
// HealthCheck error (e.g., one that contains a connection URL) reaches
// the operator-facing Error field redacted.
func TestRabbitMQChecker_HealthCheckErrorRedacted(t *testing.T) {
	t.Parallel()

	leaky := errors.New(`Get "amqps://guest:guest@rabbit.internal:15672/api/healthchecks/node": dial tcp: connection refused`)
	stub := &stubRabbitConn{connected: true, closed: false, hcErr: leaky}
	c := &RabbitMQChecker{conn: stub, hasConn: true}

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "connection refused")
	assert.NotContains(t, got.Error, "guest:guest")
}

// ----------------------------------------------------------------------------
// RedisChecker
// ----------------------------------------------------------------------------

func TestRedisChecker_Name(t *testing.T) {
	t.Parallel()

	c := NewRedisChecker(nil, true)
	assert.Equal(t, "redis", c.Name())
}

// TestRedisChecker_NilConn_Required_ReportsDown verifies the Manager-style
// case: Redis is always required, so a nil connection is a hard failure
// that flips /readyz to 503.
func TestRedisChecker_NilConn_Required_ReportsDown(t *testing.T) {
	t.Parallel()

	c := NewRedisChecker(nil, true)

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "connection not configured")
	assert.Empty(t, got.Reason,
		"down is an error condition — Reason must be empty")
}

// TestRedisChecker_NilConn_Optional_ReportsSkipped verifies the Worker-style
// case: when Redis is only used conditionally (MULTI_TENANT_ENABLED=true) and
// that flag is not set, a nil connection is expected and MUST report
// StatusSkipped — not StatusDown — so the canonical aggregation rule (skipped
// counts as healthy) does not falsely fail /readyz / startup self-probe /
// Docker healthcheck.
//
// Regression test for the Worker self-probe false-positive where
// `MULTI_TENANT_ENABLED=false` produced
// `dep:"redis", status:"down", error:"connection not configured"` and
// caused Docker to mark the Worker container unhealthy.
func TestRedisChecker_NilConn_Optional_ReportsSkipped(t *testing.T) {
	t.Parallel()

	c := NewRedisChecker(nil, false)

	got := c.Check(context.Background())
	assert.Equal(t, StatusSkipped, got.Status,
		"optional Redis with no connection must be skipped, not down")
	assert.NotEmpty(t, got.Reason,
		"skipped status must carry a human-readable reason for operators")
	assert.Contains(t, got.Reason, "MULTI_TENANT_ENABLED",
		"reason must explain why Redis was deemed not required: %q", got.Reason)
	assert.NotContains(t, got.Reason, "FETCHER_ENABLED",
		"reason must not cite the removed FETCHER_ENABLED env var: %q", got.Reason)
	assert.Empty(t, got.Error,
		"skipped is not an error condition — Error must be empty")
}

// TestRedisChecker_UnconfiguredConnection_ReportsDown verifies the Redis
// failure path without dialing a real network endpoint. An empty Address
// slice causes libRedis.RedisConnection.GetClient to return a deterministic
// "redis address list is empty" error — exercising the same Status=down
// branch the real "unreachable host" path would, but synchronously and
// without depending on OS network behavior (Docker-in-Docker / restrictive
// firewalls can hold the dial open up to the per-probe timeout, which the
// previous "127.0.0.1:1" version did, making it both slow and flaky).
//
// Note: when conn is non-nil the `required` flag does not gate behavior —
// the connection was provided so we must probe it regardless. We pass
// required=true here for parity with the Manager wiring.
//
// Replaces the prior real-network probe (test-reviewer H1).
func TestRedisChecker_UnconfiguredConnection_ReportsDown(t *testing.T) {
	t.Parallel()

	// Empty Address → GetClient returns an error before any network I/O.
	conn := &libRedis.RedisConnection{Address: nil}
	c := NewRedisChecker(conn, true)

	start := time.Now()
	got := c.Check(context.Background())
	elapsed := time.Since(start)

	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "redis address list is empty",
		"underlying error must reach the operator-facing field")
	// Deterministic stub path: must complete well under the 1s redis budget
	// (no dialing). Generous bound to avoid CI flakes.
	assert.Less(t, elapsed, 200*time.Millisecond,
		"stubbed failure path must not wait for the dial timeout, got %s", elapsed)
}

// ----------------------------------------------------------------------------
// StorageChecker
// ----------------------------------------------------------------------------

func TestStorageChecker_Name(t *testing.T) {
	t.Parallel()

	c := NewStorageChecker(nil, "")
	assert.Equal(t, "storage", c.Name())
}

func TestStorageChecker_NilClient_ReportsDown(t *testing.T) {
	t.Parallel()

	c := NewStorageChecker(nil, "")

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "storage client not configured")
}

// stubStorage is a minimal storage.ObjectStorage implementation that lets
// the StorageChecker tests inject a controlled Exists outcome without
// pulling in gomock for what is essentially a one-method probe.
type stubStorage struct {
	existsErr error
	exists    bool
}

func (s *stubStorage) Upload(context.Context, string, io.Reader, string) (string, error) {
	return "", nil
}

func (s *stubStorage) UploadWithTTL(context.Context, string, io.Reader, string, string) (string, error) {
	return "", nil
}

func (s *stubStorage) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, nil
}

func (s *stubStorage) Delete(context.Context, string) error { return nil }

func (s *stubStorage) Exists(context.Context, string) (bool, error) {
	return s.exists, s.existsErr
}

func (s *stubStorage) GeneratePresignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func TestStorageChecker_HealthyExistsCall_ReportsUp(t *testing.T) {
	t.Parallel()

	// Exists succeeds (returning false is fine — readiness only checks
	// that the API path is reachable, not that the synthetic key exists).
	c := NewStorageChecker(&stubStorage{exists: false}, "https://s3.example.com")

	got := c.Check(context.Background())
	assert.Equal(t, StatusUp, got.Status)
	require.NotNil(t, got.TLS, "TLS pointer must be populated for storage")
	assert.True(t, *got.TLS, "https endpoint must be detected as TLS")
	assert.Empty(t, got.Error)
}

func TestStorageChecker_ExistsReturnsError_ReportsDown(t *testing.T) {
	t.Parallel()

	c := NewStorageChecker(&stubStorage{existsErr: errors.New("connection refused")}, "http://s3.local:8080")

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	// Operator-visibility: surface the real error, not a generic placeholder.
	assert.Contains(t, got.Error, "connection refused")
	require.NotNil(t, got.TLS)
	assert.False(t, *got.TLS, "http endpoint must be detected as non-TLS")
}

func TestStorageChecker_PopulatesLatency(t *testing.T) {
	t.Parallel()

	c := NewStorageChecker(&stubStorage{}, "https://s3.example.com")

	got := c.Check(context.Background())
	assert.GreaterOrEqual(t, got.LatencyMs, int64(0),
		"latency must be populated even on the success path")
}

// ----------------------------------------------------------------------------
// TenantManagerChecker
// ----------------------------------------------------------------------------

func TestTenantManagerChecker_Name(t *testing.T) {
	t.Parallel()

	c := NewTenantManagerChecker(false, nil)
	assert.Equal(t, "tenant_manager", c.Name())
}

func TestTenantManagerChecker_Disabled_Skipped(t *testing.T) {
	t.Parallel()

	c := NewTenantManagerChecker(false, nil)

	got := c.Check(context.Background())
	assert.Equal(t, StatusSkipped, got.Status)
	assert.Contains(t, got.Reason, "MULTI_TENANT_ENABLED=false")
}

func TestTenantManagerChecker_EnabledNilPredicate_Down(t *testing.T) {
	t.Parallel()

	c := NewTenantManagerChecker(true, nil)

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
	assert.Contains(t, got.Error, "not configured")
}

func TestTenantManagerChecker_EnabledNotConfigured_Down(t *testing.T) {
	t.Parallel()

	c := NewTenantManagerChecker(true, func() bool { return false })

	got := c.Check(context.Background())
	assert.Equal(t, StatusDown, got.Status)
}

func TestTenantManagerChecker_EnabledConfigured_Up(t *testing.T) {
	t.Parallel()

	c := NewTenantManagerChecker(true, func() bool { return true })

	got := c.Check(context.Background())
	assert.Equal(t, StatusUp, got.Status)
}

// ----------------------------------------------------------------------------
// TLS tri-state contract — DependencyCheck.TLS must distinguish:
//
//   - true  → confirmed TLS
//   - false → confirmed non-TLS
//   - nil   → unknown (e.g. malformed URI / detection error)
//
// The previous pattern of `tlsOn, _ := DetectXTLS(uri); &tlsOn` always set a
// non-nil pointer and reported an unknown TLS posture as explicit `false`.
// detectedTLSPtr fixes this: when the detection helper returns an error the
// TLS field stays nil so consumers can tell "we don't know" from "we know
// it's plaintext".
// ----------------------------------------------------------------------------

// TestStorageChecker_MalformedEndpoint_TLSNil verifies that an endpoint
// missing the http(s) scheme — DetectS3TLS returns an error in that case —
// produces a DependencyCheck with TLS == nil rather than a non-nil pointer
// to false.
func TestStorageChecker_MalformedEndpoint_TLSNil(t *testing.T) {
	t.Parallel()

	// A bare host:port without an http(s) scheme is rejected by
	// DetectS3TLS (the contract refuses to guess plaintext-vs-TLS).
	c := NewStorageChecker(&stubStorage{exists: false}, "s3.local:9000")

	got := c.Check(context.Background())
	assert.Nil(t, got.TLS,
		"unknown TLS posture must surface as nil, never as &false")
}

// TestStorageChecker_HTTPSEndpoint_TLSTrue verifies the positive path: a
// well-formed https:// endpoint surfaces TLS == &true.
func TestStorageChecker_HTTPSEndpoint_TLSTrue(t *testing.T) {
	t.Parallel()

	c := NewStorageChecker(&stubStorage{}, "https://s3.example.com")

	got := c.Check(context.Background())
	require.NotNil(t, got.TLS)
	assert.True(t, *got.TLS)
}

// TestStorageChecker_HTTPEndpoint_TLSFalse verifies the negative path: a
// well-formed http:// endpoint surfaces TLS == &false (confirmed non-TLS,
// distinct from nil).
func TestStorageChecker_HTTPEndpoint_TLSFalse(t *testing.T) {
	t.Parallel()

	c := NewStorageChecker(&stubStorage{}, "http://s3.local:8080")

	got := c.Check(context.Background())
	require.NotNil(t, got.TLS)
	assert.False(t, *got.TLS)
}

// TestDetectedTLSPtr verifies the tri-state helper directly. The helper is
// the centerpiece of the tri-state contract; testing it in isolation pins
// the behavior so future refactors of the call sites can rely on it.
func TestDetectedTLSPtr(t *testing.T) {
	t.Parallel()

	t.Run("error returns nil", func(t *testing.T) {
		t.Parallel()

		got := detectedTLSPtr(false, errors.New("malformed URI"))
		assert.Nil(t, got)
	})

	t.Run("error with detected=true also returns nil", func(t *testing.T) {
		t.Parallel()

		// The helper must NOT trust the bool when err != nil — even if a
		// detection function happens to return (true, err), the contract
		// is that an error means the TLS posture is unknown.
		got := detectedTLSPtr(true, errors.New("malformed URI"))
		assert.Nil(t, got)
	})

	t.Run("nil error with detected=true returns &true", func(t *testing.T) {
		t.Parallel()

		got := detectedTLSPtr(true, nil)
		require.NotNil(t, got)
		assert.True(t, *got)
	})

	t.Run("nil error with detected=false returns &false", func(t *testing.T) {
		t.Parallel()

		got := detectedTLSPtr(false, nil)
		require.NotNil(t, got)
		assert.False(t, *got)
	})
}
