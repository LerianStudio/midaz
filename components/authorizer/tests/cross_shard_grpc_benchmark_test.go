// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tests

import (
	"testing"
)

// BenchmarkAuthorizeCrossShardGRPC measures the end-to-end wall-clock of a
// cross-shard Authorize when it traverses the real gRPC path between two
// authorizer instances via bufconn. The existing
// BenchmarkAuthorizeParallelCrossShard times only the in-process engine;
// the real 100K-TPS claim depends on (prepare_fanout_parallelism ==
// max(RTT_i), not Σ RTT_i) holding in the actual RPC stack — including
// HMAC peer auth, codec marshalling, stream context propagation, and
// participant lock acquisition under the real gRPC concurrency regime.
//
// Deferral: this benchmark requires two in-process authorizer bootstraps
// listening on bufconn endpoints, each with the full Config surface
// satisfied (peer auth tokens, WAL paths, publisher wiring, etc.).
// Constructing those from a benchmark file requires either (a) a
// dedicated "test harness" library in components/authorizer/tests that
// extracts the minimum-viable bootstrap, or (b) a significant refactor
// of bootstrap.Run to expose a constructor that doesn't own the process
// lifecycle. Both options are larger than this test-gap-closure commit
// justifies; we ship the skeleton so B5's "parallel prepare" claim has a
// placeholder regression guard.
//
// Target assertion when implemented: with 2 participants and ~1ms RTT
// each, end-to-end latency must be ≤ max(RTT)*1.5 — not Σ RTT. A linear
// grow-with-N pattern is B5's regression signature.
//
// Tracked at FINAL_REVIEW.md#test-gaps batch A item 2.
func BenchmarkAuthorizeCrossShardGRPC(b *testing.B) {
	b.Skip("requires dual-instance bufconn harness; see FINAL_REVIEW.md batch A — a minimum-viable authorizer test harness belongs in a follow-up infrastructure commit")

	// TODO(fred): when harness lands, shape:
	//
	//  1. inst1 := startAuthorizerOnBufconn(b, "inst-1", shards=[0..7])
	//  2. inst2 := startAuthorizerOnBufconn(b, "inst-2", shards=[8..15])
	//  3. Wire inst1.peers[inst2.addr] and vice-versa via the bufconn
	//     dialer so peer gRPC calls stay in-process.
	//  4. Seed balances on both instances s.t. our two aliases cross
	//     shard boundaries (hash to 0..7 on inst1 and 8..15 on inst2).
	//  5. b.ResetTimer(); for b.N { inst1.Authorize(cross_shard_req) }
	//  6. Report ns/op and compare to B5's parallel-prepare target.
}
