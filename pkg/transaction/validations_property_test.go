// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"math/rand"
	"testing"

	constant "github.com/LerianStudio/lib-commons/v3/commons/constants"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// validOperations is the complete set of known operation types returned by DetermineOperation.
var validOperations = map[string]bool{
	constant.DEBIT:   true,
	constant.CREDIT:  true,
	constant.ONHOLD:  true,
	constant.RELEASE: true,
}

// validDirections is the complete set of known direction values returned by DetermineOperation.
var validDirections = map[string]bool{
	constant.DEBIT:  true,
	constant.CREDIT: true,
}

// knownTransactionTypes are the transaction types recognized by the system.
var knownTransactionTypes = []string{
	constant.PENDING,
	constant.APPROVED,
	constant.CANCELED,
	constant.CREATED,
}

// arbitraryTransactionTypes includes known types plus fuzz strings to test the default branch.
var arbitraryTransactionTypes = append(
	knownTransactionTypes,
	"", "UNKNOWN", "settled", "REVERSED", "partial", "123",
)

// knownOperationCombos lists all (Operation, TransactionType) pairs that produce a balance change.
var knownOperationCombos = []struct {
	Operation       string
	TransactionType string
}{
	{constant.ONHOLD, constant.PENDING},
	{constant.RELEASE, constant.CANCELED},
	{constant.DEBIT, constant.APPROVED},
	{constant.CREDIT, constant.APPROVED},
	{constant.DEBIT, constant.CREATED},
	{constant.CREDIT, constant.CREATED},
}

const propertyIterations = 1000

// TestProperty_DetermineOperation_AlwaysValidPairs verifies that for any combination of
// (isPending, isFrom, transactionType), DetermineOperation always returns a pair from
// the known set of valid (operation, direction) values.
func TestProperty_DetermineOperation_AlwaysValidPairs(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(42))

	for i := range propertyIterations {
		isPending := rng.Intn(2) == 1
		isFrom := rng.Intn(2) == 1
		txType := arbitraryTransactionTypes[rng.Intn(len(arbitraryTransactionTypes))]

		operation, direction := DetermineOperation(isPending, isFrom, txType)

		assert.True(t, validOperations[operation],
			"iteration %d: unexpected operation %q for isPending=%v isFrom=%v txType=%q",
			i, operation, isPending, isFrom, txType)

		assert.True(t, validDirections[direction],
			"iteration %d: unexpected direction %q for isPending=%v isFrom=%v txType=%q",
			i, direction, isPending, isFrom, txType)
	}
}

// TestProperty_DetermineOperation_DirectionConsistency verifies that DEBIT and RELEASE
// always produce "debit" direction, while CREDIT and ONHOLD always produce "credit" direction.
func TestProperty_DetermineOperation_DirectionConsistency(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(99))

	for i := range propertyIterations {
		isPending := rng.Intn(2) == 1
		isFrom := rng.Intn(2) == 1
		txType := arbitraryTransactionTypes[rng.Intn(len(arbitraryTransactionTypes))]

		operation, direction := DetermineOperation(isPending, isFrom, txType)

		switch operation {
		case constant.DEBIT, constant.RELEASE:
			assert.Equal(t, constant.DEBIT, direction,
				"iteration %d: %s must have debit direction, got %q (isPending=%v isFrom=%v txType=%q)",
				i, operation, direction, isPending, isFrom, txType)
		case constant.CREDIT, constant.ONHOLD:
			assert.Equal(t, constant.CREDIT, direction,
				"iteration %d: %s must have credit direction, got %q (isPending=%v isFrom=%v txType=%q)",
				i, operation, direction, isPending, isFrom, txType)
		}
	}
}

// TestProperty_OperateBalances_VersionIncrement verifies that for any known operation combo,
// the version always increases by exactly the expected increment:
//   - 2 when Operation==ONHOLD, TransactionType==PENDING, RouteValidationEnabled==true
//   - 1 for all other known combos
//   - 0 (unchanged) for unknown combos
func TestProperty_OperateBalances_VersionIncrement(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(77))

	for i := range propertyIterations {
		startVersion := int64(rng.Intn(1000))
		value := decimal.NewFromInt(int64(rng.Intn(500) + 1))
		available := decimal.NewFromInt(int64(rng.Intn(10000) + 1000))
		onHold := decimal.NewFromInt(int64(rng.Intn(500)))
		routeEnabled := rng.Intn(2) == 1

		balance := Balance{
			Available: available,
			OnHold:    onHold,
			Version:   startVersion,
		}

		combo := knownOperationCombos[rng.Intn(len(knownOperationCombos))]

		amount := Amount{
			Value:                  value,
			Operation:              combo.Operation,
			TransactionType:        combo.TransactionType,
			RouteValidationEnabled: routeEnabled,
		}

		result, err := OperateBalances(amount, balance)
		assert.NoError(t, err, "iteration %d", i)

		if combo.Operation == constant.ONHOLD &&
			combo.TransactionType == constant.PENDING &&
			routeEnabled {
			assert.Equal(t, startVersion+2, result.Version,
				"iteration %d: ONHOLD+PENDING+RouteEnabled should increment version by 2 (start=%d got=%d)",
				i, startVersion, result.Version)
		} else {
			assert.Equal(t, startVersion+1, result.Version,
				"iteration %d: %s+%s should increment version by 1 (start=%d got=%d)",
				i, combo.Operation, combo.TransactionType, startVersion, result.Version)
		}
	}
}

