package property

import (
	"math/rand"
	"testing"
	"testing/quick"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
)

// Property: OperateBalances version always increases (monotonic) when operation changes balance
func TestProperty_BalanceVersionMonotonic(t *testing.T) {
	f := func(initialVersion int64, value int64, isDebit bool) bool {
		// Constrain to valid versions
		if initialVersion < 1 {
			initialVersion = 1
		}
		if initialVersion > 1_000_000 {
			initialVersion = 1_000_000
		}

		// Constrain value
		if value < 0 {
			value = -value
		}
		if value == 0 {
			value = 1
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(1000), // Enough for debits
			OnHold:    decimal.Zero,
			Version:   initialVersion,
		}

		operation := constant.CREDIT
		if isDebit {
			operation = constant.DEBIT
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(value),
			Operation:       operation,
			TransactionType: constant.CREATED,
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			t.Logf("OperateBalances error: %v", err)
			return true // Skip errored cases
		}

		// Property: new version should be exactly initialVersion + 1
		expectedVersion := initialVersion + 1
		if newBalance.Version != expectedVersion {
			t.Logf("Version not monotonic: initial=%d expected=%d got=%d",
				initialVersion, expectedVersion, newBalance.Version)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Balance version monotonic property failed: %v", err)
	}
}

// Property: DEBIT subtracts from Available, CREDIT adds to Available
func TestProperty_OperateBalancesDebitCredit(t *testing.T) {
	f := func(seed int64, initialAvail, operationValue int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Constrain values
		if initialAvail < 0 {
			initialAvail = -initialAvail
		}
		if operationValue < 0 {
			operationValue = -operationValue
		}
		if operationValue == 0 {
			operationValue = 1
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(initialAvail),
			OnHold:    decimal.Zero,
			Version:   1,
		}

		isDebit := rng.Intn(2) == 0
		operation := constant.CREDIT
		if isDebit {
			operation = constant.DEBIT
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(operationValue),
			Operation:       operation,
			TransactionType: constant.CREATED,
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			return true // Skip errors
		}

		expectedAvail := balance.Available
		if isDebit {
			expectedAvail = expectedAvail.Sub(amount.Value)
		} else {
			expectedAvail = expectedAvail.Add(amount.Value)
		}

		if !newBalance.Available.Equal(expectedAvail) {
			t.Logf("Available mismatch: op=%s initial=%s value=%s expected=%s got=%s",
				operation, balance.Available, amount.Value, expectedAvail, newBalance.Available)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("OperateBalances DEBIT/CREDIT property failed: %v", err)
	}
}

// Property: ONHOLD (PENDING transactions) move funds from Available to OnHold
func TestProperty_OperateBalancesOnHold(t *testing.T) {
	f := func(initialAvail, initialOnHold, value int64) bool {
		// Constrain values
		if initialAvail < 0 {
			initialAvail = -initialAvail
		}
		if initialOnHold < 0 {
			initialOnHold = -initialOnHold
		}
		if value <= 0 {
			value = 1
		}

		// Ensure enough available balance for the hold
		if initialAvail < value {
			initialAvail = value + 100
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(initialAvail),
			OnHold:    decimal.NewFromInt(initialOnHold),
			Version:   1,
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(value),
			Operation:       constant.ONHOLD,
			TransactionType: constant.PENDING,
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			return true // Skip errors
		}

		// Property: Available decreases, OnHold increases by same amount
		expectedAvail := balance.Available.Sub(amount.Value)
		expectedOnHold := balance.OnHold.Add(amount.Value)

		if !newBalance.Available.Equal(expectedAvail) {
			t.Logf("ONHOLD Available mismatch: expected=%s got=%s", expectedAvail, newBalance.Available)
			return false
		}

		if !newBalance.OnHold.Equal(expectedOnHold) {
			t.Logf("ONHOLD OnHold mismatch: expected=%s got=%s", expectedOnHold, newBalance.OnHold)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("OperateBalances ONHOLD property failed: %v", err)
	}
}

// Property: RELEASE reverses ONHOLD - funds move back from OnHold to Available
func TestProperty_OperateBalancesRelease(t *testing.T) {
	f := func(initialAvail, initialOnHold, value int64) bool {
		// Constrain values
		if initialAvail < 0 {
			initialAvail = -initialAvail
		}
		if initialOnHold < 0 {
			initialOnHold = -initialOnHold
		}
		if value <= 0 {
			value = 1
		}

		// Ensure enough on hold for the release
		if initialOnHold < value {
			initialOnHold = value + 100
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(initialAvail),
			OnHold:    decimal.NewFromInt(initialOnHold),
			Version:   1,
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(value),
			Operation:       constant.RELEASE,
			TransactionType: constant.CANCELED,
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			return true // Skip errors
		}

		// Property: Available increases, OnHold decreases by same amount
		expectedAvail := balance.Available.Add(amount.Value)
		expectedOnHold := balance.OnHold.Sub(amount.Value)

		if !newBalance.Available.Equal(expectedAvail) {
			t.Logf("RELEASE Available mismatch: expected=%s got=%s", expectedAvail, newBalance.Available)
			return false
		}

		if !newBalance.OnHold.Equal(expectedOnHold) {
			t.Logf("RELEASE OnHold mismatch: expected=%s got=%s", expectedOnHold, newBalance.OnHold)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("OperateBalances RELEASE property failed: %v", err)
	}
}

// Property: Total funds (Available + OnHold) is conserved across ONHOLD/RELEASE operations
func TestProperty_BalanceTotalConserved(t *testing.T) {
	f := func(initialAvail, initialOnHold, value int64, isPending bool) bool {
		// Constrain values
		if initialAvail < 0 {
			initialAvail = -initialAvail
		}
		if initialOnHold < 0 {
			initialOnHold = -initialOnHold
		}
		if value <= 0 {
			value = 1
		}

		// Ensure balance can handle the operation
		if isPending && initialAvail < value {
			initialAvail = value + 100
		}
		if !isPending && initialOnHold < value {
			initialOnHold = value + 100
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(initialAvail),
			OnHold:    decimal.NewFromInt(initialOnHold),
			Version:   1,
		}

		initialTotal := balance.Available.Add(balance.OnHold)

		var amount transaction.Amount
		if isPending {
			amount = transaction.Amount{
				Value:           decimal.NewFromInt(value),
				Operation:       constant.ONHOLD,
				TransactionType: constant.PENDING,
			}
		} else {
			amount = transaction.Amount{
				Value:           decimal.NewFromInt(value),
				Operation:       constant.RELEASE,
				TransactionType: constant.CANCELED,
			}
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			return true // Skip errors
		}

		newTotal := newBalance.Available.Add(newBalance.OnHold)

		// Property: total should be conserved
		if !initialTotal.Equal(newTotal) {
			t.Logf("Total not conserved: initial=%s new=%s op=%s",
				initialTotal, newTotal, amount.Operation)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Balance total conservation property failed: %v", err)
	}
}
