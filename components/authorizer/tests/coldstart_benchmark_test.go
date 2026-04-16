// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// BenchmarkColdStart_10K measures the time to upsert 10,000 balances into a
// fresh engine. This is the synthetic proxy for LoadBalancesStreaming's
// keyset-paginated load (D1 commit 893dc15c5): the streaming path ultimately
// calls UpsertBalances per page, so benchmarking the upsert cost at scale
// tells us how much boot latency the loader's final hop contributes. The
// PG-side query overhead is intentionally excluded so the benchmark stays
// stable across environments.
//
// Run as:
//
//	go test -bench=BenchmarkColdStart -benchtime=1x ./components/authorizer/tests/
func BenchmarkColdStart_10K(b *testing.B) {
	benchmarkColdStart(b, 10_000)
}

// BenchmarkColdStart_100K is the tenant-scale target (~10k tenants x 10
// balances each) we expect most production deployments to hit. A regression
// here is a regression in readiness-gate time — every ms counts during a
// rolling restart.
func BenchmarkColdStart_100K(b *testing.B) {
	benchmarkColdStart(b, 100_000)
}

// BenchmarkColdStart_1M is the horizontal-scale ceiling we've targeted for
// the 100K TPS roadmap. Benchmark is skipped unless explicitly enabled via
// -bench because 1M-balance seeding dominates short benchmark windows.
func BenchmarkColdStart_1M(b *testing.B) {
	if testing.Short() {
		b.Skip("BenchmarkColdStart_1M skipped under -short; enable with -bench and sufficient benchtime")
	}

	benchmarkColdStart(b, 1_000_000)
}

// benchmarkColdStart drives the reported benchmark. It builds a slice of
// balances outside of b.N loop (so only the upsert hot path is timed),
// then restarts the engine each iteration.
func benchmarkColdStart(b *testing.B, count int) {
	b.Helper()
	b.ReportAllocs()

	balances := buildColdStartBalances(count)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		router := shard.NewRouter(16)
		e := engine.New(router, wal.NewNoopWriter())

		b.StartTimer()

		upserted := e.UpsertBalances(balances)

		b.StopTimer()

		if upserted != int64(count) {
			b.Fatalf("cold start %d: upserted=%d, expected %d", count, upserted, count)
		}

		e.Close()

		b.StartTimer()
	}
}

// buildColdStartBalances returns count synthetic balances with unique
// aliases so they distribute across all shards. The values are chosen
// so engine math (available, version) doesn't exercise edge cases — the
// benchmark is about throughput, not correctness.
func buildColdStartBalances(count int) []*engine.Balance {
	balances := make([]*engine.Balance, count)

	for i := 0; i < count; i++ {
		balances[i] = &engine.Balance{
			ID:             fmt.Sprintf("cold-balance-%d", i),
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountID:      fmt.Sprintf("cold-account-%d", i),
			AccountAlias:   fmt.Sprintf("@cold-%d", i),
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "USD",
			Available:      1_000_000,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		}
	}

	return balances
}

// BenchmarkColdStartBootLatency_p99 is a micro-measurement of a single
// 100K-balance load reported in wall-clock time. Go's `testing.B` doesn't
// expose p50/p99 directly, but ns/op at b.N=1 is the deterministic cold-
// start cost — regressions show as ns/op drift between releases.
//
// This benchmark exists alongside BenchmarkColdStart_100K because the
// former uses b.N as a throughput multiplier (ns/op = per-op); this one
// uses StopTimer/StartTimer to report the wall-clock of a single load,
// which is closer to what operators see as "seconds to Ready".
func BenchmarkColdStartBootLatency_100K_SingleLoad(b *testing.B) {
	// We ignore b.N here deliberately — this benchmark reports the
	// wall-clock of a single 100K-balance load, not an ns/op average.
	// Using b.ReportMetric to surface boot_ms and balances/s keeps the
	// output intelligible for operators.
	balances := buildColdStartBalances(100_000)

	router := shard.NewRouter(16)
	e := engine.New(router, wal.NewNoopWriter())

	defer e.Close()

	b.ResetTimer()

	start := time.Now()

	upserted := e.UpsertBalances(balances)

	elapsed := time.Since(start)

	b.StopTimer()

	if upserted != int64(len(balances)) {
		b.Fatalf("single-load cold start: upserted=%d, expected %d", upserted, len(balances))
	}

	b.ReportMetric(float64(elapsed.Milliseconds()), "boot_ms")
	b.ReportMetric(float64(len(balances))/(elapsed.Seconds()+1e-9), "balances/s")
}
