// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
)

// fakeActiveTenantsGetter drives the tenant-manager probe classification with a
// controllable error, standing in for the real *tmclient.Client.
type fakeActiveTenantsGetter struct {
	err error
}

func (f fakeActiveTenantsGetter) GetActiveTenantsByService(_ context.Context, _ string) ([]*tmclient.TenantSummary, error) {
	return nil, f.err
}

// fakeEmitter drives the streaming probe classification with a controllable
// Healthy() error.
type fakeEmitter struct {
	healthyErr error
}

func (f fakeEmitter) Emit(_ context.Context, _ libStreaming.EmitRequest) error { return nil }
func (f fakeEmitter) Close() error                                             { return nil }
func (f fakeEmitter) Healthy(_ context.Context) error                          { return f.healthyErr }

// recordingRedisClient records whether Ping reached the external client. It
// embeds redis.UniversalClient so it satisfies the interface without stubbing
// every method; only Ping is overridden, and no other method is exercised.
type recordingRedisClient struct {
	redis.UniversalClient

	pingCalled bool
}

func (c *recordingRedisClient) Ping(ctx context.Context) *redis.StatusCmd {
	c.pingCalled = true

	return redis.NewStatusCmd(ctx)
}

// recordingTenantsGetter records whether the external tenant-manager call ran,
// so a short-circuit on a canceled context can be asserted.
type recordingTenantsGetter struct {
	called bool
	err    error
}

func (r *recordingTenantsGetter) GetActiveTenantsByService(_ context.Context, _ string) ([]*tmclient.TenantSummary, error) {
	r.called = true

	return nil, r.err
}

// recordingEmitter records whether Healthy reached the external emitter, so a
// short-circuit on a canceled context can be asserted.
type recordingEmitter struct {
	healthyErr    error
	healthyCalled bool
}

func (r *recordingEmitter) Emit(_ context.Context, _ libStreaming.EmitRequest) error { return nil }
func (r *recordingEmitter) Close() error                                             { return nil }
func (r *recordingEmitter) Healthy(_ context.Context) error {
	r.healthyCalled = true

	return r.healthyErr
}

func TestRedisPingerAdapter(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newRedisPinger(nil), "nil client must yield a nil in.RedisPinger so the probe reports not-established")

	var adapter *redisPingerAdapter

	assert.ErrorIs(t, adapter.Ping(context.Background()), in.ErrRedisConnectionNotEstablished,
		"nil adapter must report connection-not-established, not panic")

	assert.ErrorIs(t, (&redisPingerAdapter{}).Ping(context.Background()), in.ErrRedisConnectionNotEstablished,
		"nil underlying client must report connection-not-established")
}

// TestRedisPingerAdapter_CanceledContext asserts that a canceled context
// short-circuits before the external PING: the adapter returns the ctx error
// (probe layer maps it to down) and the underlying client is never touched.
func TestRedisPingerAdapter_CanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &recordingRedisClient{}
	adapter := &redisPingerAdapter{client: client}

	err := adapter.Ping(ctx)
	assert.ErrorIs(t, err, context.Canceled, "canceled context must be returned before the external PING")
	assert.False(t, client.pingCalled, "external PING must not run once the context is canceled")
}

func TestTenantManagerHealthProber_Probe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus string
	}{
		{name: "up_on_success", err: nil, wantStatus: in.StatusUp},
		{name: "degraded_on_circuit_breaker_open", err: tmcore.ErrCircuitBreakerOpen, wantStatus: in.StatusDegraded},
		{
			name:       "degraded_on_wrapped_circuit_breaker_open",
			err:        fmt.Errorf("get active tenants: %w", tmcore.ErrCircuitBreakerOpen),
			wantStatus: in.StatusDegraded,
		},
		{name: "down_on_other_error", err: errors.New("500 internal server error at https://tm.internal/x"), wantStatus: in.StatusDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prober := &tenantManagerHealthProber{client: fakeActiveTenantsGetter{err: tt.err}, service: "tracer"}

			status, probeErr := prober.Probe(context.Background())
			assert.Equal(t, tt.wantStatus, status)

			if tt.wantStatus == in.StatusDown {
				assert.ErrorIs(t, probeErr, errTenantManagerProbe,
					"down must return the generic sentinel, never the raw client error")
				assert.NotContains(t, probeErr.Error(), "tm.internal", "raw client detail must not leak even to the span")
			}
		})
	}
}

// TestTenantManagerHealthProber_ClassifiesByStatusClass asserts the B-decision:
// a reachable 4xx round-trip means the tenant-manager service IS up (only its
// answer is a client error), while 5xx / transport / read failures are "down".
// CB-open stays "degraded" and nil stays "up".
func TestTenantManagerHealthProber_ClassifiesByStatusClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus string
	}{
		{name: "nil_is_up", err: nil, wantStatus: in.StatusUp},
		{name: "circuit_breaker_open_is_degraded", err: tmcore.ErrCircuitBreakerOpen, wantStatus: in.StatusDegraded},
		{
			name:       "reachable_400_is_up",
			err:        fmt.Errorf("tenant manager returned status 400 for service tracer"),
			wantStatus: in.StatusUp,
		},
		{
			name:       "reachable_403_is_up",
			err:        fmt.Errorf("tenant manager returned status 403 for service tracer"),
			wantStatus: in.StatusUp,
		},
		{
			name:       "reachable_404_is_up",
			err:        fmt.Errorf("tenant manager returned status 404 for service tracer"),
			wantStatus: in.StatusUp,
		},
		{
			name:       "server_500_is_down",
			err:        fmt.Errorf("tenant manager returned status 500 for service tracer"),
			wantStatus: in.StatusDown,
		},
		{
			name:       "server_503_is_down",
			err:        fmt.Errorf("tenant manager returned status 503 for service tracer"),
			wantStatus: in.StatusDown,
		},
		{
			name:       "transport_failure_is_down",
			err:        fmt.Errorf("failed to execute request: %w", context.DeadlineExceeded),
			wantStatus: in.StatusDown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prober := &tenantManagerHealthProber{client: fakeActiveTenantsGetter{err: tt.err}, service: "tracer"}

			status, probeErr := prober.Probe(context.Background())
			assert.Equal(t, tt.wantStatus, status)

			switch tt.wantStatus {
			case in.StatusUp:
				assert.NoError(t, probeErr, "up must carry no error")
			case in.StatusDown:
				assert.ErrorIs(t, probeErr, errTenantManagerProbe,
					"down must return the generic sentinel, never the raw client error")
			}
		})
	}
}

