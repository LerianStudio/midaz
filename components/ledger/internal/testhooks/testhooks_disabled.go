// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build !testhooks

package testhooks

import "context"

// Point identifies a deterministic pause in the money path. The values are
// intentionally stable because the external E2E harness uses them as a
// process-boundary contract.
type Point string

const (
	PointAfterEconomicMutation   Point = "after_economic_mutation"
	PointAfterRevertClaimMutated Point = "after_revert_claim_mutated"
)

// IDs is the request and transaction identity written to a local marker when
// the testhook build is active. Empty origin/reverse IDs mean this is a normal
// transaction rather than a revert.
type IDs struct {
	RequestID     string
	TransactionID string
	OriginID      string
	ReverseID     string
}

// Pause is physically a no-op in production builds. The implementation that
// reads local testhook state is excluded by the build tag above, so production
// binaries contain no pause protocol, filesystem access, or testhook env reads.
func Pause(_ context.Context, _ Point, _ IDs) error {
	return nil
}
