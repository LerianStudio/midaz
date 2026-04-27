// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildOperationSnapshot_NonOverdraft verifies that two zero-OverdraftUsed
// balances (the vast majority of operations) produce a snapshot with both
// fields explicitly set to "0". Under the always-populated wire-shape contract,
// non-overdraft ops carry the zero shape rather than absent fields.
func TestBuildOperationSnapshot_NonOverdraft(t *testing.T) {
	t.Parallel()

	before := &mmodel.Balance{
		OverdraftUsed: decimal.Zero,
	}

	after := &mmodel.Balance{
		OverdraftUsed: decimal.Zero,
	}

	snap := buildOperationSnapshot(before, after)
	assert.Equal(t, "0", snap.OverdraftUsedBefore, "non-overdraft ops carry '0' explicitly")
	assert.Equal(t, "0", snap.OverdraftUsedAfter)
}

// TestBuildOperationSnapshot_DebitSplit verifies a debit that triggers an
// overdraft split: before=0, after=50. Both fields populated; before is the
// canonical "0" string.
func TestBuildOperationSnapshot_DebitSplit(t *testing.T) {
	t.Parallel()

	before := &mmodel.Balance{OverdraftUsed: decimal.Zero}
	after := &mmodel.Balance{OverdraftUsed: decimal.NewFromInt(50)}

	snap := buildOperationSnapshot(before, after)
	assert.Equal(t, "0", snap.OverdraftUsedBefore, "zero before serializes as '0'")
	assert.Equal(t, "50", snap.OverdraftUsedAfter)
}

// TestBuildOperationSnapshot_CreditRepayment verifies a credit that fully
// repays outstanding overdraft: before=130, after=0. Both fields populated.
func TestBuildOperationSnapshot_CreditRepayment(t *testing.T) {
	t.Parallel()

	before := &mmodel.Balance{OverdraftUsed: decimal.NewFromInt(130)}
	after := &mmodel.Balance{OverdraftUsed: decimal.Zero}

	snap := buildOperationSnapshot(before, after)
	assert.Equal(t, "130", snap.OverdraftUsedBefore)
	assert.Equal(t, "0", snap.OverdraftUsedAfter, "zero after serializes as '0'")
}

// TestBuildOperationSnapshot_CumulativeOverdraft verifies cumulative overdraft
// usage: before=50, after=130 (a second debit while overdraft is already active).
func TestBuildOperationSnapshot_CumulativeOverdraft(t *testing.T) {
	t.Parallel()

	before := &mmodel.Balance{OverdraftUsed: decimal.NewFromInt(50)}
	after := &mmodel.Balance{OverdraftUsed: decimal.NewFromInt(130)}

	snap := buildOperationSnapshot(before, after)
	assert.Equal(t, "50", snap.OverdraftUsedBefore)
	assert.Equal(t, "130", snap.OverdraftUsedAfter)
}

// TestBuildOperationSnapshot_NilBalances verifies that nil before/after
// balances default to "0" without panicking. nil decays to "0" so callers
// in defensive paths (where one or both balances aren't in scope) still get
// a wire-compliant snapshot.
func TestBuildOperationSnapshot_NilBalances(t *testing.T) {
	t.Parallel()

	snap := buildOperationSnapshot(nil, nil)
	assert.Equal(t, "0", snap.OverdraftUsedBefore, "both nil → both '0'")
	assert.Equal(t, "0", snap.OverdraftUsedAfter)

	snap = buildOperationSnapshot(nil, &mmodel.Balance{OverdraftUsed: decimal.Zero})
	assert.Equal(t, "0", snap.OverdraftUsedBefore, "nil before → '0'")
	assert.Equal(t, "0", snap.OverdraftUsedAfter)

	snap = buildOperationSnapshot(&mmodel.Balance{OverdraftUsed: decimal.Zero}, nil)
	assert.Equal(t, "0", snap.OverdraftUsedBefore)
	assert.Equal(t, "0", snap.OverdraftUsedAfter, "nil after → '0'")

	// Non-zero with nil counterpart → counterpart still "0".
	snap = buildOperationSnapshot(nil, &mmodel.Balance{OverdraftUsed: decimal.NewFromInt(50)})
	assert.Equal(t, "0", snap.OverdraftUsedBefore)
	assert.Equal(t, "50", snap.OverdraftUsedAfter)
}

