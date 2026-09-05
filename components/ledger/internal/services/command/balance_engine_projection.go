// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"maps"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type projectionMovementKey struct {
	PostingRef string
	Role       string
	Ordinal    uint32
}

type projectionLifecycle struct {
	Before engine.BalanceState
	After  engine.BalanceState
}

// ProjectBalanceEngineOperations projects one transaction's recorded movements.
// It is shared by normal finalization and recovery and never reads live state.
// Row order follows the ordered frozen contexts; balances and result are not mutated.
func ProjectBalanceEngineOperations(payload BalanceEngineRecoveryPayload, result engine.Result) ([]*operation.Operation, error) {
	if err := validateRecoveryPayload(payload); err != nil {
		return nil, err
	}

	movements, err := validateProjectionResult(payload, result)
	if err != nil {
		return nil, err
	}

	lifecycles := projectionLifecycles(payload, result.Movements)
	rows := make([]*operation.Operation, 0, len(result.Movements))

	for _, context := range payload.Projection {
		key := projectionMovementKey{context.PostingRef, context.Role, context.Ordinal}

		movement, exists := movements[key]
		if !exists {
			continue
		}

		before, after, amount := movement.Before, movement.After, movement.Amount
		lifecycle := lifecycles[key]

		switch context.CompatibilityPath {
		case ProjectionValidatedCancelRelease:
			before, after, amount = projectValidatedCancelRelease(context, lifecycle.Before)
		case ProjectionValidatedCancelCredit:
			before, after, amount = projectValidatedCancelCredit(context, lifecycle.Before)
		}

		before.OverdraftUsed, after.OverdraftUsed = lifecycle.Before.OverdraftUsed, lifecycle.After.OverdraftUsed

		id, err := DeterministicOperationID(payload.ExecutionID, payload.TransactionID, context.PostingRef, context.Role, context.Ordinal)
		if err != nil {
			return nil, err
		}

		var routeID *string

		if context.RouteID != nil {
			routeValue := *context.RouteID
			routeID = &routeValue
		}

		rows = append(rows, &operation.Operation{
			ID: id.String(), TransactionID: payload.TransactionID.String(),
			OrganizationID: payload.OrganizationID.String(), LedgerID: payload.LedgerID.String(),
			Description: context.Description, Type: context.RowType, AssetCode: context.Balance.AssetCode,
			ChartOfAccounts: context.ChartOfAccounts, Metadata: maps.Clone(context.Metadata),
			Amount: operation.Amount{Value: &amount}, Balance: projectedOperationBalance(before), BalanceAfter: projectedOperationBalance(after),
			BalanceID: context.Balance.ID, AccountID: context.Balance.AccountID,
			AccountAlias: mtransaction.SplitAlias(context.Balance.Alias), AccountType: context.Balance.AccountType, BalanceKey: context.Balance.Key,
			RouteID: routeID, RouteCode: projectedOptionalText(context.RouteCode), RouteDescription: projectedOptionalText(context.RouteDescription),
			BalanceAffected: true, Direction: context.Direction,
			CreatedAt: payload.TransactionDate, UpdatedAt: payload.OperationUpdatedAt,
			Snapshot: mmodel.OperationSnapshot{OverdraftUsedBefore: lifecycle.Before.OverdraftUsed.String(), OverdraftUsedAfter: lifecycle.After.OverdraftUsed.String()},
		})
	}

	return rows, nil
}

func projectedOperationBalance(state engine.BalanceState) operation.Balance {
	return operation.Balance{Available: &state.Available, OnHold: &state.OnHold, Version: &state.Version, OverdraftUsed: state.OverdraftUsed}
}

