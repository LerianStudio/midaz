// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
)

// FuzzDetermineOperation fuzzes the DetermineOperation function with arbitrary
// combinations of isPending, isFrom, and transactionType. It verifies that
// every returned (operation, direction) pair is drawn from the known valid set
// and that the pairing between operation and direction is consistent.
func FuzzDetermineOperation(f *testing.F) {
	// Seed corpus: all known valid combinations from the table-driven tests.
	f.Add(true, true, constant.PENDING)   // ONHOLD, credit
	f.Add(true, false, constant.PENDING)  // CREDIT, credit
	f.Add(true, true, constant.CANCELED)  // RELEASE, debit
	f.Add(true, false, constant.CANCELED) // default fallback
	f.Add(true, true, constant.APPROVED)  // DEBIT, debit
	f.Add(true, false, constant.APPROVED) // CREDIT, credit
	f.Add(false, true, constant.CREATED)  // DEBIT, debit
	f.Add(false, false, constant.CREATED) // CREDIT, credit
	// Edge cases: empty and arbitrary strings.
	f.Add(false, false, "")
	f.Add(true, true, "UNKNOWN")
	f.Add(false, true, "PENDING")
	f.Add(true, false, "CREATED")

	validOperations := map[string]bool{
		constant.DEBIT:   true,
		constant.CREDIT:  true,
		constant.ONHOLD:  true,
		constant.RELEASE: true,
	}

	validDirections := map[string]bool{
		constant.DirectionDebit:  true,
		constant.DirectionCredit: true,
	}

	// The operation-to-direction mapping enforced by the implementation.
	expectedDirection := map[string]string{
		constant.DEBIT:   constant.DirectionDebit,
		constant.CREDIT:  constant.DirectionCredit,
		constant.ONHOLD:  constant.DirectionDebit,
		constant.RELEASE: constant.DirectionCredit,
	}

	// maxStringLen bounds fuzzer-generated strings to prevent resource exhaustion.
	const maxStringLen = 64

	f.Fuzz(func(t *testing.T, isPending bool, isFrom bool, transactionType string) {
		if len(transactionType) > maxStringLen {
			transactionType = transactionType[:maxStringLen]
		}

		operation, direction := DetermineOperation(isPending, isFrom, transactionType)

		if !validOperations[operation] {
			t.Errorf("DetermineOperation(%v, %v, %q) returned invalid operation %q",
				isPending, isFrom, transactionType, operation)
		}

		if !validDirections[direction] {
			t.Errorf("DetermineOperation(%v, %v, %q) returned invalid direction %q",
				isPending, isFrom, transactionType, direction)
		}

		if want, ok := expectedDirection[operation]; ok && direction != want {
			t.Errorf("DetermineOperation(%v, %v, %q) returned operation=%q with direction=%q, want direction=%q",
				isPending, isFrom, transactionType, operation, direction, want)
		}
	})
}

