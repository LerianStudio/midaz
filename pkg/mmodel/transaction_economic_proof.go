// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// ValidateRedisTransactionEconomicEffect proves that materialized operation
// rows describe the immutable before/after movement written by the balance Lua.
// Operation IDs are deliberately excluded from the comparison: the first
// successful CAS chooses them, while every money field must already be implied
// by the Lua snapshots and the immutable transaction input.
func ValidateRedisTransactionEconomicEffect(envelope *TransactionRedisQueue, operations []OperationRedis) error {
	if envelope == nil || envelope.TransactionID == uuid.Nil || envelope.OrganizationID == uuid.Nil || envelope.LedgerID == uuid.Nil {
		return fmt.Errorf("complete transaction economic envelope is required")
	}
	if len(operations) == 0 || len(envelope.Balances) == 0 || len(envelope.Balances) != len(envelope.BalancesAfter) {
		return fmt.Errorf("complete operation and balance movement sets are required")
	}
	if !RedisBalanceSetEconomicComplete(envelope.Balances) || !RedisBalanceSetEconomicComplete(envelope.BalancesAfter) {
		return fmt.Errorf("transaction balance movement is incomplete")
	}
	for _, operation := range operations {
		if !RedisOperationEconomicComplete(operation) || operation.TransactionID != envelope.TransactionID.String() ||
			operation.OrganizationID != envelope.OrganizationID.String() || operation.LedgerID != envelope.LedgerID.String() ||
			!operation.BalanceAffected {
			return fmt.Errorf("transaction economic operation %q is incomplete", operation.ID)
		}
		if envelope.TransactionInput.Send.Asset != "" && operation.AssetCode != envelope.TransactionInput.Send.Asset {
			return fmt.Errorf("transaction economic operation %q asset differs from immutable input", operation.ID)
		}
		allowed, inputMatched := redisAllowedOperationSemantics(envelope, operation)
		if !inputMatched || !allowed.matches(operation) {
			return fmt.Errorf("transaction economic operation %q semantics differ from immutable input", operation.ID)
		}
	}

	used := make([]bool, len(operations))
	if !matchRedisEconomicBalanceMovements(envelope.Balances, envelope.BalancesAfter, operations, 0, used) {
		return fmt.Errorf("transaction operations do not reconstruct the immutable balance movement")
	}

	return nil
}

// RedisOperationSetEconomicEqualIgnoringIDs compares the complete operation
// multiset while leaving the single-assignment identity out. It is used only
// after both sides independently proved the immutable balance movement.
func RedisOperationSetEconomicEqualIgnoringIDs(left, right []OperationRedis) bool {
	if len(left) != len(right) {
		return false
	}
	used := make([]bool, len(right))
	for _, candidate := range left {
		matched := false
		for index, canonical := range right {
			if used[index] || !redisOperationEconomicEqualIgnoringID(candidate, canonical) {
				continue
			}
			used[index] = true
			matched = true
			break
		}
		if !matched {
			return false
		}
	}

	return true
}

func redisOperationEconomicEqualIgnoringID(left, right OperationRedis) bool {
	return left.TransactionID == right.TransactionID && left.BalanceID == right.BalanceID &&
		left.BalanceKey == right.BalanceKey && left.AccountID == right.AccountID &&
		left.OrganizationID == right.OrganizationID && left.LedgerID == right.LedgerID &&
		left.Type == right.Type && left.Direction == right.Direction && left.AssetCode == right.AssetCode &&
		left.BalanceAffected == right.BalanceAffected && left.AmountValue.Equal(right.AmountValue) &&
		left.BalanceAvailable.Equal(right.BalanceAvailable) && left.BalanceOnHold.Equal(right.BalanceOnHold) &&
		left.BalanceVersion == right.BalanceVersion &&
		left.BalanceAfterAvailable.Equal(right.BalanceAfterAvailable) &&
		left.BalanceAfterOnHold.Equal(right.BalanceAfterOnHold) &&
		left.BalanceAfterVersion == right.BalanceAfterVersion &&
		redisEconomicDecimalEqual(left.Snapshot.OverdraftUsedBefore, right.Snapshot.OverdraftUsedBefore) &&
		redisEconomicDecimalEqual(left.Snapshot.OverdraftUsedAfter, right.Snapshot.OverdraftUsedAfter)
}