func projectedOptionalText(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func projectValidatedCancelRelease(context FrozenProjectionContext, before engine.BalanceState) (engine.BalanceState, engine.BalanceState, decimal.Decimal) {
	after := before
	after.OnHold = before.OnHold.Sub(context.RequestedAmount)
	after.Version++

	return before, after, context.RequestedAmount
}

func projectValidatedCancelCredit(context FrozenProjectionContext, initial engine.BalanceState) (engine.BalanceState, engine.BalanceState, decimal.Decimal) {
	_, before, amount := projectValidatedCancelRelease(context, initial)
	after := before
	after.Available = before.Available.Add(amount)
	after.Version++

	return before, after, amount
}

func projectionOrigin(context FrozenProjectionContext) string {
	if context.OriginRef != "" {
		return "\x01" + context.OriginRef
	}

	return "\x00" + context.PostingRef
}

func projectionLifecycles(payload BalanceEngineRecoveryPayload, movements []engine.Movement) map[projectionMovementKey]projectionLifecycle {
	contexts := make(map[projectionMovementKey]FrozenProjectionContext, len(payload.Projection))
	for _, context := range payload.Projection {
		contexts[projectionMovementKey{context.PostingRef, context.Role, context.Ordinal}] = context
	}

	groups := make(map[string]projectionLifecycle)
	ordinals := make(map[projectionMovementKey]uint32)

	for _, movement := range movements {
		if movement.Role != engine.RolePrimary {
			continue
		}

		base := projectionMovementKey{PostingRef: movement.PostingRef, Role: movement.Role}
		key := base
		key.Ordinal = ordinals[base]
		ordinals[base]++
		context := contexts[key]
		origin := projectionOrigin(context)

		lifecycle, seen := groups[origin]
		if !seen {
			lifecycle.Before = movement.Before
		}

		lifecycle.After = movement.After
		groups[origin] = lifecycle
	}

	byContext := make(map[projectionMovementKey]projectionLifecycle, len(contexts))

	for _, context := range payload.Projection {
		key := projectionMovementKey{context.PostingRef, context.Role, context.Ordinal}
		if context.Role == engine.RoleOverdraftCompanion {
			context = contexts[projectionMovementKey{PostingRef: context.PostingRef, Role: engine.RolePrimary}]
		}

		byContext[key] = groups[projectionOrigin(context)]
	}

	return byContext
}

func validateProjectionResult(payload BalanceEngineRecoveryPayload, result engine.Result) (map[projectionMovementKey]engine.Movement, error) {
	if result.Movements == nil || result.Final == nil {
		return nil, invalidRecovery("result arrays must not be null")
	}

	contexts, identities, err := indexProjectionContexts(payload.Projection)
	if err != nil {
		return nil, err
	}

	if err := validateProjectionAttribution(contexts); err != nil {
		return nil, err
	}

	movements := make(map[projectionMovementKey]engine.Movement, len(result.Movements))
	ordinals := make(map[projectionMovementKey]uint32)
	last := make(map[string]engine.BalanceState)
	seen := make(map[string]bool)

	for _, movement := range result.Movements {
		base := projectionMovementKey{PostingRef: movement.PostingRef, Role: movement.Role}
		key := base
		key.Ordinal = ordinals[base]
		ordinals[base]++

		context, exists := contexts[key]
		if !exists || movement.TransactionID != payload.TransactionID || movement.Ref == "" || seen[movement.Ref] || context.BalanceRef != movement.BalanceRef {
			return nil, invalidRecovery("invalid per-transaction movement correlation")
		}

		seen[movement.Ref] = true
		if err := validateProjectionMovement(movement, context.RequestedAmount); err != nil {
			return nil, err
		}

		if previous, hasPrevious := last[movement.BalanceRef]; hasPrevious && !sameProjectionState(previous, movement.Before) {
			return nil, invalidRecovery("broken movement state chain")
		}

		last[movement.BalanceRef] = movement.After
		movements[key] = movement
	}

	if err := validateProjectionCompleteness(contexts, movements); err != nil {
		return nil, err
	}

	if err := validateProjectionFinal(result.Final, last, identities); err != nil {
		return nil, err
	}

	return movements, nil
}

func indexProjectionContexts(projections []FrozenProjectionContext) (map[projectionMovementKey]FrozenProjectionContext, map[string]FrozenProjectionBalance, error) {
	contexts := make(map[projectionMovementKey]FrozenProjectionContext, len(projections))
	identities := make(map[string]FrozenProjectionBalance)

	for _, context := range projections {
		contexts[projectionMovementKey{context.PostingRef, context.Role, context.Ordinal}] = context
		if existing, exists := identities[context.BalanceRef]; exists && (existing.ID != context.Balance.ID || existing.AccountID != context.Balance.AccountID) {
			return nil, nil, invalidRecovery("conflicting projection balance identity")
		}

		identities[context.BalanceRef] = context.Balance
	}

	return contexts, identities, nil
}

func validateProjectionMovement(movement engine.Movement, requested decimal.Decimal) error {
	if !movement.OverdraftDelta.Equal(movement.After.OverdraftUsed.Sub(movement.Before.OverdraftUsed)) {
		return invalidRecovery("movement debt delta disagrees with recorded states")
	}

	if !validProjectionPostingType(movement.Type) || movement.Amount.GreaterThan(requested) {
		return invalidRecovery("invalid movement type or requested amount")
	}

	if movement.Amount.IsNegative() || movement.Before.Version < 0 || movement.After.Version <= movement.Before.Version || movement.After.Version-movement.Before.Version != 1 || movement.Before.OnHold.IsNegative() || movement.After.OnHold.IsNegative() || movement.Before.OverdraftUsed.IsNegative() || movement.After.OverdraftUsed.IsNegative() {
		return invalidRecovery("invalid movement amount, state or version")
	}

	return nil
}

func validateProjectionCompleteness(contexts map[projectionMovementKey]FrozenProjectionContext, movements map[projectionMovementKey]engine.Movement) error {
	for key, context := range contexts {
		if _, exists := movements[key]; !exists && context.Role == engine.RolePrimary {
			return invalidRecovery("primary projection has no movement")
		}

		if context.Role == engine.RoleOverdraftCompanion {
			if _, exists := contexts[projectionMovementKey{PostingRef: context.PostingRef, Role: engine.RolePrimary}]; !exists {
				return invalidRecovery("companion projection has no primary")
			}
		}
	}

	for key, movement := range movements {
		if movement.Role == engine.RoleOverdraftCompanion {
			primary, exists := movements[projectionMovementKey{PostingRef: key.PostingRef, Role: engine.RolePrimary, Ordinal: key.Ordinal}]

			expectedType := engine.PostingCredit
			if primary.OverdraftDelta.IsPositive() {
				expectedType = engine.PostingDebit
			}

			if !exists || primary.OverdraftDelta.IsZero() || movement.Type != expectedType {
				return invalidRecovery("companion movement disagrees with primary debt change")
			}
		}

		if movement.Role != engine.RolePrimary || movement.OverdraftDelta.IsZero() {
			continue
		}

		companion, exists := movements[projectionMovementKey{PostingRef: key.PostingRef, Role: engine.RoleOverdraftCompanion, Ordinal: key.Ordinal}]
		if !exists || !companion.Amount.Equal(movement.OverdraftDelta.Abs()) || !companion.OverdraftDelta.IsZero() {
			return invalidRecovery("debt change has no matching companion movement")
		}
	}

	return nil
}

func validateProjectionFinal(finals []engine.BalanceSnapshot, last map[string]engine.BalanceState, identities map[string]FrozenProjectionBalance) error {
	if len(last) != len(finals) {
		return invalidRecovery("final snapshots do not match touched balances")
	}

	finalSeen := make(map[string]bool, len(finals))
	for _, final := range finals {
		state, exists := last[final.BalanceRef]

		actual := engine.BalanceState{Available: final.Available, OnHold: final.OnHold, OverdraftUsed: final.OverdraftUsed, Version: final.Version}
		if !exists || finalSeen[final.BalanceRef] || !sameProjectionState(state, actual) {
			return invalidRecovery("final snapshot disagrees with recorded movements")
		}

		identity := identities[final.BalanceRef]
		if final.ID.String() != identity.ID || final.AccountID.String() != identity.AccountID || final.Alias != mtransaction.SplitAlias(identity.Alias) || final.Key != identity.Key || final.AssetCode != identity.AssetCode || final.AccountType != identity.AccountType {
			return invalidRecovery("final snapshot identity mismatch")
		}

		finalSeen[final.BalanceRef] = true
	}

	return nil
}

func validateProjectionAttribution(contexts map[projectionMovementKey]FrozenProjectionContext) error {
	origins := make(map[string]FrozenProjectionContext)

	for _, context := range contexts {
		if context.Role == engine.RoleOverdraftCompanion {
			primary, exists := contexts[projectionMovementKey{PostingRef: context.PostingRef, Role: engine.RolePrimary}]
			if !exists || primary.Balance.AccountID != context.Balance.AccountID || primary.Balance.AssetCode != context.Balance.AssetCode || mtransaction.SplitAlias(primary.Balance.Alias) != mtransaction.SplitAlias(context.Balance.Alias) || !sameProjectionRoute(primary.RouteID, context.RouteID) {
				return invalidRecovery("companion attribution disagrees with primary")
			}

			continue
		}

		if context.OriginRef == "" {
			continue
		}

		if first, exists := origins[context.OriginRef]; exists {
			if first.BalanceRef != context.BalanceRef || !first.RequestedAmount.Equal(context.RequestedAmount) || !sameProjectionRoute(first.RouteID, context.RouteID) {
				return invalidRecovery("projection origin has inconsistent attribution")
			}
		} else {
			origins[context.OriginRef] = context
		}
	}

	return nil
}

func sameProjectionRoute(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}

	return *left == *right
}

func validProjectionPostingType(postingType engine.PostingType) bool {
	switch postingType {
	case engine.PostingDebit, engine.PostingCredit, engine.PostingReserve, engine.PostingUnreserve, engine.PostingHold, engine.PostingRelease:
		return true
	default:
		return false
	}
}

func sameProjectionState(left, right engine.BalanceState) bool {
	return left.Version == right.Version && left.Available.Equal(right.Available) && left.OnHold.Equal(right.OnHold) && left.OverdraftUsed.Equal(right.OverdraftUsed)
}
