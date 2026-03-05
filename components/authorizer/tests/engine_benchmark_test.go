// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tests

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

const benchmarkInitialBalance int64 = 1_000_000_000_000

func BenchmarkAuthorizeSingleOp(b *testing.B) {
	b.ReportAllocs()

	router := shard.NewRouter(8)
	accountAlias := "@bench-single"
	e := benchmarkEngineWithAliases(router, []string{accountAlias})

	defer e.Close()

	req := benchmarkAuthorizeRequest(
		"tx-single",
		[]*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@bench-single#default", AccountAlias: accountAlias, BalanceKey: constant.DefaultBalanceKey, Amount: 1, Scale: 2, Operation: constant.CREDIT},
		},
	)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, err := e.Authorize(req)
		if err != nil || !resp.GetAuthorized() {
			b.Fatalf("authorize failed: err=%v authorized=%v", err, resp.GetAuthorized())
		}
	}
}

func BenchmarkAuthorizeParallelSameShard(b *testing.B) {
	b.ReportAllocs()

	router := shard.NewRouter(8)
	accountAlias := "@bench-hot-shard"
	e := benchmarkEngineWithAliases(router, []string{accountAlias})

	defer e.Close()

	req := benchmarkAuthorizeRequest(
		"tx-same-shard",
		[]*authorizerv1.BalanceOperation{
			{OperationAlias: "0#@bench-hot-shard#default", AccountAlias: accountAlias, BalanceKey: constant.DefaultBalanceKey, Amount: 1, Scale: 2, Operation: constant.CREDIT},
		},
	)

	b.ResetTimer()

	var failures atomic.Int64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := e.Authorize(req)
			if err != nil || !resp.GetAuthorized() {
				failures.Add(1)
				return
			}
		}
	})

	if failures.Load() > 0 {
		b.Fatalf("parallel authorize same-shard failed: failures=%d", failures.Load())
	}
}

func BenchmarkAuthorizeParallelCrossShard(b *testing.B) {
	b.ReportAllocs()

	router := shard.NewRouter(8)
	aliases := benchmarkAliasesByDistinctShards(router, 2)
	e := benchmarkEngineWithAliases(router, aliases)

	defer e.Close()

	req := benchmarkAuthorizeRequest(
		"tx-cross-shard",
		[]*authorizerv1.BalanceOperation{
			{OperationAlias: "0#debit-a#default", AccountAlias: aliases[0], BalanceKey: constant.DefaultBalanceKey, Amount: 1, Scale: 2, Operation: constant.CREDIT},
			{OperationAlias: "1#credit-b#default", AccountAlias: aliases[1], BalanceKey: constant.DefaultBalanceKey, Amount: 1, Scale: 2, Operation: constant.CREDIT},
		},
	)

	b.ResetTimer()

	var failures atomic.Int64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := e.Authorize(req)
			if err != nil || !resp.GetAuthorized() {
				failures.Add(1)
				return
			}
		}
	})

	if failures.Load() > 0 {
		b.Fatalf("parallel authorize cross-shard failed: failures=%d", failures.Load())
	}
}

func benchmarkEngineWithAliases(router *shard.Router, aliases []string) *engine.Engine {
	e := engine.New(router, wal.NewNoopWriter())
	balances := make([]*engine.Balance, 0, len(aliases))

	for i, alias := range aliases {
		balances = append(balances, &engine.Balance{
			ID:             fmt.Sprintf("bench-balance-%d", i),
			OrganizationID: "org",
			LedgerID:       "ledger",
			AccountAlias:   alias,
			BalanceKey:     constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      benchmarkInitialBalance,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		})
	}

	e.UpsertBalances(balances)

	return e
}

func benchmarkAliasesByDistinctShards(router *shard.Router, count int) []string {
	if count <= 1 {
		return []string{"@bench-distinct-0"}
	}

	aliases := make([]string, 0, count)
	seenShard := make(map[int]struct{}, count)

	for i := 0; i < 4096 && len(aliases) < count; i++ {
		alias := fmt.Sprintf("@bench-distinct-%d", i)
		shardID := router.ResolveBalance(alias, constant.DefaultBalanceKey)

		if _, exists := seenShard[shardID]; exists {
			continue
		}

		seenShard[shardID] = struct{}{}

		aliases = append(aliases, alias)
	}

	for len(aliases) < count {
		aliases = append(aliases, fmt.Sprintf("@bench-fallback-%d", len(aliases)))
	}

	return aliases
}

func benchmarkAuthorizeRequest(transactionID string, operations []*authorizerv1.BalanceOperation) *authorizerv1.AuthorizeRequest {
	return &authorizerv1.AuthorizeRequest{
		TransactionId:     transactionID,
		OrganizationId:    "org",
		LedgerId:          "ledger",
		Pending:           false,
		TransactionStatus: constant.CREATED,
		Operations:        operations,
	}
}
