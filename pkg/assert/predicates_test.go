package assert

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// TestDebitsEqualCredits tests the DebitsEqualCredits predicate for double-entry accounting.
func TestDebitsEqualCredits(t *testing.T) {
	tests := []struct {
		name     string
		debits   decimal.Decimal
		credits  decimal.Decimal
		expected bool
	}{
		{"equal positive amounts", decimal.NewFromInt(100), decimal.NewFromInt(100), true},
		{"equal with decimals", decimal.NewFromFloat(123.45), decimal.NewFromFloat(123.45), true},
		{"equal zero", decimal.Zero, decimal.Zero, true},
		{"debits greater", decimal.NewFromInt(100), decimal.NewFromInt(99), false},
		{"credits greater", decimal.NewFromInt(99), decimal.NewFromInt(100), false},
		{"tiny difference", decimal.NewFromFloat(100.001), decimal.NewFromFloat(100.002), false},
		{"large equal", decimal.NewFromInt(1000000000), decimal.NewFromInt(1000000000), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, DebitsEqualCredits(tt.debits, tt.credits))
		})
	}
}

// TestNonZeroTotals tests the NonZeroTotals predicate for transaction validation.
func TestNonZeroTotals(t *testing.T) {
	tests := []struct {
		name     string
		debits   decimal.Decimal
		credits  decimal.Decimal
		expected bool
	}{
		{"both positive", decimal.NewFromInt(100), decimal.NewFromInt(100), true},
		{"both zero", decimal.Zero, decimal.Zero, false},
		{"debits zero", decimal.Zero, decimal.NewFromInt(100), false},
		{"credits zero", decimal.NewFromInt(100), decimal.Zero, false},
		{"small positive", decimal.NewFromFloat(0.01), decimal.NewFromFloat(0.01), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, NonZeroTotals(tt.debits, tt.credits))
		})
	}
}

// TestValidTransactionStatus tests the ValidTransactionStatus predicate.
func TestValidTransactionStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"CREATED valid", "CREATED", true},
		{"APPROVED valid", "APPROVED", true},
		{"PENDING valid", "PENDING", true},
		{"CANCELED valid", "CANCELED", true},
		{"NOTED valid", "NOTED", true},
		{"empty invalid", "", false},
		{"lowercase invalid", "pending", false},
		{"unknown invalid", "UNKNOWN", false},
		{"partial invalid", "APPROV", false},
		{"with spaces invalid", " PENDING ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, ValidTransactionStatus(tt.status))
		})
	}
}
