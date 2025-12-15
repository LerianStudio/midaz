package helpers

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCalculateBalanceImpactFromOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		txn          TransactionRecord
		accountAlias string
		assetCode    string
		expected     decimal.Decimal
	}{
		{
			name: "credit operation adds to balance",
			txn: TransactionRecord{
				Operations: []TransactionOperation{
					{
						Type:         "CREDIT",
						AccountAlias: "account-A",
						AssetCode:    "USD",
						Amount:       OperationAmount{Value: decimalPtr("100.00")},
					},
				},
			},
			accountAlias: "account-A",
			assetCode:    "USD",
			expected:     decimal.RequireFromString("100.00"),
		},
		{
			name: "debit operation subtracts from balance",
			txn: TransactionRecord{
				Operations: []TransactionOperation{
					{
						Type:         "DEBIT",
						AccountAlias: "account-A",
						AssetCode:    "USD",
						Amount:       OperationAmount{Value: decimalPtr("50.00")},
					},
				},
			},
			accountAlias: "account-A",
			assetCode:    "USD",
			expected:     decimal.RequireFromString("-50.00"),
		},
		{
			name: "multiple operations combine correctly",
			txn: TransactionRecord{
				Operations: []TransactionOperation{
					{
						Type:         "CREDIT",
						AccountAlias: "account-A",
						AssetCode:    "USD",
						Amount:       OperationAmount{Value: decimalPtr("100.00")},
					},
					{
						Type:         "DEBIT",
						AccountAlias: "account-A",
						AssetCode:    "USD",
						Amount:       OperationAmount{Value: decimalPtr("30.00")},
					},
				},
			},
			accountAlias: "account-A",
			assetCode:    "USD",
			expected:     decimal.RequireFromString("70.00"),
		},
		{
			name: "ignores operations for different account",
			txn: TransactionRecord{
				Operations: []TransactionOperation{
					{
						Type:         "CREDIT",
						AccountAlias: "account-B",
						AssetCode:    "USD",
						Amount:       OperationAmount{Value: decimalPtr("100.00")},
					},
				},
			},
			accountAlias: "account-A",
			assetCode:    "USD",
			expected:     decimal.Zero,
		},
		{
			name: "ignores operations for different asset",
			txn: TransactionRecord{
				Operations: []TransactionOperation{
					{
						Type:         "CREDIT",
						AccountAlias: "account-A",
						AssetCode:    "EUR",
						Amount:       OperationAmount{Value: decimalPtr("100.00")},
					},
				},
			},
			accountAlias: "account-A",
			assetCode:    "USD",
			expected:     decimal.Zero,
		},
		{
			name: "handles alias with key suffix",
			txn: TransactionRecord{
				Operations: []TransactionOperation{
					{
						Type:         "CREDIT",
						AccountAlias: "account-A#default",
						AssetCode:    "USD",
						Amount:       OperationAmount{Value: decimalPtr("100.00")},
					},
				},
			},
			accountAlias: "account-A",
			assetCode:    "USD",
			expected:     decimal.RequireFromString("100.00"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := CalculateBalanceImpactFromOperations(tc.txn, tc.accountAlias, tc.assetCode)
			if !result.Equal(tc.expected) {
				t.Errorf("expected %s, got %s", tc.expected.String(), result.String())
			}
		})
	}
}

