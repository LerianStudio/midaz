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

type operationMovementKey struct {
	PostingRef string
	Role       string
	Ordinal    uint32
}

type operationLifecycle struct {
	Before engine.BalanceState
	After  engine.BalanceState
}

// BuildOperationRecordsFromMovements projects one transaction's recorded movements.
// It is shared by normal completion and recovery and never reads live state.
// Row order follows the ordered operation specs; balances and result are not mutated.
func BuildOperationRecordsFromMovements(payload TransactionCompletionPlan, result engine.Result) ([]*operation.Operation, error) {
	if err := validateTransactionCompletionPlan(payload); err != nil {
		return nil, err
	}

	movements, err := validateOperationMovementResult(payload, result)
	if err != nil {
		return nil, err
	}

	lifecycles := operationLifecycles(payload, result.Movements)
	rows := make([]*operation.Operation, 0, len(result.Movements))

	for _, context := range payload.OperationSpecs {
		key := operationMovementKey{context.PostingRef, context.Role, context.Ordinal}

		movement, exists := movements[key]
		if !exists {
			continue
		}

		before, after, amount := movement.Before, movement.After, movement.Amount
		lifecycle := lifecycles[key]

		switch context.CompatibilityPath {
		case OperationRecordValidatedCancelRelease:
			before, after, amount = projectValidatedCancelRelease(context, lifecycle.Before)
		case OperationRecordValidatedCancelCredit:
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

func projectValidatedCancelRelease(context OperationRecordSpec, before engine.BalanceState) (engine.BalanceState, engine.BalanceState, decimal.Decimal) {
	after := before
	after.OnHold = before.OnHold.Sub(context.RequestedAmount)
	after.Version++

	return before, after, context.RequestedAmount
}

func projectValidatedCancelCredit(context OperationRecordSpec, initial engine.BalanceState) (engine.BalanceState, engine.BalanceState, decimal.Decimal) {
	_, before, amount := projectValidatedCancelRelease(context, initial)
	after := before
	after.Available = before.Available.Add(amount)
	after.Version++

	return before, after, amount
}

func operationOrigin(context OperationRecordSpec) string {
	if context.OriginRef != "" {
		return "\x01" + context.OriginRef
	}

	return "\x00" + context.PostingRef
}

func operationLifecycles(payload TransactionCompletionPlan, movements []engine.Movement) map[operationMovementKey]operationLifecycle {
	contexts := make(map[operationMovementKey]OperationRecordSpec, len(payload.OperationSpecs))
	for _, context := range payload.OperationSpecs {
		contexts[operationMovementKey{context.PostingRef, context.Role, context.Ordinal}] = context
	}

	groups := make(map[string]operationLifecycle)
	ordinals := make(map[operationMovementKey]uint32)

	for _, movement := range movements {
		if movement.Role != engine.RolePrimary {
			continue
		}

		base := operationMovementKey{PostingRef: movement.PostingRef, Role: movement.Role}
		key := base
		key.Ordinal = ordinals[base]
		ordinals[base]++
		context := contexts[key]
		origin := operationOrigin(context)

		lifecycle, seen := groups[origin]
		if !seen {
			lifecycle.Before = movement.Before
		}

		lifecycle.After = movement.After
		groups[origin] = lifecycle
	}

	byContext := make(map[operationMovementKey]operationLifecycle, len(contexts))

	for _, context := range payload.OperationSpecs {
		key := operationMovementKey{context.PostingRef, context.Role, context.Ordinal}
		if context.Role == engine.RoleOverdraftCompanion {
			context = contexts[operationMovementKey{PostingRef: context.PostingRef, Role: engine.RolePrimary}]
		}

		byContext[key] = groups[operationOrigin(context)]
	}

	return byContext
}

func validateOperationMovementResult(payload TransactionCompletionPlan, result engine.Result) (map[operationMovementKey]engine.Movement, error) {
	if result.Movements == nil || result.Final == nil {
		return nil, invalidTransactionCompletionRecord("result arrays must not be null")
	}

	contexts, identities, err := indexOperationRecordSpecs(payload.OperationSpecs)
	if err != nil {
		return nil, err
	}

	if err := validateOperationRecordAttribution(contexts); err != nil {
		return nil, err
	}

	movements := make(map[operationMovementKey]engine.Movement, len(result.Movements))
	ordinals := make(map[operationMovementKey]uint32)
	last := make(map[string]engine.BalanceState)
	seen := make(map[string]bool)

	for _, movement := range result.Movements {
		base := operationMovementKey{PostingRef: movement.PostingRef, Role: movement.Role}
		key := base
		key.Ordinal = ordinals[base]
		ordinals[base]++

		context, exists := contexts[key]
		if !exists || movement.TransactionID != payload.TransactionID || movement.Ref == "" || seen[movement.Ref] || context.BalanceRef != movement.BalanceRef {
			return nil, invalidTransactionCompletionRecord("invalid per-transaction movement correlation")
		}

		seen[movement.Ref] = true
		if err := validateOperationMovement(movement, context.RequestedAmount); err != nil {
			return nil, err
		}

		if previous, hasPrevious := last[movement.BalanceRef]; hasPrevious && !sameOperationState(previous, movement.Before) {
			return nil, invalidTransactionCompletionRecord("broken movement state chain")
		}

		last[movement.BalanceRef] = movement.After
		movements[key] = movement
	}

	if err := validateOperationRecordCompleteness(contexts, movements); err != nil {
		return nil, err
	}

	if err := validateOperationFinal(result.Final, last, identities); err != nil {
		return nil, err
	}

	return movements, nil
}

func indexOperationRecordSpecs(projections []OperationRecordSpec) (map[operationMovementKey]OperationRecordSpec, map[string]OperationBalanceContext, error) {
	contexts := make(map[operationMovementKey]OperationRecordSpec, len(projections))
	identities := make(map[string]OperationBalanceContext)

	for _, context := range projections {
		contexts[operationMovementKey{context.PostingRef, context.Role, context.Ordinal}] = context
		if existing, exists := identities[context.BalanceRef]; exists && (existing.ID != context.Balance.ID || existing.AccountID != context.Balance.AccountID) {
			return nil, nil, invalidTransactionCompletionRecord("conflicting operation spec balance identity")
		}

		identities[context.BalanceRef] = context.Balance
	}

	return contexts, identities, nil
}

func validateOperationMovement(movement engine.Movement, requested decimal.Decimal) error {
	if !movement.OverdraftDelta.Equal(movement.After.OverdraftUsed.Sub(movement.Before.OverdraftUsed)) {
		return invalidTransactionCompletionRecord("movement debt delta disagrees with recorded states")
	}

	if !validOperationRecordPostingType(movement.Type) || movement.Amount.GreaterThan(requested) {
		return invalidTransactionCompletionRecord("invalid movement type or requested amount")
	}

	if movement.Amount.IsNegative() || movement.Before.Version < 0 || movement.After.Version <= movement.Before.Version || movement.After.Version-movement.Before.Version != 1 || movement.Before.OnHold.IsNegative() || movement.After.OnHold.IsNegative() || movement.Before.OverdraftUsed.IsNegative() || movement.After.OverdraftUsed.IsNegative() {
		return invalidTransactionCompletionRecord("invalid movement amount, state or version")
	}

	return nil
}

func validateOperationRecordCompleteness(contexts map[operationMovementKey]OperationRecordSpec, movements map[operationMovementKey]engine.Movement) error {
	for key, context := range contexts {
		if _, exists := movements[key]; !exists && context.Role == engine.RolePrimary {
			return invalidTransactionCompletionRecord("primary operation spec has no movement")
		}

		if context.Role == engine.RoleOverdraftCompanion {
			if _, exists := contexts[operationMovementKey{PostingRef: context.PostingRef, Role: engine.RolePrimary}]; !exists {
				return invalidTransactionCompletionRecord("companion operation spec has no primary")
			}
		}
	}

	for key, movement := range movements {
		if movement.Role == engine.RoleOverdraftCompanion {
			primary, exists := movements[operationMovementKey{PostingRef: key.PostingRef, Role: engine.RolePrimary, Ordinal: key.Ordinal}]

			expectedType := engine.PostingCredit
			if primary.OverdraftDelta.IsPositive() {
				expectedType = engine.PostingDebit
			}

			if !exists || primary.OverdraftDelta.IsZero() || movement.Type != expectedType {
				return invalidTransactionCompletionRecord("companion movement disagrees with primary debt change")
			}
		}

		if movement.Role != engine.RolePrimary || movement.OverdraftDelta.IsZero() {
			continue
		}

		companion, exists := movements[operationMovementKey{PostingRef: key.PostingRef, Role: engine.RoleOverdraftCompanion, Ordinal: key.Ordinal}]
		if !exists || !companion.Amount.Equal(movement.OverdraftDelta.Abs()) || !companion.OverdraftDelta.IsZero() {
			return invalidTransactionCompletionRecord("debt change has no matching companion movement")
		}
	}

	return nil
}

func validateOperationFinal(finals []engine.BalanceSnapshot, last map[string]engine.BalanceState, identities map[string]OperationBalanceContext) error {
	if len(last) != len(finals) {
		return invalidTransactionCompletionRecord("final snapshots do not match touched balances")
	}

	finalSeen := make(map[string]bool, len(finals))
	for _, final := range finals {
		state, exists := last[final.BalanceRef]

		actual := engine.BalanceState{Available: final.Available, OnHold: final.OnHold, OverdraftUsed: final.OverdraftUsed, Version: final.Version}
		if !exists || finalSeen[final.BalanceRef] || !sameOperationState(state, actual) {
			return invalidTransactionCompletionRecord("final snapshot disagrees with recorded movements")
		}

		identity := identities[final.BalanceRef]
		if final.ID.String() != identity.ID || final.AccountID.String() != identity.AccountID || final.Alias != mtransaction.SplitAlias(identity.Alias) || final.Key != identity.Key || final.AssetCode != identity.AssetCode || final.AccountType != identity.AccountType {
			return invalidTransactionCompletionRecord("final snapshot identity mismatch")
		}

		finalSeen[final.BalanceRef] = true
	}

	return nil
}

func validateOperationRecordAttribution(contexts map[operationMovementKey]OperationRecordSpec) error {
	origins := make(map[string]OperationRecordSpec)

	for _, context := range contexts {
		if context.Role == engine.RoleOverdraftCompanion {
			primary, exists := contexts[operationMovementKey{PostingRef: context.PostingRef, Role: engine.RolePrimary}]
			if !exists || primary.Balance.AccountID != context.Balance.AccountID || primary.Balance.AssetCode != context.Balance.AssetCode || mtransaction.SplitAlias(primary.Balance.Alias) != mtransaction.SplitAlias(context.Balance.Alias) || !sameOperationRoute(primary.RouteID, context.RouteID) {
				return invalidTransactionCompletionRecord("companion attribution disagrees with primary")
			}

			continue
		}

		if context.OriginRef == "" {
			continue
		}

		if first, exists := origins[context.OriginRef]; exists {
			if first.BalanceRef != context.BalanceRef || !first.RequestedAmount.Equal(context.RequestedAmount) || !sameOperationRoute(first.RouteID, context.RouteID) {
				return invalidTransactionCompletionRecord("operation spec origin has inconsistent attribution")
			}
		} else {
			origins[context.OriginRef] = context
		}
	}

	return nil
}

func sameOperationRoute(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}

	return *left == *right
}

func validOperationRecordPostingType(postingType engine.PostingType) bool {
	switch postingType {
	case engine.PostingDebit, engine.PostingCredit, engine.PostingReserve, engine.PostingUnreserve, engine.PostingHold, engine.PostingRelease:
		return true
	default:
		return false
	}
}

func sameOperationState(left, right engine.BalanceState) bool {
	return left.Version == right.Version && left.Available.Equal(right.Available) && left.OnHold.Equal(right.OnHold) && left.OverdraftUsed.Equal(right.OverdraftUsed)
}
