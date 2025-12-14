package transaction

import (
	"testing"
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