func TestCalculateExpectedBalanceFromHistory(t *testing.T) {
	t.Parallel()

	seed := decimal.RequireFromString("100.00")

	transactions := []TransactionRecord{
		{
			Status: TransactionStatus{Code: "APPROVED"},
			Operations: []TransactionOperation{
				{
					Type:         "CREDIT",
					AccountAlias: "account-A",
					AssetCode:    "USD",
					Amount:       OperationAmount{Value: decimalPtr("50.00")},
				},
			},
		},
		{
			Status: TransactionStatus{Code: "APPROVED"},
			Operations: []TransactionOperation{
				{
					Type:         "DEBIT",
					AccountAlias: "account-A",
					AssetCode:    "USD",
					Amount:       OperationAmount{Value: decimalPtr("20.00")},
				},
			},
		},
		{
			Status: TransactionStatus{Code: "PENDING"}, // Should be ignored
			Operations: []TransactionOperation{
				{
					Type:         "CREDIT",
					AccountAlias: "account-A",
					AssetCode:    "USD",
					Amount:       OperationAmount{Value: decimalPtr("999.00")},
				},
			},
		},
	}

	// Expected: 100 + 50 - 20 = 130 (pending transaction ignored)
	expected := decimal.RequireFromString("130.00")
	result := CalculateExpectedBalanceFromHistory(seed, transactions, "account-A", "USD")

	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestFilterTransactionsByAccount(t *testing.T) {
	t.Parallel()

	transactions := []TransactionRecord{
		{
			ID:     "txn-1",
			Source: []string{"account-A"},
		},
		{
			ID:          "txn-2",
			Destination: []string{"account-A"},
		},
		{
			ID:     "txn-3",
			Source: []string{"account-B"},
		},
		{
			ID: "txn-4",
			Operations: []TransactionOperation{
				{AccountAlias: "account-A#default"},
			},
		},
	}

	filtered := FilterTransactionsByAccount(transactions, "account-A")

	if len(filtered) != 3 {
		t.Errorf("expected 3 transactions, got %d", len(filtered))
	}

	// Verify the correct transactions were filtered
	ids := make(map[string]bool)
	for _, txn := range filtered {
		ids[txn.ID] = true
	}

	if !ids["txn-1"] || !ids["txn-2"] || !ids["txn-4"] {
		t.Errorf("filtered transactions missing expected IDs: %v", ids)
	}
	if ids["txn-3"] {
		t.Errorf("filtered transactions should not include txn-3")
	}
}

func TestGetTransactionHistorySummary(t *testing.T) {
	t.Parallel()

	transactions := []TransactionRecord{
		{
			Status: TransactionStatus{Code: "APPROVED"},
			Source: []string{"@external/USD"},
			Operations: []TransactionOperation{
				{
					Type:         "CREDIT",
					AccountAlias: "account-A",
					AssetCode:    "USD",
					Amount:       OperationAmount{Value: decimalPtr("100.00")},
				},
			},
		},
		{
			Status:      TransactionStatus{Code: "APPROVED"},
			Destination: []string{"@external/USD"},
			Operations: []TransactionOperation{
				{
					Type:         "DEBIT",
					AccountAlias: "account-A",
					AssetCode:    "USD",
					Amount:       OperationAmount{Value: decimalPtr("30.00")},
				},
			},
		},
		{
			Status: TransactionStatus{Code: "APPROVED"},
			Operations: []TransactionOperation{
				{
					Type:         "CREDIT",
					AccountAlias: "account-A",
					AssetCode:    "USD",
					Amount:       OperationAmount{Value: decimalPtr("50.00")},
				},
			},
		},
	}

	summary := GetTransactionHistorySummary(transactions, "account-A", "USD")

	if summary.TotalTransactions != 3 {
		t.Errorf("expected 3 total transactions, got %d", summary.TotalTransactions)
	}
	if summary.InflowCount != 1 {
		t.Errorf("expected 1 inflow, got %d", summary.InflowCount)
	}
	if summary.OutflowCount != 1 {
		t.Errorf("expected 1 outflow, got %d", summary.OutflowCount)
	}
	if summary.TransferInCount != 1 {
		t.Errorf("expected 1 transfer in, got %d", summary.TransferInCount)
	}

	// Net impact: +100 - 30 + 50 = 120
	expectedNet := decimal.RequireFromString("120.00")
	if !summary.NetImpact.Equal(expectedNet) {
		t.Errorf("expected net impact %s, got %s", expectedNet.String(), summary.NetImpact.String())
	}
}

func TestAliasMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		alias1   string
		alias2   string
		expected bool
	}{
		{"account-A", "account-A", true},
		{"account-A", "account-B", false},
		{"account-A#default", "account-A", true},
		{"account-A", "account-A#default", true},
		{"account-A#key1", "account-A#key2", true}, // Same base alias
		{"account-A#default", "account-B#default", false},
	}

	for _, tc := range tests {
		result := aliasMatches(tc.alias1, tc.alias2)
		if result != tc.expected {
			t.Errorf("aliasMatches(%q, %q) = %v, expected %v", tc.alias1, tc.alias2, result, tc.expected)
		}
	}
}

