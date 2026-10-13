// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"sort"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ReplayReconciliationVerdict says whether a replay may write its rebuilt
// operations.
type ReplayReconciliationVerdict string

const (
	// ReplayReconciliationResolved means every rebuilt leg is proven either
	// persisted, and carries the persisted ID, or missing.
	ReplayReconciliationResolved ReplayReconciliationVerdict = "resolved"
	// ReplayReconciliationMismatch means the persisted legs cannot be told apart
	// from the rebuilt ones, so nothing may be written.
	ReplayReconciliationMismatch ReplayReconciliationVerdict = "mismatch"
)

// ReplayReconciliationMismatchReason is the closed set of reasons a
// reconciliation refuses to decide. It is safe for logs and span attributes.
type ReplayReconciliationMismatchReason string

const (
	// ReplayMismatchCountMismatch: an exact leg is persisted, but a different
	// number of times than it was rebuilt (a partial repeated leg or an earlier
	// duplicate).
	ReplayMismatchCountMismatch ReplayReconciliationMismatchReason = "count_mismatch"
	// ReplayMismatchUnpairedPersisted: a persisted leg on the same balance and
	// starting version as a rebuilt leg has no exact rebuilt pair (an amount
	// clipped by overdraft, or a leg shaped by another route setting).
	ReplayMismatchUnpairedPersisted ReplayReconciliationMismatchReason = "unpaired_persisted"
	// ReplayMismatchIncompleteRebuilt: a rebuilt leg lacks its type, starting
	// version or amount, so it cannot be compared at all.
	ReplayMismatchIncompleteRebuilt ReplayReconciliationMismatchReason = "incomplete_rebuilt"
)

// ReconcileResult is the outcome of reconciling rebuilt operations with the
// ones already persisted for the same transaction.
type ReconcileResult struct {
	Verdict ReplayReconciliationVerdict
	// Operations has one entry per rebuilt leg, in the rebuilt order. A leg
	// proven persisted carries the persisted ID; any other leg is unchanged. It
	// is nil on a mismatch.
	Operations     []*operation.Operation
	Adopted        int
	Inserted       int
	MismatchReason ReplayReconciliationMismatchReason
}

// ReconcileReplayedOperations reads the operations already persisted for one
// transaction from the primary and reconciles the rebuilt ones against them
// (see reconcileReplayedOperations). A read failure returns an error and no
// verdict: whether a leg is persisted is then unknown.
func (uc *UseCase) ReconcileReplayedOperations(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, rebuilt []*operation.Operation) (ReconcileResult, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reconcile_replayed_operations")
	defer span.End()

	persisted, err := uc.OperationRepo.ListByTransactionPrimary(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read persisted operations of replayed transaction", err)

		return ReconcileResult{}, fmt.Errorf("read persisted operations of replayed transaction: %w", err)
	}

	result := reconcileReplayedOperations(transactionID.String(), rebuilt, persisted)

	span.SetAttributes(
		attribute.String("app.replay.reconciliation.verdict", string(result.Verdict)),
		attribute.Int("app.replay.reconciliation.persisted", len(persisted)),
		attribute.Int("app.replay.reconciliation.adopted", result.Adopted),
		attribute.Int("app.replay.reconciliation.inserted", result.Inserted),
	)

	if result.Verdict == ReplayReconciliationMismatch {
		span.SetAttributes(attribute.String("app.replay.reconciliation.mismatch_reason", string(result.MismatchReason)))
	}

	return result, nil
}

// legGroup identifies the legs a reconciliation compares as one population:
// one transaction, balance, balance key, operation type and starting version.
// Hold, commit and cancel legs of one transaction differ in type or starting
// version; annotation legs all start at version 0 and differ by amount.
type legGroup struct {
	transactionID string
	balanceID     string
	balanceKey    string
	opType        string
	versionBefore int64
}

// legPosition is a leg's balance and starting version regardless of its type.
type legPosition struct {
	balanceID     string
	balanceKey    string
	versionBefore int64
}

type persistedLeg struct {
	op     *operation.Operation
	paired bool
}