type redisOperationSemantics map[string]map[string]struct{}

func (s redisOperationSemantics) add(operationType, direction string) {
	if operationType == "" || direction == "" {
		return
	}
	if s[operationType] == nil {
		s[operationType] = make(map[string]struct{})
	}
	s[operationType][direction] = struct{}{}
}

func (s redisOperationSemantics) matches(operation OperationRedis) bool {
	_, ok := s[operation.Type][operation.Direction]

	return ok
}

func redisAllowedOperationSemantics(
	envelope *TransactionRedisQueue,
	operation OperationRedis,
) (redisOperationSemantics, bool) {
	allowed := make(redisOperationSemantics)
	if envelope.Validate == nil {
		return allowed, false
	}
	balance, found := redisEconomicBalanceForOperation(envelope.Balances, operation)
	if !found {
		return allowed, false
	}
	inputMatched := false
	add := func(amount mtransaction.Amount) {
		inputMatched = true
		operationType := amount.Operation
		if envelope.TransactionInput.OperationTypeOverride != "" {
			operationType = envelope.TransactionInput.OperationTypeOverride
		}
		allowed.add(operationType, amount.Direction)
		allowed.add(constant.OVERDRAFT, amount.Direction)
		if amount.RouteValidationEnabled && amount.TransactionType == constant.PENDING {
			allowed.add(constant.DEBIT, constant.DirectionDebit)
			allowed.add(constant.ONHOLD, constant.DirectionCredit)
		}
		if amount.RouteValidationEnabled && amount.TransactionType == constant.CANCELED {
			allowed.add(constant.RELEASE, constant.DirectionDebit)
			allowed.add(constant.CREDIT, constant.DirectionCredit)
		}
	}
	for key, amount := range envelope.Validate.From {
		if redisEconomicInputTargetsBalance(key, balance) {
			add(amount)
		}
	}
	for key, amount := range envelope.Validate.To {
		if redisEconomicInputTargetsBalance(key, balance) {
			add(amount)
		}
	}

	return allowed, inputMatched
}

func redisEconomicBalanceForOperation(balances []BalanceRedis, operation OperationRedis) (BalanceRedis, bool) {
	for _, balance := range balances {
		if balance.ID == operation.BalanceID && balance.Key == operation.BalanceKey &&
			balance.AccountID == operation.AccountID && balance.AssetCode == operation.AssetCode {
			return balance, true
		}
	}

	return BalanceRedis{}, false
}

func redisEconomicInputTargetsBalance(key string, balance BalanceRedis) bool {
	if key == balance.ID {
		return true
	}
	target := balance.Alias
	if !strings.HasSuffix(target, "#"+balance.Key) {
		target = mtransaction.AliasKey(target, balance.Key)
	}
	normalized := mtransaction.SplitAliasWithKey(key)
	legacyFullKey := strings.Contains(balance.Key, "#") && (key == balance.Key || normalized == balance.Key)

	return key == target || normalized == target || legacyFullKey
}

func matchRedisEconomicBalanceMovements(
	before, after []BalanceRedis,
	operations []OperationRedis,
	pair int,
	used []bool,
) bool {
	if pair == len(before) {
		for _, consumed := range used {
			if !consumed {
				return false
			}
		}

		return true
	}
	if !redisBalanceIdentityAndPolicyEqual(before[pair], after[pair]) {
		return false
	}
	paths := redisEconomicOperationPaths(before[pair], after[pair], before, after, operations, used)
	for _, path := range paths {
		for _, index := range path {
			used[index] = true
		}
		if matchRedisEconomicBalanceMovements(before, after, operations, pair+1, used) {
			return true
		}
		for _, index := range path {
			used[index] = false
		}
	}

	return false
}

