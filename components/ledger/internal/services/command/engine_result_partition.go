// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// PartitionEngineResult derives one ordered completion result per prepared
// transaction from the engine's compact global result. Movement order is never
// reconstructed from maps: each transaction owns one contiguous range and each
// final set follows that transaction's first-touch order.
func PartitionEngineResult(prepared PreparedEngineExecution, result accounting.ExecutionResult) ([]accounting.ExecutionResult, error) {
	if err := validatePreparedEngineExecution(prepared); err != nil {
		return nil, err
	}

	return partitionValidatedEngineResult(prepared, result)
}

func partitionValidatedEngineResult(prepared PreparedEngineExecution, result accounting.ExecutionResult) ([]accounting.ExecutionResult, error) {
	if result.Movements == nil || result.Final == nil {
		return nil, invalidTransactionCompletionRecord("engine result arrays must not be null")
	}

	request := prepared.Execution.Execution
	transactionIndices := make(map[uuid.UUID]int, len(request.Transactions))
	partitions := make([]accounting.ExecutionResult, len(request.Transactions))
	transactionTouches := make([][]string, len(request.Transactions))
	transactionLast := make([]map[string]accounting.BalanceState, len(request.Transactions))

	for index, transaction := range request.Transactions {
		transactionIndices[transaction.ID] = index
		partitions[index] = accounting.ExecutionResult{
			Movements: make([]accounting.Movement, 0),
			Final:     make([]accounting.BalanceSnapshot, 0),
		}
		transactionLast[index] = make(map[string]accounting.BalanceState)
	}

	balances, err := indexEngineResultBalances(request.Balances)
	if err != nil {
		return nil, err
	}

	globalTouches := make([]string, 0, len(result.Final))
	globalLast := make(map[string]accounting.BalanceState, len(result.Final))
	movementRefs := make(map[string]struct{}, len(result.Movements))
	previousTransactionIndex := -1

	for _, movement := range result.Movements {
		transactionIndex, exists := transactionIndices[movement.TransactionID]
		if !exists {
			return nil, invalidTransactionCompletionRecord("engine movement belongs to an unknown transaction")
		}

		if transactionIndex < previousTransactionIndex {
			return nil, invalidTransactionCompletionRecord("engine transaction movement ranges are interleaved or unordered")
		}

		if _, balanceExists := balances[movement.BalanceRef]; !balanceExists {
			return nil, invalidTransactionCompletionRecord("engine movement references an unknown balance")
		}

		if movement.Ref == "" {
			return nil, invalidTransactionCompletionRecord("engine movement has no identity")
		}

		if _, duplicate := movementRefs[movement.Ref]; duplicate {
			return nil, invalidTransactionCompletionRecord("engine movement identity is duplicated")
		}

		movementRefs[movement.Ref] = struct{}{}
		previousTransactionIndex = transactionIndex
		partitions[transactionIndex].Movements = append(partitions[transactionIndex].Movements, movement)

		if previous, touched := globalLast[movement.BalanceRef]; touched {
			if !sameOperationState(previous, movement.Before) {
				return nil, invalidTransactionCompletionRecord("engine movement state chain is discontinuous")
			}
		} else {
			globalTouches = append(globalTouches, movement.BalanceRef)
		}

		globalLast[movement.BalanceRef] = movement.After

		if _, touched := transactionLast[transactionIndex][movement.BalanceRef]; !touched {
			transactionTouches[transactionIndex] = append(transactionTouches[transactionIndex], movement.BalanceRef)
		}

		transactionLast[transactionIndex][movement.BalanceRef] = movement.After
	}

	finals, err := validateGlobalEngineFinal(result.Final, globalTouches, globalLast, balances)
	if err != nil {
		return nil, err
	}

	for transactionIndex := range partitions {
		for _, balanceRef := range transactionTouches[transactionIndex] {
			snapshot := finals[balanceRef]
			state := transactionLast[transactionIndex][balanceRef]
			snapshot.Available = state.Available
			snapshot.OnHold = state.OnHold
			snapshot.OverdraftUsed = state.OverdraftUsed
			snapshot.Version = state.Version
			partitions[transactionIndex].Final = append(partitions[transactionIndex].Final, snapshot)
		}

		if _, err := validateOperationMovementResult(prepared.CompletionPlans[transactionIndex], partitions[transactionIndex]); err != nil {
			return nil, err
		}
	}

	return partitions, nil
}

func indexEngineResultBalances(balances []accounting.BalanceSnapshot) (map[string]accounting.BalanceSnapshot, error) {
	byRef := make(map[string]accounting.BalanceSnapshot, len(balances))
	byID := make(map[uuid.UUID]struct{}, len(balances))

	for _, balance := range balances {
		if balance.BalanceRef == "" || balance.ID == uuid.Nil || balance.AccountID == uuid.Nil {
			return nil, invalidTransactionCompletionRecord("execution contains an invalid balance identity")
		}

		if _, duplicate := byRef[balance.BalanceRef]; duplicate {
			return nil, invalidTransactionCompletionRecord("execution contains a duplicate balance reference")
		}

		if _, duplicate := byID[balance.ID]; duplicate {
			return nil, invalidTransactionCompletionRecord("execution contains a duplicate balance identity")
		}

		byRef[balance.BalanceRef] = balance
		byID[balance.ID] = struct{}{}
	}

	return byRef, nil
}

func validateGlobalEngineFinal(
	final []accounting.BalanceSnapshot,
	firstTouch []string,
	last map[string]accounting.BalanceState,
	balances map[string]accounting.BalanceSnapshot,
) (map[string]accounting.BalanceSnapshot, error) {
	if len(final) != len(firstTouch) {
		return nil, invalidTransactionCompletionRecord("engine final snapshots do not match touched balances")
	}

	byRef := make(map[string]accounting.BalanceSnapshot, len(final))
	for index, snapshot := range final {
		balance, exists := balances[snapshot.BalanceRef]
		if !exists || snapshot.BalanceRef != firstTouch[index] {
			return nil, invalidTransactionCompletionRecord("engine final snapshot order does not match first touch")
		}

		if !sameEngineBalanceIdentity(balance, snapshot) {
			return nil, invalidTransactionCompletionRecord("engine final snapshot identity differs from execution balance")
		}

		state := accounting.BalanceState{
			Available:     snapshot.Available,
			OnHold:        snapshot.OnHold,
			OverdraftUsed: snapshot.OverdraftUsed,
			Version:       snapshot.Version,
		}
		if !sameOperationState(last[snapshot.BalanceRef], state) {
			return nil, invalidTransactionCompletionRecord("engine final snapshot disagrees with the last movement")
		}

		byRef[snapshot.BalanceRef] = snapshot
	}

	return byRef, nil
}

func sameEngineBalanceIdentity(left, right accounting.BalanceSnapshot) bool {
	return left.BalanceRef == right.BalanceRef && left.ID == right.ID && left.AccountID == right.AccountID &&
		left.AccountType == right.AccountType && left.AssetCode == right.AssetCode && left.Alias == right.Alias && left.Key == right.Key
}