// reconcileReplayedOperations decides which rebuilt legs are already persisted.
// A replay may only insert a leg it proves missing; in doubt it writes nothing.
//
// Only the transaction's own persisted legs are compared, grouped by
// (transaction, balance, balance key, type, starting version) and, inside a
// group, by exact decimal amount, counting multiplicity. For every group with
// rebuilt legs:
//
//  1. No persisted leg in the group: every rebuilt leg is missing.
//  2. For each amount: as many persisted as rebuilt legs adopts the persisted
//     IDs in ID order; no persisted leg means the rebuilt legs are missing; any
//     other count is a count_mismatch.
//  3. A persisted leg in the group whose amount no rebuilt leg has is an
//     unpaired_persisted mismatch, such as an amount clipped by overdraft.
//  4. A persisted leg left unpaired on the same balance, balance key and
//     starting version as any rebuilt leg is an unpaired_persisted mismatch
//     even with another type, such as a hold persisted as DEBIT and ONHOLD and
//     rebuilt as ON_HOLD after the route setting changed. A rebuilt leg without
//     type, starting version or amount is an incomplete_rebuilt mismatch.
//
// Other persisted legs, such as an overdraft companion or the legs of another
// phase of the same transaction, are ignored. A mismatch anywhere refuses the
// whole record. Adoption copies only the ID; after-balances, aliases and IDs are
// never compared, and the input is not mutated.
func reconcileReplayedOperations(transactionID string, rebuilt, persisted []*operation.Operation) ReconcileResult {
	mismatch := func(reason ReplayReconciliationMismatchReason) ReconcileResult {
		return ReconcileResult{Verdict: ReplayReconciliationMismatch, MismatchReason: reason}
	}

	rebuiltByGroup := make(map[legGroup][]int, len(rebuilt))
	rebuiltPositions := make(map[legPosition]bool, len(rebuilt))
	groupOrder := make([]legGroup, 0, len(rebuilt))

	for i, op := range rebuilt {
		if op == nil || op.Type == "" || op.Balance.Version == nil || op.Amount.Value == nil {
			return mismatch(ReplayMismatchIncompleteRebuilt)
		}

		group := groupOf(transactionID, op)
		if _, seen := rebuiltByGroup[group]; !seen {
			groupOrder = append(groupOrder, group)
		}

		rebuiltByGroup[group] = append(rebuiltByGroup[group], i)
		rebuiltPositions[positionOf(group)] = true
	}

	persistedByGroup := make(map[legGroup][]*persistedLeg, len(persisted))
	allPersisted := make([]*persistedLeg, 0, len(persisted))

	for _, op := range persisted {
		if op == nil || op.Balance.Version == nil {
			continue
		}

		leg := &persistedLeg{op: op}
		group := groupOf(transactionID, op)
		persistedByGroup[group] = append(persistedByGroup[group], leg)
		allPersisted = append(allPersisted, leg)
	}

	resolved := make([]*operation.Operation, len(rebuilt))
	copy(resolved, rebuilt)

	adopted := 0

	for _, group := range groupOrder {
		candidates := persistedByGroup[group]
		if len(candidates) == 0 {
			continue
		}

		for _, indexes := range byAmount(rebuilt, rebuiltByGroup[group]) {
			count, ok := adoptExactLegs(rebuilt, resolved, indexes, candidates)
			if !ok {
				return mismatch(ReplayMismatchCountMismatch)
			}

			adopted += count
		}
	}

	// One check covers rules 3 and 4: an unpaired persisted leg in a group with
	// rebuilt legs shares that group's balance and starting version.
	for _, leg := range allPersisted {
		if !leg.paired && rebuiltPositions[positionOf(groupOf(transactionID, leg.op))] {
			return mismatch(ReplayMismatchUnpairedPersisted)
		}
	}

	return ReconcileResult{
		Verdict:    ReplayReconciliationResolved,
		Operations: resolved,
		Adopted:    adopted,
		Inserted:   len(rebuilt) - adopted,
	}
}

// adoptExactLegs pairs the rebuilt legs at indexes, which share one group and
// one amount, with the group's persisted legs of that exact amount. No persisted
// match leaves them missing; as many matches as legs gives each the persisted ID
// in ID order, without mutating the rebuilt leg; any other count reports false.
func adoptExactLegs(rebuilt, resolved []*operation.Operation, indexes []int, candidates []*persistedLeg) (int, bool) {
	amount := *rebuilt[indexes[0]].Amount.Value

	matches := make([]*persistedLeg, 0, len(indexes))

	for _, leg := range candidates {
		if leg.op.Amount.Value != nil && leg.op.Amount.Value.Equal(amount) {
			matches = append(matches, leg)
		}
	}

	if len(matches) == 0 {
		return 0, true
	}

	if len(matches) != len(indexes) {
		return 0, false
	}

	sort.SliceStable(matches, func(a, b int) bool { return matches[a].op.ID < matches[b].op.ID })

	for k, index := range indexes {
		adoptedLeg := *rebuilt[index]
		adoptedLeg.ID = matches[k].op.ID
		resolved[index] = &adoptedLeg
		matches[k].paired = true
	}

	return len(indexes), true
}

func groupOf(transactionID string, op *operation.Operation) legGroup {
	balanceKey := op.BalanceKey
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	return legGroup{
		transactionID: transactionID,
		balanceID:     op.BalanceID,
		balanceKey:    balanceKey,
		opType:        op.Type,
		versionBefore: *op.Balance.Version,
	}
}

func positionOf(group legGroup) legPosition {
	return legPosition{balanceID: group.balanceID, balanceKey: group.balanceKey, versionBefore: group.versionBefore}
}

// byAmount splits one group's rebuilt legs by exact decimal amount, keeping the
// rebuilt order inside each amount and the order amounts first appear.
func byAmount(rebuilt []*operation.Operation, indexes []int) [][]int {
	var (
		amounts []decimal.Decimal
		split   [][]int
	)

	for _, index := range indexes {
		amount := *rebuilt[index].Amount.Value
		position := -1

		for i, seen := range amounts {
			if seen.Equal(amount) {
				position = i

				break
			}
		}

		if position < 0 {
			amounts = append(amounts, amount)
			split = append(split, nil)
			position = len(split) - 1
		}

		split[position] = append(split[position], index)
	}

	return split
}
