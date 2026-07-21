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

func TestRedisPingerAdapter(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newRedisPinger(nil), "nil client must yield a nil in.RedisPinger so the probe reports not-established")

	var adapter *redisPingerAdapter

	assert.ErrorIs(t, adapter.Ping(context.Background()), in.ErrRedisConnectionNotEstablished,
		"nil adapter must report connection-not-established, not panic")

	assert.ErrorIs(t, (&redisPingerAdapter{}).Ping(context.Background()), in.ErrRedisConnectionNotEstablished,
		"nil underlying client must report connection-not-established")
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

func TestStreamingHealthProber_NotWired(t *testing.T) {
	t.Parallel()

	assert.Nil(t, newStreamingHealthProber(nil),
		"nil emitter must yield a nil in.StreamingHealthProber so the probe reports down")

	prober := &streamingHealthProber{emitter: nil}

	status, probeErr := prober.Probe(context.Background())
	assert.Equal(t, in.StatusDown, status)
	assert.ErrorIs(t, probeErr, errStreamingNotWired)
}
