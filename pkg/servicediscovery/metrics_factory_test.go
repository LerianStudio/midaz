// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"
	"errors"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// newTestMetricsFactory builds a real *metrics.MetricsFactory backed by an
// in-memory OTel meter provider (no exporter), mirroring the repo's bootstrap
// tests. It lets the factory-backed recorder exercise the real Counter/Histogram
// paths without a live telemetry pipeline.
func newTestMetricsFactory(t *testing.T) *metrics.MetricsFactory {
	t.Helper()

	mp := sdkmetric.NewMeterProvider()
	meter := mp.Meter("servicediscovery_test")

	factory, err := metrics.NewMetricsFactory(meter, nil)
	if err != nil {
		t.Fatalf("NewMetricsFactory returned error: %v", err)
	}

	return factory
}

func TestNewMetricsFactoryRecorder_NilFactoryReturnsNop(t *testing.T) {
	r := NewMetricsFactoryRecorder(nil, libLog.NewNop())
	if r == nil {
		t.Fatal("NewMetricsFactoryRecorder(nil, ...) returned nil; want non-nil no-op recorder")
	}

	if _, ok := r.(NopMetricsRecorder); !ok {
		t.Fatalf("NewMetricsFactoryRecorder(nil, ...) returned %T; want NopMetricsRecorder", r)
	}

	// The returned no-op recorder must be callable without panic.
	ctx := context.Background()
	r.RegisterInitiated(ctx)
	r.DeregisterResult(ctx, ResultError)
	r.ResolveResult(ctx, "plugin-auth", ResultFallback, 3)
}

func TestNewMetricsFactoryRecorder_NonNilFactoryImplementsRecorder(t *testing.T) {
	factory := newTestMetricsFactory(t)

	r := NewMetricsFactoryRecorder(factory, libLog.NewNop())
	if r == nil {
		t.Fatal("NewMetricsFactoryRecorder(factory, ...) returned nil; want a recorder")
	}

	if _, ok := r.(NopMetricsRecorder); ok {
		t.Fatal("NewMetricsFactoryRecorder(factory, ...) returned NopMetricsRecorder; want the factory-backed recorder")
	}
}

func TestMetricsFactoryRecorder_MethodsRecordWithoutPanic(t *testing.T) {
	factory := newTestMetricsFactory(t)
	r := NewMetricsFactoryRecorder(factory, libLog.NewNop())
	ctx := context.Background()

	// All three interface methods must record against the real factory and
	// return normally (recording MUST NOT affect the caller).
	r.RegisterInitiated(ctx)
	r.DeregisterResult(ctx, ResultOK)
	r.DeregisterResult(ctx, ResultError)
	r.ResolveResult(ctx, "plugin-auth", ResultResolved, 12)
	r.ResolveResult(ctx, "plugin-auth", ResultFallback, 0)
	r.ResolveResult(ctx, "plugin-auth", ResultError, 999)
}

func TestMetricsFactoryRecorder_NilLoggerDoesNotPanic(t *testing.T) {
	factory := newTestMetricsFactory(t)

	// A nil logger must not cause a panic on the happy path (the factory paths
	// succeed, so the logger is never invoked). This guards the construction
	// contract without depending on unreachable error branches.
	r := NewMetricsFactoryRecorder(factory, nil)
	ctx := context.Background()

	r.RegisterInitiated(ctx)
	r.DeregisterResult(ctx, ResultOK)
	r.ResolveResult(ctx, "plugin-auth", ResultResolved, 5)
}

func TestMetricsFactoryRecorder_SatisfiesInterface(t *testing.T) {
	var _ MetricsRecorder = (*metricsFactoryRecorder)(nil)
}

// TestMetricsFactoryRecorder_Warn exercises the Warn logging helper directly. The
// counter/histogram builder error branches are unreachable with a valid factory
// (factory.Counter/Histogram only error on meter-creation failure), so warn is
// tested via its own reachable branches: nil logger (no-op) and real logger.
func TestMetricsFactoryRecorder_Warn(t *testing.T) {
	factory := newTestMetricsFactory(t)
	ctx := context.Background()
	err := errors.New("boom")

	// Nil logger must short-circuit without panic.
	nilLoggerRec := &metricsFactoryRecorder{factory: factory, logger: nil}
	nilLoggerRec.warn(ctx, "should be dropped", err)

	// Real logger must accept the Warn call without panic.
	realLoggerRec := &metricsFactoryRecorder{factory: factory, logger: libLog.NewNop()}
	realLoggerRec.warn(ctx, "recorded at warn", err)
}

func TestSDMetricDescriptors(t *testing.T) {
	tests := []struct {
		name     string
		gotName  string
		gotUnit  string
		wantName string
		wantUnit string
	}{
		{
			name:     "register_total",
			gotName:  sdRegisterTotal.Name,
			gotUnit:  sdRegisterTotal.Unit,
			wantName: "sd_register_total",
			wantUnit: "1",
		},
		{
			name:     "deregister_total",
			gotName:  sdDeregisterTotal.Name,
			gotUnit:  sdDeregisterTotal.Unit,
			wantName: "sd_deregister_total",
			wantUnit: "1",
		},
		{
			name:     "resolve_total",
			gotName:  sdResolveTotal.Name,
			gotUnit:  sdResolveTotal.Unit,
			wantName: "sd_resolve_total",
			wantUnit: "1",
		},
		{
			name:     "resolve_duration_milliseconds",
			gotName:  sdResolveDurationMs.Name,
			gotUnit:  sdResolveDurationMs.Unit,
			wantName: "sd_resolve_duration_milliseconds",
			wantUnit: "ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.gotName != tt.wantName {
				t.Errorf("Name = %q; want %q", tt.gotName, tt.wantName)
			}

			if tt.gotUnit != tt.wantUnit {
				t.Errorf("Unit = %q; want %q", tt.gotUnit, tt.wantUnit)
			}
		})
	}
}