// TestProperty_OperateBalances_VersionUnchangedForUnknownOps verifies that unknown
// operation/transactionType combinations leave the version unchanged.
func TestProperty_OperateBalances_VersionUnchangedForUnknownOps(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(55))
	unknownOps := []string{"UNKNOWN", "SETTLE", "REVERSE", ""}
	unknownTypes := []string{"UNKNOWN", "SETTLED", "PARTIAL", ""}

	for i := range propertyIterations {
		startVersion := int64(rng.Intn(1000))
		value := decimal.NewFromInt(int64(rng.Intn(500) + 1))

		balance := Balance{
			Available: decimal.NewFromInt(int64(rng.Intn(10000) + 1000)),
			OnHold:    decimal.NewFromInt(int64(rng.Intn(500))),
			Version:   startVersion,
		}

		amount := Amount{
			Value:           value,
			Operation:       unknownOps[rng.Intn(len(unknownOps))],
			TransactionType: unknownTypes[rng.Intn(len(unknownTypes))],
		}

		result, err := OperateBalances(amount, balance)
		assert.NoError(t, err, "iteration %d", i)
		assert.Equal(t, startVersion, result.Version,
			"iteration %d: unknown op %q+%q should not change version",
			i, amount.Operation, amount.TransactionType)
		assert.True(t, balance.Available.Equal(result.Available),
			"iteration %d: unknown op should not change Available", i)
		assert.True(t, balance.OnHold.Equal(result.OnHold),
			"iteration %d: unknown op should not change OnHold", i)
	}
}

// TestProperty_OperateBalances_Conservation verifies the exact balance changes for each
// known operation combo. For every operation, the specific fields that should change
// are verified by exact amount, and fields that should not change remain unchanged.
func TestProperty_OperateBalances_Conservation(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(123))

	for i := range propertyIterations {
		value := decimal.NewFromInt(int64(rng.Intn(500) + 1))
		available := decimal.NewFromInt(int64(rng.Intn(10000) + 1000))
		onHold := decimal.NewFromInt(int64(rng.Intn(500) + 500))

		balance := Balance{
			Available: available,
			OnHold:    onHold,
			Version:   int64(rng.Intn(100)),
		}

		combo := knownOperationCombos[rng.Intn(len(knownOperationCombos))]

		amount := Amount{
			Value:           value,
			Operation:       combo.Operation,
			TransactionType: combo.TransactionType,
		}

		result, err := OperateBalances(amount, balance)
		assert.NoError(t, err, "iteration %d: %s+%s", i, combo.Operation, combo.TransactionType)

		switch {
		case combo.Operation == constant.DEBIT && combo.TransactionType == constant.CREATED:
			// Available decreases by Value, OnHold unchanged
			assert.True(t, result.Available.Equal(available.Sub(value)),
				"iteration %d: DEBIT+CREATED Available should decrease by %s", i, value)
			assert.True(t, result.OnHold.Equal(onHold),
				"iteration %d: DEBIT+CREATED OnHold should be unchanged", i)

		case combo.Operation == constant.CREDIT && combo.TransactionType == constant.CREATED:
			// Available increases by Value, OnHold unchanged
			assert.True(t, result.Available.Equal(available.Add(value)),
				"iteration %d: CREDIT+CREATED Available should increase by %s", i, value)
			assert.True(t, result.OnHold.Equal(onHold),
				"iteration %d: CREDIT+CREATED OnHold should be unchanged", i)

		case combo.Operation == constant.ONHOLD && combo.TransactionType == constant.PENDING:
			// Available decreases, OnHold increases -- net zero across Available+OnHold
			availableDelta := available.Sub(result.Available)
			onHoldDelta := result.OnHold.Sub(onHold)
			assert.True(t, availableDelta.Equal(value),
				"iteration %d: ONHOLD+PENDING Available should decrease by %s", i, value)
			assert.True(t, onHoldDelta.Equal(value),
				"iteration %d: ONHOLD+PENDING OnHold should increase by %s", i, value)
			assert.True(t, availableDelta.Equal(onHoldDelta),
				"iteration %d: ONHOLD+PENDING conservation violated: Available delta != OnHold delta", i)

		case combo.Operation == constant.RELEASE && combo.TransactionType == constant.CANCELED:
			// Available increases, OnHold decreases -- net zero across Available+OnHold
			availableDelta := result.Available.Sub(available)
			onHoldDelta := onHold.Sub(result.OnHold)
			assert.True(t, availableDelta.Equal(value),
				"iteration %d: RELEASE+CANCELED Available should increase by %s", i, value)
			assert.True(t, onHoldDelta.Equal(value),
				"iteration %d: RELEASE+CANCELED OnHold should decrease by %s", i, value)
			assert.True(t, availableDelta.Equal(onHoldDelta),
				"iteration %d: RELEASE+CANCELED conservation violated", i)

		case combo.Operation == constant.DEBIT && combo.TransactionType == constant.APPROVED:
			// OnHold decreases by Value, Available unchanged
			assert.True(t, result.Available.Equal(available),
				"iteration %d: DEBIT+APPROVED Available should be unchanged", i)
			assert.True(t, result.OnHold.Equal(onHold.Sub(value)),
				"iteration %d: DEBIT+APPROVED OnHold should decrease by %s", i, value)

		case combo.Operation == constant.CREDIT && combo.TransactionType == constant.APPROVED:
			// Available increases by Value, OnHold unchanged
			assert.True(t, result.Available.Equal(available.Add(value)),
				"iteration %d: CREDIT+APPROVED Available should increase by %s", i, value)
			assert.True(t, result.OnHold.Equal(onHold),
				"iteration %d: CREDIT+APPROVED OnHold should be unchanged", i)
		}
	}
}

