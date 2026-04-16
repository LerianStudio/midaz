// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"testing"
)

// TestChaos_CrossShardPeerPartition_MidPrepare covers the 2PC zombie
// closure: when a peer authorizer becomes unreachable in the middle of a
// cross-shard Prepare, the coordinator MUST abort cleanly — no zombie
// prepared-tx, no partial commit, and the coordinator MUST release its
// local locks.
//
// Scenario:
//  1. Two authorizer instances bring up with their peer gRPC endpoints
//     behind a Toxiproxy proxy.
//  2. A cross-shard Authorize starts and reaches runPrepareSequence /
//     parallel prepare dispatch.
//  3. Toxiproxy drops all bytes mid-RPC (timeout toxic).
//  4. Expected: Prepare returns an error, the coordinator surfaces
//     UNAVAILABLE, and the prepared_pending_depth gauge on both instances
//     returns to zero within the reaper's cleanup interval.
//
// Deferral: the two-authorizer docker-compose harness with Toxiproxy
// in front of :50051 is not yet wired into tests/utils/chaos. The work
// item is tracked at FINAL_REVIEW.md#test-gaps batch A. This test is
// intentionally shipped as a skeleton so the dependency is visible in the
// build matrix and a future commit can finish it without a re-plumbing
// pass through chaos/infrastructure.go.
func TestChaos_CrossShardPeerPartition_MidPrepare(t *testing.T) { //nolint:paralleltest // chaos tests interact with shared Docker infrastructure
	shouldRunChaos(t)
	t.Skip("requires two-authorizer Toxiproxy harness (peer gRPC proxy); run via `make test-chaos-authorizer` once infrastructure is plumbed — see FINAL_REVIEW.md batch A")

	// TODO(fred): when authorizer-peer Toxiproxy fixture lands:
	//  1. orch.StartToxiproxyBackedAuthorizerPair(t)
	//  2. Issue a cross-shard Authorize via the first instance.
	//  3. Inject timeout toxic on the peer proxy during Prepare.
	//  4. Assert coordinator surfaces an error and both instances'
	//     prepared_pending_depth gauges return to zero.
}

// TestChaos_CrossShardPeerPartition_MidCommit covers the 2PC recovery
// closure: when a peer becomes unreachable AFTER Prepare succeeded but
// BEFORE Commit lands, the recovery runner (commit_intent_recovery.go)
// MUST drive the transaction to completion using the durable
// commitIntent record in Redpanda.
//
// Scenario:
//  1. Cross-shard Prepare succeeds on both shards.
//  2. Commit-intent is durably published.
//  3. Toxiproxy severs the coordinator → peer connection during the
//     Commit RPC.
//  4. Expected: the peer's commit_intent_recovery loop reconciles from
//     Redpanda within its poll interval; no MANUAL_INTERVENTION_REQUIRED
//     fires; final balances reflect the commit.
//
// Deferred for the same reason as above.
func TestChaos_CrossShardPeerPartition_MidCommit(t *testing.T) { //nolint:paralleltest // chaos tests interact with shared Docker infrastructure
	shouldRunChaos(t)
	t.Skip("requires two-authorizer Toxiproxy harness (peer gRPC proxy); run via `make test-chaos-authorizer` once infrastructure is plumbed — see FINAL_REVIEW.md batch A")
}
