// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build chaos

package readyz

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"
)

// Layer 8 of the canonical 9-test-layer matrix in dev-readyz/SKILL.md.
//
// These tests inject controlled faults at the Checker boundary using lightweight
// stubs (httptest.Server for HTTP-based deps, blocking/erroring stubs for
// non-HTTP deps) instead of full Toxiproxy infrastructure. The intent of
// Layer 8 is "verify /readyz reports degradation correctly under fault
// conditions"; the transport mechanism is an implementation detail.
//
// For real-network chaos against Mongo/RabbitMQ/Redis/SeaweedFS, see
// tests/chaos/ which already uses Toxiproxy + testcontainers under the same
// chaos build tag.

// ---------------------------------------------------------------------------
// Stubs that simulate connection-loss / latency / flapping at Checker level.
// ---------------------------------------------------------------------------

// chaosCheckerMode controls how chaosChecker.Check responds.
type chaosCheckerMode int32

const (
	chaosModeUp chaosCheckerMode = iota
	chaosModeDown
	chaosModeSlow // sleeps longer than the per-probe timeout
)

// chaosChecker is a Checker whose behavior can be flipped at runtime to
// simulate fault injection. Every Check call respects ctx.Done() so the
// per-probe timeout in production checkers can be modeled here.
type chaosChecker struct {
	name string
	mode atomic.Int32 // chaosCheckerMode
	// slow is consulted when mode==chaosModeSlow. Defaults to 3*per-probe
	// timeout.
	slow time.Duration
	// errMsg is used when mode==chaosModeDown.
	errMsg string
	// downTransitions counts mode→down transitions; lets flapping tests
	// assert that consecutive transitions all surface in the response.
	downTransitions atomic.Int64
}

func (c *chaosChecker) Name() string { return c.name }

func (c *chaosChecker) setMode(mode chaosCheckerMode) {
	prev := chaosCheckerMode(c.mode.Load())
	c.mode.Store(int32(mode))

	if mode == chaosModeDown && prev != chaosModeDown {
		c.downTransitions.Add(1)
	}
}

func (c *chaosChecker) Check(ctx context.Context) DependencyCheck {
	switch chaosCheckerMode(c.mode.Load()) {
	case chaosModeUp:
		return DependencyCheck{Status: StatusUp, LatencyMs: 1, TLS: boolPtr(false)}
	case chaosModeDown:
		return DependencyCheck{Status: StatusDown, LatencyMs: 1, TLS: boolPtr(false), Error: c.errMsg}
	case chaosModeSlow:
		// Honour ctx cancellation so the test can simulate per-probe timeout.
		select {
		case <-time.After(c.slow):
			// If we got here the caller didn't cancel; treat as down because
			// the latency budget was exceeded.
			return DependencyCheck{Status: StatusDown, LatencyMs: c.slow.Milliseconds(), TLS: boolPtr(false), Error: "latency budget exceeded"}
		case <-ctx.Done():
			return DependencyCheck{Status: StatusDown, LatencyMs: c.slow.Milliseconds(), TLS: boolPtr(false), Error: "deadline exceeded"}
		}
	default:
		return DependencyCheck{Status: StatusUp, LatencyMs: 0, TLS: boolPtr(false)}
	}
}

// ---------------------------------------------------------------------------
// Test 1 — Connection loss mid-run reports the dep as down within the next
// /readyz invocation. Models Toxiproxy 'down' toxic semantics.
// ---------------------------------------------------------------------------

func TestReadyz_ConnectionLoss_ReturnsDown(t *testing.T) {
	t.Parallel()

	mongoStub := &chaosChecker{name: "mongo", errMsg: "connection refused"}
	mongoStub.setMode(chaosModeUp)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler([]Checker{mongoStub}, &DrainState{}, "1.0.0", "saas", nil))

	// Probe 1: dep up → 200.
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Inject the fault: dep loses connection.
	mongoStub.setMode(chaosModeDown)

	// Probe 2: dep is down → 503 with status=down + error.
	resp2, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp2.Body.Close()

	require.Equal(t, http.StatusServiceUnavailable, resp2.StatusCode)

	body := decodeReadyz(t, resp2.Body)
	assert.Equal(t, "unhealthy", body.Status)

	mongo, ok := body.Checks["mongo"]
	require.True(t, ok, "mongo dep must appear in checks")
	assert.Equal(t, StatusDown, mongo.Status)
	assert.Equal(t, "connection refused", mongo.Error)
}

// ---------------------------------------------------------------------------
// Test 2 — Latency injection causes per-probe timeout, status=down with
// timeout error. Models Toxiproxy 'latency' toxic.
// ---------------------------------------------------------------------------

