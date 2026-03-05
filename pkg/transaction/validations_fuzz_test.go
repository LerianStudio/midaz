// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"testing"

	constant "github.com/LerianStudio/lib-commons/v3/commons/constants"
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
		constant.DEBIT:  true,
		constant.CREDIT: true,
	}

	// The operation-to-direction mapping enforced by the implementation.
	expectedDirection := map[string]string{
		constant.DEBIT:   constant.DEBIT,
		constant.CREDIT:  constant.CREDIT,
		constant.ONHOLD:  constant.CREDIT,
		constant.RELEASE: constant.DEBIT,
	}

	f.Fuzz(func(t *testing.T, isPending bool, isFrom bool, transactionType string) {
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
//   - Known operations always increment version (by 1 or 2)
//   - RouteValidationEnabled only causes version+2 for ONHOLD+PENDING
//   - Balance math is conserved for ONHOLD and RELEASE (Available+OnHold invariant)
func FuzzOperateBalances(f *testing.F) {
	// Seed corpus entries: (amountVal, available, onHold, version, operation, transactionType, routeValidation)
	// ONHOLD + PENDING (route off)
	f.Add(int64(100), int64(1000), int64(0), int64(1), constant.ONHOLD, constant.PENDING, false)
	// ONHOLD + PENDING (route on -> version+2)
	f.Add(int64(100), int64(1000), int64(0), int64(1), constant.ONHOLD, constant.PENDING, true)
	// RELEASE + CANCELED
	f.Add(int64(50), int64(500), int64(50), int64(3), constant.RELEASE, constant.CANCELED, false)
	// DEBIT + APPROVED (only reduces onHold)
	f.Add(int64(50), int64(100), int64(50), int64(5), constant.DEBIT, constant.APPROVED, false)
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

	// The set of known operation+transactionType combos that trigger a version increment.
	type opKey struct {
		operation       string
		transactionType string
	}

	knownOps := map[opKey]bool{
		{constant.ONHOLD, constant.PENDING}:   true,
		{constant.RELEASE, constant.CANCELED}: true,
		{constant.DEBIT, constant.APPROVED}:   true,
		{constant.CREDIT, constant.APPROVED}:  true,
		{constant.DEBIT, constant.CREATED}:    true,
		{constant.CREDIT, constant.CREATED}:   true,
	}

	f.Fuzz(func(t *testing.T, amountVal, available, onHold, version int64,
		operation, transactionType string, routeValidation bool,
	) {
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

		key := opKey{operation, transactionType}
		isKnown := knownOps[key]

		if !isKnown {
			// Unknown operation: balance must be returned unchanged.
			if !result.Available.Equal(balance.Available) {
				t.Errorf("unknown op %q+%q changed Available: %s -> %s",
					operation, transactionType, balance.Available, result.Available)
			}

			if !result.OnHold.Equal(balance.OnHold) {
				t.Errorf("unknown op %q+%q changed OnHold: %s -> %s",
					operation, transactionType, balance.OnHold, result.OnHold)
			}

			if result.Version != balance.Version {
				t.Errorf("unknown op %q+%q changed Version: %d -> %d",
					operation, transactionType, balance.Version, result.Version)
			}

			return
		}

		// Known operation: version must increment.
		isOnHoldPending := operation == constant.ONHOLD && transactionType == constant.PENDING

		if isOnHoldPending && routeValidation {
			if result.Version != version+2 {
				t.Errorf("ONHOLD+PENDING with RouteValidation: version=%d, want %d",
					result.Version, version+2)
			}
		} else {
			if result.Version != version+1 {
				t.Errorf("op %q+%q (route=%v): version=%d, want %d",
					operation, transactionType, routeValidation, result.Version, version+1)
			}
		}

		// Conservation checks for operations that move funds between Available and OnHold.
		amountDec := decimal.NewFromInt(amountVal)
		availableDec := decimal.NewFromInt(available)
		onHoldDec := decimal.NewFromInt(onHold)
		originalSum := availableDec.Add(onHoldDec)
		resultSum := result.Available.Add(result.OnHold)

		switch {
		case isOnHoldPending:
			// Available+OnHold should be conserved (funds move between them).
			if !resultSum.Equal(originalSum) {
				t.Errorf("ONHOLD+PENDING broke conservation: original=%s, result=%s",
					originalSum, resultSum)
			}

			// Available should decrease by amount, OnHold should increase by amount.
			if !result.Available.Equal(availableDec.Sub(amountDec)) {
				t.Errorf("ONHOLD+PENDING Available: got %s, want %s",
					result.Available, availableDec.Sub(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec.Add(amountDec)) {
				t.Errorf("ONHOLD+PENDING OnHold: got %s, want %s",
					result.OnHold, onHoldDec.Add(amountDec))
			}
		case operation == constant.RELEASE && transactionType == constant.CANCELED:
			// Available+OnHold should be conserved.
			if !resultSum.Equal(originalSum) {
				t.Errorf("RELEASE+CANCELED broke conservation: original=%s, result=%s",
					originalSum, resultSum)
			}
		case operation == constant.DEBIT && transactionType == constant.APPROVED:
			// Only OnHold changes, Available stays the same.
			if !result.Available.Equal(availableDec) {
				t.Errorf("DEBIT+APPROVED changed Available: got %s, want %s",
					result.Available, availableDec)
			}

			if !result.OnHold.Equal(onHoldDec.Sub(amountDec)) {
				t.Errorf("DEBIT+APPROVED OnHold: got %s, want %s",
					result.OnHold, onHoldDec.Sub(amountDec))
			}
		case operation == constant.CREDIT && transactionType == constant.APPROVED:
			// Only Available changes, OnHold stays the same.
			if !result.Available.Equal(availableDec.Add(amountDec)) {
				t.Errorf("CREDIT+APPROVED Available: got %s, want %s",
					result.Available, availableDec.Add(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec) {
				t.Errorf("CREDIT+APPROVED changed OnHold: got %s, want %s",
					result.OnHold, onHoldDec)
			}
		case operation == constant.DEBIT && transactionType == constant.CREATED:
			// Available decreases, OnHold unchanged.
			if !result.Available.Equal(availableDec.Sub(amountDec)) {
				t.Errorf("DEBIT+CREATED Available: got %s, want %s",
					result.Available, availableDec.Sub(amountDec))
			}

			if !result.OnHold.Equal(onHoldDec) {
				t.Errorf("DEBIT+CREATED changed OnHold: got %s, want %s",
					result.OnHold, onHoldDec)
			}
		case operation == constant.CREDIT && transactionType == constant.CREATED:
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
