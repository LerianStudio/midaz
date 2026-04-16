// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package chaos

import (
	"testing"
)

// TestChaos_Redpanda_UnavailabilityMidPrepare asserts that if Redpanda is
// partitioned during commit-intent publish, the coordinator surfaces the
// publish error rather than silently dropping the commit intent. This is
// the regression guard for B3: a publisher that silently reports success
// on failed-to-publish commit intents leads to zombie cross-shard
// transactions that never reconcile.
//
// Scenario:
//  1. Start an authorizer with Redpanda behind a Toxiproxy bandwidth/close
//     toxic so publish() hangs or fails mid-flight.
//  2. Issue a cross-shard Authorize that reaches publishCommitIntent.
//  3. Inject the toxic.
//  4. Expected: RedpandaPublisher.Publish returns a timeout/network error;
//     Authorize surfaces the error; the publish-errors counter fires;
//     no zombie prepared-tx remains.
//
// Deferral: requires Redpanda-through-Toxiproxy fixture. The chaos
// harness already supports Toxiproxy in front of PostgreSQL (see
// postgres_restart_writes_test.go); the equivalent Redpanda fixture is
// a one-commit addition but belongs in the DevOps/infrastructure track,
// not this test-gap-closure commit. Tracked at FINAL_REVIEW.md batch A.
func TestChaos_Redpanda_UnavailabilityMidPrepare(t *testing.T) { //nolint:paralleltest // chaos tests interact with shared Docker infrastructure
	shouldRunChaos(t)
	t.Skip("requires Redpanda-through-Toxiproxy fixture; run via `make test-chaos-authorizer` once infrastructure is plumbed — see FINAL_REVIEW.md batch A")

	// TODO(fred): when Redpanda-Toxiproxy fixture lands:
	//  1. orch.StartRedpandaThroughToxiproxy(t)
	//  2. Configure authorizer with Redpanda proxy address.
	//  3. Issue a cross-shard Authorize.
	//  4. Inject close/bandwidth toxic on the Redpanda proxy.
	//  5. Assert Publish returns an error and no commit-intent is silently
	//     treated as durable.
}