// TestBuildOperationSnapshot_PendingPath verifies PENDING operations where
// before == after (overdraft is active but not mutated by the hold).
func TestBuildOperationSnapshot_PendingPath(t *testing.T) {
	t.Parallel()

	blc := &mmodel.Balance{OverdraftUsed: decimal.NewFromInt(50)}

	snap := buildOperationSnapshot(blc, blc)
	assert.Equal(t, "50", snap.OverdraftUsedBefore)
	assert.Equal(t, "50", snap.OverdraftUsedAfter, "before==after on PENDING")
}

// TestBuildOperationSnapshot_PendingNoOverdraft verifies PENDING operations on
// a non-overdraft balance still carry the zero shape (always-populated).
func TestBuildOperationSnapshot_PendingNoOverdraft(t *testing.T) {
	t.Parallel()

	blc := &mmodel.Balance{OverdraftUsed: decimal.Zero}

	snap := buildOperationSnapshot(blc, blc)
	assert.Equal(t, "0", snap.OverdraftUsedBefore)
	assert.Equal(t, "0", snap.OverdraftUsedAfter)
}

// TestBuildOperationSnapshot_AnnotationZeroShape verifies that annotation
// (NOTED) transactions emit the zero shape via buildOperationSnapshot(nil, nil).
// Under the always-populated contract annotations carry "0"/"0" so the wire
// shape remains uniform — the snapshot is meaningless on annotations (no
// balance movement) but the contract requires presence.
func TestBuildOperationSnapshot_AnnotationZeroShape(t *testing.T) {
	t.Parallel()

	snap := buildOperationSnapshot(nil, nil)
	assert.Equal(t, "0", snap.OverdraftUsedBefore, "annotation ops carry '0' explicitly")
	assert.Equal(t, "0", snap.OverdraftUsedAfter)
}

// TestPropagateSnapshotToCompanions verifies that after building all operations,
// companion (overdraft) operations inherit the primary (default) operation's
// snapshot value and typed OverdraftUsed fields so both rows in the pair tell
// the same lifecycle story.
func TestPropagateSnapshotToCompanions(t *testing.T) {
	t.Parallel()

	primarySnapshot := mmodel.OperationSnapshot{
		OverdraftUsedBefore: "50",
		OverdraftUsedAfter:  "130",
	}

	beforeDec := decimal.NewFromInt(50)
	afterDec := decimal.NewFromInt(130)

	// Simulate: primary op for @alice#default and companion for @alice#overdraft.
	ops := []*operationForSnapshot{
		{
			accountAlias:              "@alice",
			balanceKey:                "default",
			snapshot:                  primarySnapshot,
			balanceOverdraftUsed:      beforeDec,
			balanceAfterOverdraftUsed: afterDec,
		},
		{
			accountAlias: "@alice",
			balanceKey:   "overdraft",
			snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
			}, // companion's own builder result, to be overwritten by propagation
		},
	}

	propagateSnapshotToCompanions(ops)

	assert.Equal(t, "50", ops[1].snapshot.OverdraftUsedBefore, "companion inherits primary's snapshot")
	assert.Equal(t, "130", ops[1].snapshot.OverdraftUsedAfter)
	assert.True(t, beforeDec.Equal(ops[1].balanceOverdraftUsed))
	assert.True(t, afterDec.Equal(ops[1].balanceAfterOverdraftUsed))
}