// TestProperty_OperateBalances_NoPanic verifies that OperateBalances never panics
// regardless of input values, including edge cases like zero, negative, and very large values.
func TestProperty_OperateBalances_NoPanic(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(256))

	edgeValues := []decimal.Decimal{
		decimal.Zero,
		decimal.NewFromInt(-1),
		decimal.NewFromInt(-999999),
		decimal.NewFromInt(1),
		decimal.NewFromInt(999999999),
		decimal.NewFromFloat(0.001),
		decimal.NewFromFloat(99999999.99),
	}

	allOps := []string{
		constant.DEBIT, constant.CREDIT, constant.ONHOLD, constant.RELEASE,
		"", "UNKNOWN", "NULL", "DROP TABLE",
	}

	allTypes := []string{
		constant.PENDING, constant.APPROVED, constant.CANCELED, constant.CREATED,
		"", "UNKNOWN", "SETTLED", "bogus",
	}

	for i := range propertyIterations {
		var value decimal.Decimal
		if rng.Intn(3) == 0 {
			// Use edge case value
			value = edgeValues[rng.Intn(len(edgeValues))]
		} else {
			value = decimal.NewFromInt(int64(rng.Int63n(1000000) - 500000))
		}

		var available decimal.Decimal
		if rng.Intn(3) == 0 {
			available = edgeValues[rng.Intn(len(edgeValues))]
		} else {
			available = decimal.NewFromInt(int64(rng.Int63n(1000000) - 500000))
		}

		var onHold decimal.Decimal
		if rng.Intn(3) == 0 {
			onHold = edgeValues[rng.Intn(len(edgeValues))]
		} else {
			onHold = decimal.NewFromInt(int64(rng.Int63n(1000000) - 500000))
		}

		balance := Balance{
			Available: available,
			OnHold:    onHold,
			Version:   int64(rng.Int63n(1000000) - 500000),
		}

		amount := Amount{
			Value:                  value,
			Operation:              allOps[rng.Intn(len(allOps))],
			TransactionType:        allTypes[rng.Intn(len(allTypes))],
			RouteValidationEnabled: rng.Intn(2) == 1,
		}

		// The primary assertion: this must not panic
		assert.NotPanics(t, func() {
			_, _ = OperateBalances(amount, balance)
		}, "iteration %d: OperateBalances panicked with op=%q type=%q value=%s available=%s onHold=%s",
			i, amount.Operation, amount.TransactionType, value, available, onHold)
	}
}

// TestProperty_OperateBalances_OnHoldPendingConservation specifically tests the
// conservation-of-value invariant for ONHOLD+PENDING: the sum of (Available + OnHold)
// must remain constant before and after the operation.
func TestProperty_OperateBalances_OnHoldPendingConservation(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(314))

	for i := range propertyIterations {
		value := decimal.NewFromInt(int64(rng.Intn(5000) + 1))
		available := decimal.NewFromInt(int64(rng.Intn(50000) + 5000))
		onHold := decimal.NewFromInt(int64(rng.Intn(5000)))
		routeEnabled := rng.Intn(2) == 1

		balance := Balance{
			Available: available,
			OnHold:    onHold,
			Version:   int64(rng.Intn(100)),
		}

		amount := Amount{
			Value:                  value,
			Operation:              constant.ONHOLD,
			TransactionType:        constant.PENDING,
			RouteValidationEnabled: routeEnabled,
		}

		totalBefore := balance.Available.Add(balance.OnHold)

		result, err := OperateBalances(amount, balance)
		assert.NoError(t, err, "iteration %d", i)

		totalAfter := result.Available.Add(result.OnHold)
		assert.True(t, totalBefore.Equal(totalAfter),
			"iteration %d: conservation violated for ONHOLD+PENDING (routeEnabled=%v): "+
				"before(Available=%s + OnHold=%s = %s) != after(Available=%s + OnHold=%s = %s)",
			i, routeEnabled,
			available, onHold, totalBefore,
			result.Available, result.OnHold, totalAfter)
	}
}
