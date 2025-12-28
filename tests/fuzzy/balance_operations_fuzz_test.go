package fuzzy

import (
	"testing"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg/transaction"
	fuzz "github.com/google/gofuzz"
	"github.com/shopspring/decimal"
)

// FuzzOperateBalances tests balance operations (DEBIT/CREDIT/ONHOLD/RELEASE)
// with gofuzz-generated diverse inputs plus manual edge cases.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzOperateBalances -run=^$ -fuzztime=60s
func FuzzOperateBalances(f *testing.F) {
	// Valid operations and transaction types for normalization
	validOps := []string{constant.DEBIT, constant.CREDIT, constant.ONHOLD, constant.RELEASE}
	validTxTypes := []string{constant.PENDING, constant.CANCELED, constant.APPROVED, constant.CREATED}

	// Use gofuzz to generate diverse seed values
	fuzzer := fuzz.New().NilChance(0).Funcs(
		// Custom fuzzer for int64 to generate interesting values
		func(i *int64, c fuzz.Continue) {
			choices := []int64{
				0, 1, -1, 100, -100, 1000, -1000,
				9223372036854775807,  // max int64
				-9223372036854775808, // min int64
				9007199254740992,     // 2^53 float64 precision limit
				9007199254740993,     // beyond float64 precision
			}
			if c.RandBool() {
				*i = choices[c.Intn(len(choices))]
			} else {
				*i = c.Int63()
			}
		},
	)

	// Generate 20 diverse seeds using gofuzz
	for i := 0; i < 20; i++ {
		var available, onHold, amountVal int64
		fuzzer.Fuzz(&available)
		fuzzer.Fuzz(&onHold)
		fuzzer.Fuzz(&amountVal)
		op := validOps[i%len(validOps)]
		txType := validTxTypes[i%len(validTxTypes)]
		f.Add(available, onHold, amountVal, op, txType)
	}

	// Manual edge cases: normal operations
	f.Add(int64(1000), int64(500), int64(100), "DEBIT", "CREATED")
	f.Add(int64(1000), int64(500), int64(100), "CREDIT", "CREATED")
	f.Add(int64(1000), int64(500), int64(100), "ONHOLD", "PENDING")
	f.Add(int64(1000), int64(500), int64(100), "RELEASE", "CANCELED")
	f.Add(int64(1000), int64(500), int64(100), "DEBIT", "APPROVED")

	// Manual edge cases: boundary values
	f.Add(int64(0), int64(0), int64(0), "DEBIT", "CREATED")
	f.Add(int64(9223372036854775807), int64(0), int64(1), "DEBIT", "CREATED")    // max int64
	f.Add(int64(-9223372036854775808), int64(0), int64(1), "CREDIT", "CREATED")  // min int64
	f.Add(int64(1), int64(9223372036854775807), int64(1), "RELEASE", "CANCELED") // max onHold

	// Manual edge cases: precision boundary (2^53 float64 limit)
	f.Add(int64(9007199254740992), int64(0), int64(1), "DEBIT", "CREATED")
	f.Add(int64(9007199254740993), int64(0), int64(1), "DEBIT", "CREATED")

	// Manual edge cases: negative balances (allowed for external accounts)
	f.Add(int64(-1000), int64(0), int64(500), "DEBIT", "CREATED")
	f.Add(int64(-1000), int64(0), int64(500), "CREDIT", "CREATED")

	// Manual edge cases: edge case transaction types
	f.Add(int64(1000), int64(500), int64(100), "DEBIT", "PENDING")
	f.Add(int64(1000), int64(500), int64(100), "CREDIT", "APPROVED")

	f.Fuzz(func(t *testing.T, available, onHold, amountVal int64, operation, transactionType string) {
		// Recover from panics - the function should NEVER panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("OperateBalances panicked: available=%d, onHold=%d, amount=%d, op=%s, txType=%s, panic=%v",
					available, onHold, amountVal, operation, transactionType, r)
			}
		}()

		// Normalize operation to valid values
		opValid := false
		for _, v := range validOps {
			if operation == v {
				opValid = true
				break
			}
		}
		if !opValid {
			operation = constant.DEBIT
		}

		// Normalize transaction type to valid values
		txValid := false
		for _, v := range validTxTypes {
			if transactionType == v {
				txValid = true
				break
			}
		}
		if !txValid {
			transactionType = constant.CREATED
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(available),
			OnHold:    decimal.NewFromInt(onHold),
			Version:   1,
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(amountVal),
			Operation:       operation,
			TransactionType: transactionType,
		}

		// Call the function - should not panic
		result, err := transaction.OperateBalances(amount, balance)

		// Verify result consistency
		if err == nil {
			// Version should increase for operations that change balance
			if result.Version < balance.Version {
				t.Errorf("Version decreased: before=%d, after=%d", balance.Version, result.Version)
			}
		}
	})
}