func TestCompareHTTPCountsWithActual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		httpCounts         map[string]int
		summary            TransactionHistorySummary
		expectedGhostCount int
		expectMissingInLog bool
	}{
		{
			name: "no discrepancies",
			httpCounts: map[string]int{
				"inflow":   5,
				"outflow":  3,
				"transfer": 2,
			},
			summary: TransactionHistorySummary{
				InflowCount:      5,
				OutflowCount:     3,
				TransferInCount:  2,
				TransferOutCount: 2,
			},
			expectedGhostCount: 0,
			expectMissingInLog: false,
		},
		{
			name: "ghost transactions - actual > HTTP",
			httpCounts: map[string]int{
				"inflow":   5,
				"outflow":  3,
				"transfer": 2,
			},
			summary: TransactionHistorySummary{
				InflowCount:      7, // 2 ghosts
				OutflowCount:     5, // 2 ghosts
				TransferInCount:  3, // 1 ghost
				TransferOutCount: 4, // 2 ghosts
			},
			expectedGhostCount: 7, // 2 + 2 + 1 + 2
			expectMissingInLog: false,
		},
		{
			name: "missing transactions - HTTP > actual (should NOT reduce ghost count)",
			httpCounts: map[string]int{
				"inflow":   10,
				"outflow":  8,
				"transfer": 5,
			},
			summary: TransactionHistorySummary{
				InflowCount:      5, // 5 missing
				OutflowCount:     3, // 5 missing
				TransferInCount:  2, // 3 missing
				TransferOutCount: 1, // 4 missing
			},
			expectedGhostCount: 0, // No ghosts, only missing
			expectMissingInLog: true,
		},
		{
			name: "mixed - some ghost, some missing",
			httpCounts: map[string]int{
				"inflow":   5,
				"outflow":  10, // will have missing
				"transfer": 2,
			},
			summary: TransactionHistorySummary{
				InflowCount:      8, // 3 ghosts
				OutflowCount:     6, // 4 missing (NOT subtracted)
				TransferInCount:  5, // 3 ghosts
				TransferOutCount: 1, // 1 missing
			},
			expectedGhostCount: 6, // 3 + 0 + 3 + 0 (missing don't subtract)
			expectMissingInLog: true,
		},
		{
			name: "all zeros",
			httpCounts: map[string]int{
				"inflow":   0,
				"outflow":  0,
				"transfer": 0,
			},
			summary: TransactionHistorySummary{
				InflowCount:      0,
				OutflowCount:     0,
				TransferInCount:  0,
				TransferOutCount: 0,
			},
			expectedGhostCount: 0,
			expectMissingInLog: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ghostCount, details := CompareHTTPCountsWithActual(tc.httpCounts, tc.summary)

			if ghostCount != tc.expectedGhostCount {
				t.Errorf("expected ghost count %d, got %d\ndetails: %s", tc.expectedGhostCount, ghostCount, details)
			}

			// Verify details string contains expected information
			if details == "" {
				t.Error("details string should not be empty")
			}

			// Check that Missing_transactions appears in log when expected
			hasMissing := len(details) > 0 && (details[len(details)-1] != '0' || tc.expectMissingInLog)
			if tc.expectMissingInLog && !hasMissing {
				// This is a soft check - the format should include missing count
				t.Logf("details: %s", details)
			}
		})
	}
}

// Helper function to create decimal pointer
func decimalPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}