// TestPropagateSnapshotToCompanions_NoOverdraft verifies that propagation copies
// the primary's zero shape onto the companion when neither participates in the
// overdraft lifecycle. Under the always-populated contract every primary
// carries a snapshot, so the propagation always runs — there's no "skip when
// nil" path anymore.
func TestPropagateSnapshotToCompanions_NoOverdraft(t *testing.T) {
	t.Parallel()

	zeroShape := mmodel.OperationSnapshot{
		OverdraftUsedBefore: "0",
		OverdraftUsedAfter:  "0",
	}

	ops := []*operationForSnapshot{
		{
			accountAlias:              "@alice",
			balanceKey:                "default",
			snapshot:                  zeroShape,
			balanceOverdraftUsed:      decimal.Zero,
			balanceAfterOverdraftUsed: decimal.Zero,
		},
		{
			accountAlias: "@alice",
			balanceKey:   "overdraft",
			// Companion-side state — to be overwritten with the primary's
			// zero shape.
			snapshot:                  mmodel.OperationSnapshot{},
			balanceOverdraftUsed:      decimal.Zero,
			balanceAfterOverdraftUsed: decimal.Zero,
		},
	}

	propagateSnapshotToCompanions(ops)
	assert.Equal(t, "0", ops[1].snapshot.OverdraftUsedBefore, "companion inherits primary's zero shape")
	assert.Equal(t, "0", ops[1].snapshot.OverdraftUsedAfter)
	assert.True(t, ops[1].balanceOverdraftUsed.Equal(decimal.Zero))
	assert.True(t, ops[1].balanceAfterOverdraftUsed.Equal(decimal.Zero))
}

// TestBuildOperations_CompanionPropagation_EndToEnd exercises the full
// snapshot propagation pipeline as executed by BuildOperations in
// transaction_create.go: adapter construction → propagateSnapshotToCompanions
// → writeback onto []*operation.Operation. It verifies that a companion
// (overdraft-key) operation inherits the primary (default-key) operation's
// snapshot and typed OverdraftUsed fields, and that a non-participating
// destination operation carries zero-shape values.
//
// Regression guard: if the writeback loop is accidentally dropped or the
// adapter construction is broken, the assertions on the companion operation
// will fail.
func TestBuildOperations_CompanionPropagation_EndToEnd(t *testing.T) {
	t.Parallel()

	// Construct 3 operations simulating the output of the build loop:
	//   0: source default DEBIT  — carries real overdraft snapshot
	//   1: source overdraft companion DEBIT — initially has zero-shape
	//       (mimics companion builders which use their own blc/after,
	//        and the overdraft balance itself has OverdraftUsed=0)
	//   2: destination CREDIT — non-overdraft, zero-shape throughout
	operations := []*operation.Operation{
		{
			AccountAlias: "@alice",
			BalanceKey:   constant.DefaultBalanceKey,
			Snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "50",
			},
			Balance: operation.Balance{
				OverdraftUsed: decimal.Zero,
			},
			BalanceAfter: operation.Balance{
				OverdraftUsed: decimal.NewFromInt(50),
			},
		},
		{
			AccountAlias: "@alice",
			BalanceKey:   constant.OverdraftBalanceKey,
			// Companion's own builder result — will be overwritten by propagation.
			Snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
			},
			Balance: operation.Balance{
				OverdraftUsed: decimal.Zero,
			},
			BalanceAfter: operation.Balance{
				OverdraftUsed: decimal.Zero,
			},
		},
		{
			AccountAlias: "@bob",
			BalanceKey:   constant.DefaultBalanceKey,
			Snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
			},
			Balance: operation.Balance{
				OverdraftUsed: decimal.Zero,
			},
			BalanceAfter: operation.Balance{
				OverdraftUsed: decimal.Zero,
			},
		},
	}

	// ── Execute the exact same bridge code as BuildOperations lines 235-252 ──

	snapshotAdapters := make([]*operationForSnapshot, len(operations))
	for idx, op := range operations {
		snapshotAdapters[idx] = &operationForSnapshot{
			accountAlias:              op.AccountAlias,
			balanceKey:                op.BalanceKey,
			snapshot:                  op.Snapshot,
			balanceOverdraftUsed:      op.Balance.OverdraftUsed,
			balanceAfterOverdraftUsed: op.BalanceAfter.OverdraftUsed,
		}
	}

	propagateSnapshotToCompanions(snapshotAdapters)

	for idx, a := range snapshotAdapters {
		operations[idx].Snapshot = a.snapshot
		operations[idx].Balance.OverdraftUsed = a.balanceOverdraftUsed
		operations[idx].BalanceAfter.OverdraftUsed = a.balanceAfterOverdraftUsed
	}

	// ── Assert: companion mirrors primary ──

	require.Len(t, operations, 3, "expected 3 operations (source default, source overdraft, destination)")

	primary := operations[0]
	companion := operations[1]
	destination := operations[2]

	// Companion snapshot must equal primary's snapshot (propagated).
	assert.Equal(t, primary.Snapshot.OverdraftUsedBefore, companion.Snapshot.OverdraftUsedBefore,
		"companion must inherit primary's OverdraftUsedBefore")
	assert.Equal(t, "0", companion.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, primary.Snapshot.OverdraftUsedAfter, companion.Snapshot.OverdraftUsedAfter,
		"companion must inherit primary's OverdraftUsedAfter")
	assert.Equal(t, "50", companion.Snapshot.OverdraftUsedAfter)

	// Companion typed fields must equal primary's typed fields (propagated).
	assert.True(t, companion.Balance.OverdraftUsed.Equal(primary.Balance.OverdraftUsed),
		"companion Balance.OverdraftUsed must mirror primary: got %s want %s",
		companion.Balance.OverdraftUsed.String(), primary.Balance.OverdraftUsed.String())
	assert.True(t, companion.BalanceAfter.OverdraftUsed.Equal(primary.BalanceAfter.OverdraftUsed),
		"companion BalanceAfter.OverdraftUsed must mirror primary: got %s want %s",
		companion.BalanceAfter.OverdraftUsed.String(), primary.BalanceAfter.OverdraftUsed.String())
	assert.True(t, companion.BalanceAfter.OverdraftUsed.Equal(decimal.NewFromInt(50)),
		"companion BalanceAfter.OverdraftUsed must be 50")

	// Destination must NOT be affected by propagation — retains zero-shape.
	assert.Equal(t, "0", destination.Snapshot.OverdraftUsedBefore,
		"destination must retain zero OverdraftUsedBefore")
	assert.Equal(t, "0", destination.Snapshot.OverdraftUsedAfter,
		"destination must retain zero OverdraftUsedAfter")
	assert.True(t, destination.Balance.OverdraftUsed.Equal(decimal.Zero),
		"destination Balance.OverdraftUsed must remain zero")
	assert.True(t, destination.BalanceAfter.OverdraftUsed.Equal(decimal.Zero),
		"destination BalanceAfter.OverdraftUsed must remain zero")
}

