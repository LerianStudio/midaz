package transaction

import (
	"fmt"
	"strings"
	"testing"

	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		dsl         string
		expected    any
		expectError bool
	}{
		{
			name:        "Empty DSL input",
			dsl:         "",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Invalid DSL input",
			dsl:         "INVALID SYNTAX",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Variable in send amount numeric position should fail",
			dsl:         `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD $amount|0 (source (from @A :amount USD 100|0)) (distribute (to @B :amount USD 100|0))))`,
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Variable in from amount numeric position should fail",
			dsl:         `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 100|0 (source (from @A :amount USD $value|0)) (distribute (to @B :amount USD 100|0))))`,
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Variable in scale numeric position should fail",
			dsl:         `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 100|$scale (source (from @A :amount USD 100|0)) (distribute (to @B :amount USD 100|0))))`,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tt.expectError {
						t.Fatalf("Unexpected panic: %v", r)
					}
				}
			}()

			transaction := Parse(tt.dsl)

			if tt.expectError {
				if transaction != nil {
					t.Errorf("Expected error, got %v", transaction)
				}
			} else {
				if transaction != tt.expected {
					t.Errorf("Expected %v, got %v", tt.expected, transaction)
				}
			}
		})
	}
}

func TestParse_SharePercentageOver100_Panics(t *testing.T) {
	// Arrange: DSL with share percentage > 100
	dsl := `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 1000|2 (source (from @A :share 150)) (distribute (to @B :remaining))))`

	// Act & Assert: Should panic for invalid percentage range
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for share percentage > 100, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "share percentage must be between 0 and 100") {
			t.Errorf("Expected panic about share percentage range, got: %v", r)
		}
	}()

	Parse(dsl)
}

func TestParse_SharePercentageNegative_ReturnsNil(t *testing.T) {
	// Note: The lexer rejects negative numbers, so Parse should return nil without panicking.
	dsl := `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 1000|2 (source (from @A :share -5)) (distribute (to @B :remaining))))`

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Unexpected panic for negative share percentage: %v", r)
		}
	}()

	result := Parse(dsl)
	if result != nil {
		t.Errorf("Expected nil transaction for negative share percentage, got %v", result)
	}
}

func TestParse_ValidDSL_ReturnsTransaction(t *testing.T) {
	// Arrange: Valid DSL
	dsl := `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 1000|2 (source (from @A :amount USD 1000|2)) (distribute (to @B :amount USD 1000|2))))`

	// Act
	result := Parse(dsl)

	// Assert: Should return valid transaction
	if result == nil {
		t.Errorf("Expected valid transaction, got nil")
		return
	}

	tx, ok := result.(pkgTransaction.Transaction)
	if !ok {
		t.Errorf("Expected pkgTransaction.Transaction, got %T", result)
		return
	}

	if len(tx.Send.Source.From) == 0 {
		t.Errorf("Expected at least one source, got none")
	}
	if len(tx.Send.Distribute.To) == 0 {
		t.Errorf("Expected at least one destination, got none")
	}
}

func TestParse_ZeroSendValue_Panics(t *testing.T) {
	// Arrange: DSL with zero send value
	dsl := `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 0|2 (source (from @A :amount USD 0|2)) (distribute (to @B :amount USD 0|2))))`

	// Act & Assert: Should panic for zero send value
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for zero send value, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "send value must be positive") {
			t.Errorf("Expected panic about positive send value, got: %v", r)
		}
	}()

	Parse(dsl)
}