// FuzzOperateBalances fuzzes the OperateBalances function with arbitrary amount
// values, operations, transaction types, balance states, and the RouteValidationEnabled
// flag. It verifies:
//   - No panics on any input
//   - Known operations always increment version by 1
//   - Balance math is conserved or correctly applied per operation branch
//   - Double-entry operations (route on) correctly affect only one field each
func FuzzOperateBalances(f *testing.F) {
	// Seed corpus entries: (amountVal, available, onHold, version, operation, transactionType, routeValidation)
	// DEBIT + PENDING (route on -> Available-- only, version+1)
	f.Add(int64(100), int64(1000), int64(0), int64(1), constant.DEBIT, constant.PENDING, true)
	// ONHOLD + PENDING (route off -> legacy: Available-- AND OnHold++)
	f.Add(int64(100), int64(1000), int64(0), int64(1), constant.ONHOLD, constant.PENDING, false)
	// ONHOLD + PENDING (route on -> OnHold++ only, version+1)
	f.Add(int64(100), int64(1000), int64(0), int64(1), constant.ONHOLD, constant.PENDING, true)
	// RELEASE + CANCELED (legacy, route off -> Available+OnHold conserved)
	f.Add(int64(50), int64(500), int64(50), int64(3), constant.RELEASE, constant.CANCELED, false)
	// RELEASE + CANCELED (route on -> OnHold-- only, Available unchanged, version+1)
	f.Add(int64(50), int64(500), int64(50), int64(3), constant.RELEASE, constant.CANCELED, true)
	// CREDIT + CANCELED (route on -> Available++ only, version+1)
	f.Add(int64(50), int64(500), int64(50), int64(3), constant.CREDIT, constant.CANCELED, true)
	// CREDIT + CANCELED (route off -> falls through to default, no change)
	f.Add(int64(50), int64(500), int64(50), int64(3), constant.CREDIT, constant.CANCELED, false)
	// DEBIT + APPROVED (legacy: only reduces onHold)
	f.Add(int64(50), int64(100), int64(50), int64(5), constant.DEBIT, constant.APPROVED, false)
	// ONHOLD + APPROVED (route on -> reduces onHold, same math as DEBIT)
	f.Add(int64(50), int64(100), int64(50), int64(5), constant.ONHOLD, constant.APPROVED, true)
	// CREDIT + APPROVED (adds to available)
	f.Add(int64(30), int64(100), int64(0), int64(2), constant.CREDIT, constant.APPROVED, false)
	// DEBIT + CREATED (subtracts from available)
	f.Add(int64(50), int64(100), int64(10), int64(1), constant.DEBIT, constant.CREATED, false)
	// CREDIT + CREATED (adds to available)
	f.Add(int64(50), int64(100), int64(10), int64(1), constant.CREDIT, constant.CREATED, false)
	// Unknown operation (no version change)
	f.Add(int64(50), int64(100), int64(10), int64(5), "UNKNOWN", "UNKNOWN", false)
	// Zero amount
	f.Add(int64(0), int64(0), int64(0), int64(0), constant.DEBIT, constant.CREATED, false)
	// Large values
	f.Add(int64(999999999), int64(999999999), int64(999999999), int64(0), constant.CREDIT, constant.CREATED, true)
	// Negative amount (edge case the fuzzer should explore)
	f.Add(int64(-100), int64(1000), int64(0), int64(1), constant.DEBIT, constant.CREATED, false)

	// maxStringLen bounds fuzzer-generated strings to prevent resource exhaustion.
	const maxStringLen = 64

	f.Fuzz(func(t *testing.T, amountVal, available, onHold, version int64,
		operation, transactionType string, routeValidation bool,
	) {
		// Bound string inputs to prevent OOM from extremely large fuzzer strings.
		if len(operation) > maxStringLen {
			operation = operation[:maxStringLen]
		}

		if len(transactionType) > maxStringLen {
			transactionType = transactionType[:maxStringLen]
		}

		amount := Amount{
			Value:                  decimal.NewFromInt(amountVal),
			Operation:              operation,
			TransactionType:        transactionType,
			RouteValidationEnabled: routeValidation,
		}

		balance := Balance{
			Available: decimal.NewFromInt(available),
			OnHold:    decimal.NewFromInt(onHold),
			Version:   version,
		}

		result, err := OperateBalances(amount, balance)
		if err != nil {
			// OperateBalances currently never returns an error, but if it does
			// in the future, that is acceptable -- just not a panic.
			return
		}

		// Determine which implementation branch this input hits.
		// The switch in OperateBalances evaluates in order, so we replicate
		// that precedence here to know exactly which branch fired.
		isDebitPendingRoute := operation == constant.DEBIT && transactionType == constant.PENDING && routeValidation
		isOnHoldPending := operation == constant.ONHOLD && transactionType == constant.PENDING
		isReleaseCanceled := operation == constant.RELEASE && transactionType == constant.CANCELED
		isCreditCanceledRoute := operation == constant.CREDIT && transactionType == constant.CANCELED && routeValidation
		isDebitApproved := operation == constant.DEBIT && transactionType == constant.APPROVED
		isOnHoldApproved := operation == constant.ONHOLD && transactionType == constant.APPROVED
		isCreditApproved := operation == constant.CREDIT && transactionType == constant.APPROVED
		isDebitCreated := operation == constant.DEBIT && transactionType == constant.CREATED
		isCreditCreated := operation == constant.CREDIT && transactionType == constant.CREATED

		isKnown := isDebitPendingRoute || isOnHoldPending || isReleaseCanceled || isCreditCanceledRoute ||
			isDebitApproved || isOnHoldApproved || isCreditApproved || isDebitCreated || isCreditCreated

		if !isKnown {
			// Unknown operation: balance must be returned unchanged.
			if !result.Available.Equal(balance.Available) {
				t.Errorf("unknown op %q+%q (route=%v) changed Available: %s -> %s",
					operation, transactionType, routeValidation, balance.Available, result.Available)
			}

			if !result.OnHold.Equal(balance.OnHold) {
				t.Errorf("unknown op %q+%q (route=%v) changed OnHold: %s -> %s",
					operation, transactionType, routeValidation, balance.OnHold, result.OnHold)
			}

			if result.Version != balance.Version {
				t.Errorf("unknown op %q+%q (route=%v) changed Version: %d -> %d",
					operation, transactionType, routeValidation, balance.Version, result.Version)
			}

			return
		}

		// Known operation: version must always increment by exactly 1.
		if result.Version != version+1 {
			t.Errorf("op %q+%q (route=%v): version=%d, want %d",
				operation, transactionType, routeValidation, result.Version, version+1)
		}

		// Correctness checks per branch.
		amountDec := decimal.NewFromInt(amountVal)
		availableDec := decimal.NewFromInt(available)
		onHoldDec := decimal.NewFromInt(onHold)
		originalSum := availableDec.Add(onHoldDec)
		resultSum := result.Available.Add(result.OnHold)

		switch {
		case isDebitPendingRoute:
			// Double-entry: DEBIT+PENDING only decrements Available. OnHold unchanged.
			if !result.Available.Equal(availableDec.Sub(amountDec)) {
				t.Errorf("DEBIT+PENDING (route=true) Available: got %s, want %s",
					result.Available, availableDec.Sub(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec) {
				t.Errorf("DEBIT+PENDING (route=true) changed OnHold: got %s, want %s",
					result.OnHold, onHoldDec)
			}
		case isOnHoldPending && routeValidation:
			// Double-entry: ON_HOLD+PENDING only increments OnHold. Available unchanged.
			if !result.Available.Equal(availableDec) {
				t.Errorf("ONHOLD+PENDING (route=true) Available: got %s, want %s (unchanged)",
					result.Available, availableDec)
			}

			if !result.OnHold.Equal(onHoldDec.Add(amountDec)) {
				t.Errorf("ONHOLD+PENDING (route=true) OnHold: got %s, want %s",
					result.OnHold, onHoldDec.Add(amountDec))
			}
		case isOnHoldPending:
			// Legacy: Available+OnHold should be conserved (funds move between them).
			if !resultSum.Equal(originalSum) {
				t.Errorf("ONHOLD+PENDING broke conservation: original=%s, result=%s",
					originalSum, resultSum)
			}

			if !result.Available.Equal(availableDec.Sub(amountDec)) {
				t.Errorf("ONHOLD+PENDING Available: got %s, want %s",
					result.Available, availableDec.Sub(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec.Add(amountDec)) {
				t.Errorf("ONHOLD+PENDING OnHold: got %s, want %s",
					result.OnHold, onHoldDec.Add(amountDec))
			}
		case isReleaseCanceled && routeValidation:
			// Double-entry: RELEASE only decrements OnHold. Available stays unchanged.
			if !result.Available.Equal(availableDec) {
				t.Errorf("RELEASE+CANCELED (route=true) Available: got %s, want %s (unchanged)",
					result.Available, availableDec)
			}

			if !result.OnHold.Equal(onHoldDec.Sub(amountDec)) {
				t.Errorf("RELEASE+CANCELED (route=true) OnHold: got %s, want %s",
					result.OnHold, onHoldDec.Sub(amountDec))
			}
		case isReleaseCanceled:
			// Legacy: Available+OnHold should be conserved (funds move between them).
			if !resultSum.Equal(originalSum) {
				t.Errorf("RELEASE+CANCELED (legacy) broke conservation: original=%s, result=%s",
					originalSum, resultSum)
			}

			if !result.OnHold.Equal(onHoldDec.Sub(amountDec)) {
				t.Errorf("RELEASE+CANCELED (legacy) OnHold: got %s, want %s",
					result.OnHold, onHoldDec.Sub(amountDec))
			}

			if !result.Available.Equal(availableDec.Add(amountDec)) {
				t.Errorf("RELEASE+CANCELED (legacy) Available: got %s, want %s",
					result.Available, availableDec.Add(amountDec))
			}
		case isCreditCanceledRoute:
			// Double-entry: CREDIT+CANCELED adds to Available only; OnHold unchanged.
			if !result.Available.Equal(availableDec.Add(amountDec)) {
				t.Errorf("CREDIT+CANCELED (route=true) Available: got %s, want %s",
					result.Available, availableDec.Add(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec) {
				t.Errorf("CREDIT+CANCELED (route=true) changed OnHold: got %s, want %s",
					result.OnHold, onHoldDec)
			}
		case isDebitApproved, isOnHoldApproved:
			// Only OnHold changes, Available stays the same.
			// Both DEBIT (legacy) and ON_HOLD (route validation) reduce onHold.
			if !result.Available.Equal(availableDec) {
				t.Errorf("%s+APPROVED changed Available: got %s, want %s",
					operation, result.Available, availableDec)
			}

			if !result.OnHold.Equal(onHoldDec.Sub(amountDec)) {
				t.Errorf("%s+APPROVED OnHold: got %s, want %s",
					operation, result.OnHold, onHoldDec.Sub(amountDec))
			}
		case isCreditApproved:
			// Only Available changes, OnHold stays the same.
			if !result.Available.Equal(availableDec.Add(amountDec)) {
				t.Errorf("CREDIT+APPROVED Available: got %s, want %s",
					result.Available, availableDec.Add(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec) {
				t.Errorf("CREDIT+APPROVED changed OnHold: got %s, want %s",
					result.OnHold, onHoldDec)
			}
		case isDebitCreated:
			// Available decreases, OnHold unchanged.
			if !result.Available.Equal(availableDec.Sub(amountDec)) {
				t.Errorf("DEBIT+CREATED Available: got %s, want %s",
					result.Available, availableDec.Sub(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec) {
				t.Errorf("DEBIT+CREATED changed OnHold: got %s, want %s",
					result.OnHold, onHoldDec)
			}
		case isCreditCreated:
			// Available increases, OnHold unchanged.
			if !result.Available.Equal(availableDec.Add(amountDec)) {
				t.Errorf("CREDIT+CREATED Available: got %s, want %s",
					result.Available, availableDec.Add(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec) {
				t.Errorf("CREDIT+CREATED changed OnHold: got %s, want %s",
					result.OnHold, onHoldDec)
			}
		}
	})
}
