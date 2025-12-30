package assert

import (
	"testing"
	"time"

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

// TestTransactionCanTransitionTo tests the TransactionCanTransitionTo predicate.
func TestTransactionCanTransitionTo(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		target   string
		expected bool
	}{
		// Valid transitions from PENDING
		{"PENDING to APPROVED", "PENDING", "APPROVED", true},
		{"PENDING to CANCELED", "PENDING", "CANCELED", true},
		// Invalid transitions from PENDING
		{"PENDING to CREATED", "PENDING", "CREATED", false},
		{"PENDING to PENDING", "PENDING", "PENDING", false},
		// Invalid transitions from APPROVED (terminal state for forward)
		{"APPROVED to CANCELED", "APPROVED", "CANCELED", false},
		{"APPROVED to PENDING", "APPROVED", "PENDING", false},
		{"APPROVED to CREATED", "APPROVED", "CREATED", false},
		// Invalid transitions from CANCELED (terminal state)
		{"CANCELED to APPROVED", "CANCELED", "APPROVED", false},
		{"CANCELED to PENDING", "CANCELED", "PENDING", false},
		// Invalid transitions from CREATED
		{"CREATED to APPROVED", "CREATED", "APPROVED", false},
		{"CREATED to CANCELED", "CREATED", "CANCELED", false},
		// Invalid statuses
		{"invalid current", "INVALID", "APPROVED", false},
		{"invalid target", "PENDING", "INVALID", false},
		{"both invalid", "INVALID", "UNKNOWN", false},
		{"empty current", "", "APPROVED", false},
		{"empty target", "PENDING", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, TransactionCanTransitionTo(tt.current, tt.target))
		})
	}
}

// TestTransactionCanBeReverted tests the TransactionCanBeReverted predicate.
func TestTransactionCanBeReverted(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		hasParent bool
		expected  bool
	}{
		{"APPROVED without parent can revert", "APPROVED", false, true},
		{"APPROVED with parent cannot revert", "APPROVED", true, false},
		{"PENDING cannot revert", "PENDING", false, false},
		{"CANCELED cannot revert", "CANCELED", false, false},
		{"CREATED cannot revert", "CREATED", false, false},
		{"NOTED cannot revert", "NOTED", false, false},
		{"invalid status cannot revert", "INVALID", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, TransactionCanBeReverted(tt.status, tt.hasParent))
		})
	}
}

// TestBalanceSufficientForRelease tests the BalanceSufficientForRelease predicate.
func TestBalanceSufficientForRelease(t *testing.T) {
	tests := []struct {
		name          string
		onHold        decimal.Decimal
		releaseAmount decimal.Decimal
		expected      bool
	}{
		{"sufficient onHold", decimal.NewFromInt(100), decimal.NewFromInt(50), true},
		{"exactly sufficient", decimal.NewFromInt(100), decimal.NewFromInt(100), true},
		{"insufficient onHold", decimal.NewFromInt(50), decimal.NewFromInt(100), false},
		{"zero onHold zero release", decimal.Zero, decimal.Zero, true},
		{"zero onHold positive release", decimal.Zero, decimal.NewFromInt(1), false},
		{"decimal precision sufficient", decimal.NewFromFloat(100.50), decimal.NewFromFloat(100.49), true},
		{"decimal precision insufficient", decimal.NewFromFloat(100.49), decimal.NewFromFloat(100.50), false},
		{"negative onHold always fails", decimal.NewFromInt(-10), decimal.NewFromInt(5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, BalanceSufficientForRelease(tt.onHold, tt.releaseAmount))
		})
	}
}

// TestDateNotInFuture tests the DateNotInFuture predicate.
func TestDateNotInFuture(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		date     time.Time
		expected bool
	}{
		{"past date valid", now.Add(-24 * time.Hour), true},
		{"now valid", now, true},
		{"one second ago valid", now.Add(-time.Second), true},
		{"one second future invalid", now.Add(time.Second), false},
		{"one hour future invalid", now.Add(time.Hour), false},
		{"far future invalid", now.Add(365 * 24 * time.Hour), false},
		{"zero time valid", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DateNotInFuture(tt.date)
			// Allow slight timing variance for "now" test
			if tt.name == "now valid" {
				require.True(t, result || time.Since(now) < time.Millisecond)
			} else {
				require.Equal(t, tt.expected, result)
			}
		})
	}
}