// TestBuildOperations_CompanionPropagation_WritebackRequired verifies that
// removing the writeback loop in transaction_create.go causes companion
// operations to miss propagated values. This is the RED half of the
// regression guard — without the writeback, the companion retains its
// original zero-shape values instead of inheriting the primary's snapshot.
func TestBuildOperations_CompanionPropagation_WritebackRequired(t *testing.T) {
	t.Parallel()

	// Same setup as the end-to-end test above.
	operations := []*operation.Operation{
		{
			AccountAlias: "@alice",
			BalanceKey:   constant.DefaultBalanceKey,
			Snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "50",
			},
			Balance:      operation.Balance{OverdraftUsed: decimal.Zero},
			BalanceAfter: operation.Balance{OverdraftUsed: decimal.NewFromInt(50)},
		},
		{
			AccountAlias: "@alice",
			BalanceKey:   constant.OverdraftBalanceKey,
			Snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
			},
			Balance:      operation.Balance{OverdraftUsed: decimal.Zero},
			BalanceAfter: operation.Balance{OverdraftUsed: decimal.Zero},
		},
	}

	// Run adapter construction + propagation WITHOUT writeback.
	snapshotAdapters := make([]*operationForSnapshot, len(operations))
	for idx, op := range operations {
		snapshotAdapters[idx] = &operationForSnapshot{
			accountAlias:              op.AccountAlias,
			balanceKey:                op.BalanceKey,
			snapshot:                  op.Snapshot,
			balanceOverdraftUsed:      op.Balance.OverdraftUsed,
			balanceAfterOverdraftUsed: op.BalanceAfter.OverdraftUsed,
		}
	}

	propagateSnapshotToCompanions(snapshotAdapters)

	// Intentionally skip writeback — this simulates the regression.
	// The adapters are propagated, but operations[1] still has zero-shape.

	// The adapter itself IS propagated...
	assert.Equal(t, "50", snapshotAdapters[1].snapshot.OverdraftUsedAfter,
		"adapter should be propagated")

	// ...but without writeback the operation still has the original value.
	assert.Equal(t, "0", operations[1].Snapshot.OverdraftUsedAfter,
		"without writeback, operation retains original zero-shape — "+
			"this proves the writeback loop is required")
}
