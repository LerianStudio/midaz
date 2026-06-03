// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/workers/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// newBenchSupervisor builds a WorkerSupervisor with the minimum wiring needed
// for the EnsureWorkers fast-path benchmark. Reimplemented (rather than
// reusing newSupervisorTestDeps) because the shared helper's signature takes
// *testing.T, and gomock.NewController typed on *testing.B is the cleaner
// path than casting.
func newBenchSupervisor(b *testing.B) *WorkerSupervisor {
	b.Helper()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	b.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	ctrl := gomock.NewController(b)

	syncRepo := mocks.NewMockRuleSyncRepository(ctrl)
	syncRepo.EXPECT().
		GetRulesUpdatedSince(gomock.Any(), gomock.Any()).
		Return([]*model.Rule{}, nil).
		AnyTimes()

	usageRepo := mocks.NewMockUsageCounterCleanupRepository(ctrl)
	usageRepo.EXPECT().
		DeleteExpiredCounters(gomock.Any(), gomock.Any()).
		Return(int64(0), nil).
		AnyTimes()

	compiler := mocks.NewMockExpressionCompiler(ctrl)
	compiler.EXPECT().
		Compile(gomock.Any(), gomock.Any()).
		Return("compiled", nil).
		AnyTimes()

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)
	logger := testutil.NewMockLogger()

	deps := WorkerSupervisorDeps{
		RuleCache: ruleCache,
		SyncRepo:  syncRepo,
		UsageRepo: usageRepo,
		Compiler:  compiler,
		SyncConfig: RuleSyncWorkerConfig{
			PollInterval:       50 * time.Millisecond,
			StalenessThreshold: 500 * time.Millisecond,
			OverlapBuffer:      10 * time.Millisecond,
		},
		CleanupConfig: UsageCleanupWorkerConfig{
			CleanupInterval: 1 * time.Hour,
		},
		CleanupWorkerEnabled: true,
		CBTemplate: CircuitBreakerTemplate{
			NamePrefix:    "bench_supervisor",
			MaxRequests:   1,
			Interval:      0,
			Timeout:       1 * time.Second,
			FailureThresh: 5,
			FailureRatio:  0,
			MinRequests:   0,
		},
		Clock:      clk,
		MaxTenants: 10,
		Logger:     logger,
	}

	sup, err := NewWorkerSupervisor(deps)
	if err != nil {
		b.Fatalf("new supervisor: %v", err)
	}

	b.Cleanup(sup.Shutdown)

	return sup
}

// BenchmarkEnsureWorkers_HappyPath measures the steady-state cost of the
// middleware-called EnsureWorkers when the tenant is already registered.
// After the sync.Map refactor this path is a lock-free Load + return; before
// it paid a sync.Mutex acquire on every request. Tracking this number catches
// regressions if someone reintroduces a lock on the hot path.
//
// Run:
//
//	go test -bench=BenchmarkEnsureWorkers_HappyPath -benchmem \
//	    ./internal/services/workers/...
func BenchmarkEnsureWorkers_HappyPath(b *testing.B) {
	sup := newBenchSupervisor(b)

	ctx := context.Background()
	if err := sup.EnsureWorkers(ctx, "tenant-a"); err != nil {
		b.Fatalf("prime: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = sup.EnsureWorkers(ctx, "tenant-a")
	}
}

// BenchmarkEnsureWorkers_HappyPathParallel exercises the fast path under
// parallel contention, which is the production shape (one Fiber goroutine
// per request). sync.Map's read-mostly optimisation only pays off when
// multiple goroutines Load concurrently; this bench highlights that win.
func BenchmarkEnsureWorkers_HappyPathParallel(b *testing.B) {
	sup := newBenchSupervisor(b)

	ctx := context.Background()
	if err := sup.EnsureWorkers(ctx, "tenant-a"); err != nil {
		b.Fatalf("prime: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = sup.EnsureWorkers(ctx, "tenant-a")
		}
	})
}