// TestTenantManagerHealthProber_Probe_CanceledContext asserts that a canceled
// context short-circuits to down with the generic sentinel before the external
// tenant-manager call runs — preserving the "nothing sensitive on the wire/span"
// discipline (the raw ctx error is not surfaced).
func TestTenantManagerHealthProber_Probe_CanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	getter := &recordingTenantsGetter{}
	prober := &tenantManagerHealthProber{client: getter, service: "tracer"}

	status, probeErr := prober.Probe(ctx)
	assert.Equal(t, in.StatusDown, status)
	assert.ErrorIs(t, probeErr, errTenantManagerProbe,
		"canceled context must yield the generic down sentinel, never the raw ctx error")
	assert.False(t, getter.called, "external tenant-manager call must not run once the context is canceled")
}

func TestTenantManagerHealthProber_NotWired(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newTenantManagerHealthProber(nil, "tracer"),
		"nil client must yield a nil in.TenantManagerProber so the probe reports down")

	prober := &tenantManagerHealthProber{client: nil, service: "tracer"}

	status, probeErr := prober.Probe(context.Background())
	assert.Equal(t, in.StatusDown, status)
	assert.ErrorIs(t, probeErr, errTenantManagerNotWired)
}

func TestStreamingHealthProber_Probe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		healthyErr error
		wantStatus string
	}{
		{name: "up_on_nil", healthyErr: nil, wantStatus: in.StatusUp},
		{
			name:       "up_on_healthy_state_error",
			healthyErr: libStreaming.NewHealthError(libStreaming.Healthy, nil),
			wantStatus: in.StatusUp,
		},
		{
			name:       "degraded_on_degraded_state",
			healthyErr: libStreaming.NewHealthError(libStreaming.Degraded, errors.New("broker slow")),
			wantStatus: in.StatusDegraded,
		},
		{
			name:       "down_on_down_state",
			healthyErr: libStreaming.NewHealthError(libStreaming.Down, errors.New("broker unreachable")),
			wantStatus: in.StatusDown,
		},
		{name: "down_on_non_health_error", healthyErr: errors.New("unexpected"), wantStatus: in.StatusDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prober := &streamingHealthProber{emitter: fakeEmitter{healthyErr: tt.healthyErr}}

			status, _ := prober.Probe(context.Background())
			assert.Equal(t, tt.wantStatus, status)
		})
	}
}

// TestStreamingHealthProber_NonHealthErrorIsSanitized asserts that the
// unclassifiable-failure fallback (a non-*HealthError from Emitter.Healthy)
// does NOT surface the raw error onto the probe span — only a generic sentinel
// leaves the boundary, mirroring the tenant_manager prober.
func TestStreamingHealthProber_NonHealthErrorIsSanitized(t *testing.T) {
	t.Parallel()

	raw := errors.New("broker topology kafka-internal-9092 unreachable")
	prober := &streamingHealthProber{emitter: fakeEmitter{healthyErr: raw}}

	status, probeErr := prober.Probe(context.Background())

	assert.Equal(t, in.StatusDown, status)
	assert.ErrorIs(t, probeErr, errStreamingProbe,
		"non-HealthError fallback must return the generic sentinel, not the raw error")
	assert.NotContains(t, probeErr.Error(), "kafka-internal-9092",
		"raw broker/topology detail must never reach the probe span")
}

// TestStreamingHealthProber_Probe_CanceledContext asserts that a canceled
// context short-circuits to down with the generic sentinel before Emitter.Healthy
// runs — same sanitization discipline as the tenant_manager prober.
func TestStreamingHealthProber_Probe_CanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	emitter := &recordingEmitter{}
	prober := &streamingHealthProber{emitter: emitter}

	status, probeErr := prober.Probe(ctx)
	assert.Equal(t, in.StatusDown, status)
	assert.ErrorIs(t, probeErr, errStreamingProbe,
		"canceled context must yield the generic down sentinel")
	assert.False(t, emitter.healthyCalled, "external Healthy call must not run once the context is canceled")
}

func TestStreamingHealthProber_NotWired(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newStreamingHealthProber(nil),
		"nil emitter must yield a nil in.StreamingHealthProber so the probe reports down")

	prober := &streamingHealthProber{emitter: nil}

	status, probeErr := prober.Probe(context.Background())
	assert.Equal(t, in.StatusDown, status)
	assert.ErrorIs(t, probeErr, errStreamingNotWired)
}