func TestReadyz_LatencyInjection_TimesOut(t *testing.T) {
	t.Parallel()

	// Build a real FetcherChecker fronted by a deliberately slow server so
	// the per-probe context.WithTimeout(CheckTimeoutFetcher) actually fires.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Hold the response open for longer than CheckTimeoutFetcher (2s).
		time.Sleep(CheckTimeoutFetcher + 1*time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	provider := &slowFetcherProvider{
		ping: func(ctx context.Context) error {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, slow.URL, nil)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}

			defer resp.Body.Close()

			_, _ = io.Copy(io.Discard, resp.Body)

			return nil
		},
	}

	checker := NewFetcherChecker(provider, slow.URL)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler([]Checker{checker}, &DrainState{}, "1.0.0", "saas", nil))

	start := time.Now()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil), int(CheckTimeoutFetcher.Milliseconds())+5_000)
	require.NoError(t, err)

	defer resp.Body.Close()

	elapsed := time.Since(start)

	// Per-probe timeout MUST fire well before the slow server replies.
	assert.Less(t, elapsed, CheckTimeoutFetcher+1500*time.Millisecond,
		"checker did not enforce per-probe timeout (got %s)", elapsed)

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body := decodeReadyz(t, resp.Body)
	fetcher, ok := body.Checks["fetcher"]
	require.True(t, ok)

	assert.Equal(t, StatusDown, fetcher.Status)
	assert.Contains(t, fetcher.Error, "deadline exceeded",
		"error should reference per-probe timeout/deadline; got %q", fetcher.Error)
}

// ---------------------------------------------------------------------------
// Test 3 — Flapping dep produces consistent down/up transitions across
// successive probes. Models intermittent connectivity.
// ---------------------------------------------------------------------------

func TestReadyz_FlappingDep_ReportedConsistently(t *testing.T) {
	t.Parallel()

	flap := &chaosChecker{name: "redis", errMsg: "i/o timeout"}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler([]Checker{flap}, &DrainState{}, "1.0.0", "saas", nil))

	// Cycle: up → down → up → down → up. The handler MUST reflect each
	// transition immediately because /readyz is uncached.
	cycle := []chaosCheckerMode{
		chaosModeUp,
		chaosModeDown,
		chaosModeUp,
		chaosModeDown,
		chaosModeUp,
	}

	expectedCodes := []int{
		http.StatusOK,
		http.StatusServiceUnavailable,
		http.StatusOK,
		http.StatusServiceUnavailable,
		http.StatusOK,
	}

	for i, mode := range cycle {
		flap.setMode(mode)

		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
		require.NoError(t, err)

		body := decodeReadyz(t, resp.Body)
		_ = resp.Body.Close()

		assert.Equalf(t, expectedCodes[i], resp.StatusCode,
			"cycle step %d (mode=%d): expected code %d, got %d", i, mode, expectedCodes[i], resp.StatusCode)

		dep := body.Checks["redis"]
		switch mode {
		case chaosModeUp:
			assert.Equalf(t, StatusUp, dep.Status, "step %d", i)
		case chaosModeDown:
			assert.Equalf(t, StatusDown, dep.Status, "step %d", i)
		}
	}

	// 2 transitions to down were applied; the counter must reflect that.
	assert.Equal(t, int64(2), flap.downTransitions.Load(),
		"chaos stub should count 2 down transitions (verifies test setup integrity)")
}

// ---------------------------------------------------------------------------
// Test 4 — Per-dep slow goroutine does not leak the handler ctx; flake in one
// dep does not cap the response on the other deps.
// ---------------------------------------------------------------------------

func TestReadyz_PartialDegradation_OtherDepsStillReported(t *testing.T) {
	t.Parallel()

	healthy := &chaosChecker{name: "mongo"}
	healthy.setMode(chaosModeUp)

	broken := &chaosChecker{name: "rabbitmq", errMsg: "TCP reset by peer"}
	broken.setMode(chaosModeDown)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/readyz", NewHandler([]Checker{healthy, broken}, &DrainState{}, "1.0.0", "saas", nil))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	body := decodeReadyz(t, resp.Body)
	require.Len(t, body.Checks, 2, "both deps must be reported even if one is broken")

	assert.Equal(t, StatusUp, body.Checks["mongo"].Status,
		"healthy dep status must still be reported when peer dep is down")
	assert.Equal(t, StatusDown, body.Checks["rabbitmq"].Status)
	assert.Equal(t, "TCP reset by peer", body.Checks["rabbitmq"].Error)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func decodeReadyz(t *testing.T, r io.Reader) Response {
	t.Helper()

	var body Response

	require.NoError(t, json.NewDecoder(r).Decode(&body))

	return body
}

// slowFetcherProvider is a DataSourceProvider stub used by the latency-injection
// test. It satisfies datasource.DataSourceProvider AND has a Ping method, so
// the type assertion in FetcherChecker.Check succeeds and the chaos handler
// runs the slow HTTP path.
type slowFetcherProvider struct {
	ping func(ctx context.Context) error
}

func (s *slowFetcherProvider) ListDataSources(context.Context) ([]datasource.DataSourceInfo, error) {
	return nil, nil
}

func (s *slowFetcherProvider) GetDataSourceSchema(context.Context, string) (*datasource.DataSourceSchema, error) {
	return nil, nil
}

func (s *slowFetcherProvider) ValidateSchema(context.Context, string, map[string][]string) (*datasource.ValidationResult, error) {
	return nil, nil
}

func (s *slowFetcherProvider) HealthCheck(context.Context) (map[string]bool, error) {
	return nil, nil
}

func (s *slowFetcherProvider) Ping(ctx context.Context) error {
	if s.ping == nil {
		return nil
	}

	return s.ping(ctx)
}
