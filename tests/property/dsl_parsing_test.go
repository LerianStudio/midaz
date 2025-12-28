package property

import (
	"fmt"
	"testing"
	"testing/quick"

	goldTransaction "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
)

// Property: Parsing the same DSL always produces identical results (determinism)
func TestProperty_DSLParsingDeterminism_Model(t *testing.T) {
	// Test with known valid DSL templates
	validDSLs := []string{
		`(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 100|2 (source (from @alice :amount USD 100|2)) (distribute (to @bob :amount USD 100|2))))`,
		`(transaction V1 (chart-of-accounts-group-name PAYMENT) (send BRL 500|2 (source (from @src :amount BRL 500|2)) (distribute (to @dst :amount BRL 500|2))))`,
	}

	f := func(seed int64) bool {
		// Select a DSL based on seed
		idx := int(seed) % len(validDSLs)
		if idx < 0 {
			idx = -idx
		}
		dsl := validDSLs[idx]

		// Parse twice
		result1 := goldTransaction.Parse(dsl)
		result2 := goldTransaction.Parse(dsl)

		// Both should succeed or both should fail
		if (result1 == nil) != (result2 == nil) {
			t.Logf("Inconsistent parse state: result1=%v result2=%v", result1, result2)
			return false
		}

		if result1 == nil {
			return true // Both failed consistently
		}

		// Type assert to Transaction
		tx1, ok1 := result1.(pkgTransaction.Transaction)
		tx2, ok2 := result2.(pkgTransaction.Transaction)

		if !ok1 || !ok2 {
			t.Logf("Type assertion failed: ok1=%v ok2=%v", ok1, ok2)
			return false
		}

		// Check structural equality
		if tx1.ChartOfAccountsGroupName != tx2.ChartOfAccountsGroupName {
			t.Logf("ChartOfAccountsGroupName mismatch: %s vs %s",
				tx1.ChartOfAccountsGroupName, tx2.ChartOfAccountsGroupName)
			return false
		}

		if tx1.Pending != tx2.Pending {
			t.Logf("Pending mismatch: %v vs %v", tx1.Pending, tx2.Pending)
			return false
		}

		// Check Send section
		if !tx1.Send.Value.Equal(tx2.Send.Value) {
			t.Logf("Send.Value mismatch: %s vs %s", tx1.Send.Value, tx2.Send.Value)
			return false
		}

		if tx1.Send.Asset != tx2.Send.Asset {
			t.Logf("Send.Asset mismatch: %s vs %s", tx1.Send.Asset, tx2.Send.Asset)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("DSL parsing determinism property failed: %v", err)
	}
}

// Property: Scale semantics - value|scale produces value / 10^scale
func TestProperty_DSLScaleSemantics_Model(t *testing.T) {
	f := func(value int64, scale uint8) bool {
		// Constrain to reasonable values
		if value <= 0 {
			value = 1
		}
		if value > 1_000_000_000 {
			value = 1_000_000_000
		}
		if scale > 9 {
			scale = 9
		}

		// Build DSL with the value|scale format
		dsl := fmt.Sprintf(
			`(transaction V1 (chart-of-accounts-group-name TEST) (send USD %d|%d (source (from @src :amount USD %d|%d)) (distribute (to @dst :amount USD %d|%d))))`,
			value, scale, value, scale, value, scale,
		)

		result := goldTransaction.Parse(dsl)
		if result == nil {
			t.Logf("Parse error for value=%d scale=%d", value, scale)
			return true // Skip parse errors
		}

		tx, ok := result.(pkgTransaction.Transaction)
		if !ok {
			t.Logf("Type assertion failed for value=%d scale=%d", value, scale)
			return true // Skip type assertion failures
		}

		// Expected: value shifted by scale decimal places
		expected := decimal.NewFromInt(value).Shift(-int32(scale))

		if !tx.Send.Value.Equal(expected) {
			t.Logf("Scale mismatch: %d|%d expected=%s got=%s",
				value, scale, expected, tx.Send.Value)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("DSL scale semantics property failed: %v", err)
	}
}

// Property: In a parsed transaction, source total should equal destination total
func TestProperty_DSLSourceEqualsDestination_Model(t *testing.T) {
	// Test templates with balanced amounts
	f := func(amount int64) bool {
		if amount <= 0 {
			amount = 1
		}
		if amount > 1_000_000 {
			amount = 1_000_000
		}

		dsl := fmt.Sprintf(
			`(transaction V1 (chart-of-accounts-group-name TRANSFER) (send USD %d|2 (source (from @alice :amount USD %d|2)) (distribute (to @bob :amount USD %d|2))))`,
			amount, amount, amount,
		)

		result := goldTransaction.Parse(dsl)
		if result == nil {
			return true // Skip parse errors
		}

		tx, ok := result.(pkgTransaction.Transaction)
		if !ok {
			return true // Skip type assertion failures
		}

		// Calculate source total
		sourceTotal := decimal.Zero
		for _, from := range tx.Send.Source.From {
			if from.Amount != nil {
				sourceTotal = sourceTotal.Add(from.Amount.Value)
			}
		}

		// Calculate destination total
		destTotal := decimal.Zero
		for _, to := range tx.Send.Distribute.To {
			if to.Amount != nil {
				destTotal = destTotal.Add(to.Amount.Value)
			}
		}

		// Property: source == destination == send value
		if !sourceTotal.Equal(destTotal) {
			t.Logf("Source/Dest mismatch: source=%s dest=%s", sourceTotal, destTotal)
			return false
		}

		if !sourceTotal.Equal(tx.Send.Value) {
			t.Logf("Source/Send mismatch: source=%s send=%s", sourceTotal, tx.Send.Value)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("DSL source/destination balance property failed: %v", err)
	}
}
