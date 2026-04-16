// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tests

import (
	"sync/atomic"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// BenchmarkAuthorizeContentionSameBalanceWithDEBIT stresses the near-zero-
// available path with N goroutines all contending the same balance with
// DEBIT operations. The existing BenchmarkAuthorizeParallelSameShard uses
// CREDIT with 1e12 initial balance, which never hits the balance-check
// branch; this benchmark explicitly exercises the insufficient-funds
// rejection path under contention so a regression in the InsufficientFunds
// early-exit or the version-gate is visible as throughput degradation.
//
// Expected shape: approvals and rejections are both frequent. A healthy
// engine rejects deterministically based on the per-balance mutex order,
// which prevents the "race condition double-debit" failure mode.
func BenchmarkAuthorizeContentionSameBalanceWithDEBIT(b *testing.B) {
	b.ReportAllocs()

	router := shard.NewRouter(8)
	alias := "@contended-debit"

	e := engine.New(router, wal.NewNoopWriter())

	defer e.Close()

	// Seed with a small available balance so DEBIT frequently triggers
	// insufficient-funds. Exactly 100 successful debits of 1 unit each
	// drain the balance.
	e.UpsertBalances([]*engine.Balance{{
		ID:             "contended",
		OrganizationID: "org", LedgerID: "ledger",
		AccountAlias: alias,
		BalanceKey:   constant.DefaultBalanceKey,
		AssetCode:    "USD",
		Available:    100,
		Scale:        2,
		Version:      1,
		AllowSending: true, AllowReceiving: true,
	}})

	req := benchmarkAuthorizeRequest(
		"tx-contended-debit",
		[]*authorizerv1.BalanceOperation{
			{
				OperationAlias: "0#" + alias + "#default",
				AccountAlias:   alias,
				BalanceKey:     constant.DefaultBalanceKey,
				Amount:         1,
				Scale:          2,
				Operation:      constant.DEBIT,
			},
		},
	)

	var (
		approvals atomic.Int64
		rejects   atomic.Int64
	)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := e.Authorize(req)
			if err != nil {
				b.Fatalf("authorize error: %v", err)
			}

			if resp.GetAuthorized() {
				approvals.Add(1)
			} else {
				rejects.Add(1)
			}
		}
	})

	b.StopTimer()

	b.ReportMetric(float64(approvals.Load()), "approvals")
	b.ReportMetric(float64(rejects.Load()), "rejects")

	// Invariant: at most 100 approvals can succeed (seed=100, debit=1).
	// More than 100 would indicate a broken version-gate/double-commit —
	// catastrophic for a financial ledger.
	if approvals.Load() > 100 {
		b.Fatalf("BROKEN: %d approvals exceeded seed balance (100) — double-commit regression", approvals.Load())
	}
}