// FuzzCalculateTotal tests share and percentage calculations with gofuzz-generated
// diverse inputs plus manual edge cases.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzCalculateTotal -run=^$ -fuzztime=60s
func FuzzCalculateTotal(f *testing.F) {
	// Use gofuzz to generate diverse Share struct values
	fuzzer := fuzz.New().NilChance(0).Funcs(
		// Custom fuzzer for int64 percentage values
		func(i *int64, c fuzz.Continue) {
			// Focus on percentage-relevant values
			choices := []int64{0, 1, 25, 33, 50, 66, 75, 99, 100}
			if c.RandBool() {
				*i = choices[c.Intn(len(choices))]
			} else {
				*i = int64(c.Intn(200)) // 0-199 for edge cases
			}
		},
	)

	// Generate 20 diverse seeds using gofuzz
	for i := 0; i < 20; i++ {
		var sendValue, percentage, percentageOf int64
		fuzzer.Fuzz(&sendValue)
		fuzzer.Fuzz(&percentage)
		fuzzer.Fuzz(&percentageOf)
		// Ensure percentageOf is valid (> 0, <= 100)
		if percentageOf <= 0 {
			percentageOf = 100
		}
		if percentageOf > 100 {
			percentageOf = 100
		}
		isFrom := i%2 == 0
		f.Add(sendValue, percentage, percentageOf, isFrom)
	}

	// Manual edge cases: normal percentage calculations
	f.Add(int64(10000), int64(50), int64(100), true)   // 50% of 100%
	f.Add(int64(10000), int64(100), int64(100), false) // 100% distribution
	f.Add(int64(10000), int64(25), int64(50), true)    // 25% of 50%

	// Manual edge cases: boundary percentages
	f.Add(int64(10000), int64(0), int64(100), true)   // 0%
	f.Add(int64(10000), int64(100), int64(100), true) // 100%
	f.Add(int64(10000), int64(1), int64(100), true)   // 1%

	// Manual edge cases: large values
	f.Add(int64(9223372036854775807), int64(50), int64(100), true) // max int64 amount
	f.Add(int64(1000000000000000), int64(33), int64(100), false)   // large with odd percentage

	// Manual edge cases: precision edge cases
	f.Add(int64(100), int64(33), int64(100), true) // 33% - repeating decimal
	f.Add(int64(100), int64(66), int64(100), true) // 66% - repeating decimal
	f.Add(int64(1), int64(50), int64(100), true)   // small amount with percentage

	// Manual edge cases: zero and negative
	f.Add(int64(0), int64(50), int64(100), true)
	f.Add(int64(-1000), int64(50), int64(100), false)

	f.Fuzz(func(t *testing.T, sendValue, percentage, percentageOf int64, isFrom bool) {
		// Recover from panics
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("CalculateTotal panicked: sendValue=%d, pct=%d, pctOf=%d, isFrom=%v, panic=%v",
					sendValue, percentage, percentageOf, isFrom, r)
			}
		}()

		// Skip invalid percentages to focus on valid inputs
		if percentage < 0 || percentage > 100 || percentageOf <= 0 || percentageOf > 100 {
			return
		}

		// Build minimal transaction structure
		fromTos := []transaction.FromTo{
			{
				AccountAlias: "@test-account",
				IsFrom:       isFrom,
				Share: &transaction.Share{
					Percentage:             percentage,
					PercentageOfPercentage: percentageOf,
				},
			},
		}

		tx := transaction.Transaction{
			Send: transaction.Send{
				Asset: "USD",
				Value: decimal.NewFromInt(sendValue),
			},
		}

		// Call the function - should not panic
		total, amounts, aliases, routes := transaction.CalculateTotal(fromTos, tx, constant.CREATED)

		// Verify results are consistent
		// Note: When percentage is 0, function may return empty results which is valid
		if percentage > 0 {
			if len(amounts) != 1 {
				t.Errorf("Expected 1 amount for non-zero percentage, got %d (pct=%d)", len(amounts), percentage)
			}

			if len(aliases) != 1 {
				t.Errorf("Expected 1 alias for non-zero percentage, got %d (pct=%d)", len(aliases), percentage)
			}

			// Routes should have an entry for non-zero percentage
			if len(routes) != 1 {
				t.Errorf("Expected 1 route for non-zero percentage, got %d (pct=%d)", len(routes), percentage)
			}
		}

		// Total should not be negative for positive inputs
		if sendValue >= 0 && total.IsNegative() {
			t.Errorf("Total is negative for positive sendValue: sendValue=%d, total=%s",
				sendValue, total.String())
		}
	})
}