func redisEconomicOperationPaths(
	before, after BalanceRedis,
	allBefore, allAfter []BalanceRedis,
	operations []OperationRedis,
	used []bool,
) [][]int {
	var paths [][]int
	var visit func(decimal.Decimal, decimal.Decimal, int64, []int)
	visit = func(available, onHold decimal.Decimal, version int64, path []int) {
		if len(path) > 0 && available.Equal(after.Available) && onHold.Equal(after.OnHold) && version == after.Version {
			paths = append(paths, append([]int(nil), path...))
			return
		}
		if len(path) >= len(operations) {
			return
		}
		for index, operation := range operations {
			if used[index] || containsOperationIndex(path, index) ||
				!redisOperationStartsAt(operation, before, available, onHold, version) ||
				!redisOperationMovementExact(operation) ||
				!redisOperationSnapshotMatches(operation, before, after, allBefore, allAfter) {
				continue
			}
			visit(operation.BalanceAfterAvailable, operation.BalanceAfterOnHold,
				operation.BalanceAfterVersion, append(path, index))
		}
	}
	visit(before.Available, before.OnHold, before.Version, nil)

	return paths
}

func containsOperationIndex(indices []int, candidate int) bool {
	for _, index := range indices {
		if index == candidate {
			return true
		}
	}

	return false
}

func redisOperationStartsAt(
	operation OperationRedis,
	balance BalanceRedis,
	available, onHold decimal.Decimal,
	version int64,
) bool {
	return operation.BalanceID == balance.ID && operation.BalanceKey == balance.Key &&
		operation.AccountID == balance.AccountID && operation.AssetCode == balance.AssetCode &&
		operation.BalanceAvailable.Equal(available) && operation.BalanceOnHold.Equal(onHold) &&
		operation.BalanceVersion == version
}

func redisOperationMovementExact(operation OperationRedis) bool {
	availableDelta := operation.BalanceAfterAvailable.Sub(operation.BalanceAvailable).Abs()
	onHoldDelta := operation.BalanceAfterOnHold.Sub(operation.BalanceOnHold).Abs()
	movement := availableDelta
	if onHoldDelta.GreaterThan(movement) {
		movement = onHoldDelta
	}
	if movement.IsZero() {
		before, beforeErr := decimal.NewFromString(operation.Snapshot.OverdraftUsedBefore)
		after, afterErr := decimal.NewFromString(operation.Snapshot.OverdraftUsedAfter)
		if beforeErr != nil || afterErr != nil {
			return false
		}
		movement = after.Sub(before).Abs()
	}

	return movement.Equal(operation.AmountValue)
}

func redisOperationSnapshotMatches(
	operation OperationRedis,
	before, after BalanceRedis,
	allBefore, allAfter []BalanceRedis,
) bool {
	if redisEconomicDecimalEqual(operation.Snapshot.OverdraftUsedBefore, before.OverdraftUsed) &&
		redisEconomicDecimalEqual(operation.Snapshot.OverdraftUsedAfter, after.OverdraftUsed) {
		return true
	}
	for index := range allBefore {
		if allBefore[index].AccountID == before.AccountID && allBefore[index].Key == constant.DefaultBalanceKey &&
			allAfter[index].AccountID == after.AccountID && allAfter[index].Key == constant.DefaultBalanceKey &&
			redisEconomicDecimalEqual(operation.Snapshot.OverdraftUsedBefore, allBefore[index].OverdraftUsed) &&
			redisEconomicDecimalEqual(operation.Snapshot.OverdraftUsedAfter, allAfter[index].OverdraftUsed) {
			return true
		}
	}

	return false
}

func redisBalanceIdentityAndPolicyEqual(left, right BalanceRedis) bool {
	return left.ID == right.ID && left.Alias == right.Alias && left.Key == right.Key &&
		left.AccountID == right.AccountID && left.AssetCode == right.AssetCode &&
		left.AccountType == right.AccountType && left.AllowSending == right.AllowSending &&
		left.AllowReceiving == right.AllowReceiving && left.Direction == right.Direction &&
		left.AllowOverdraft == right.AllowOverdraft &&
		left.OverdraftLimitEnabled == right.OverdraftLimitEnabled &&
		redisEconomicDecimalEqual(left.OverdraftLimit, right.OverdraftLimit) &&
		left.BalanceScope == right.BalanceScope
}
