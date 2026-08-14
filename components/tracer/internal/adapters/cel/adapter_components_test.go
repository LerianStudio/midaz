// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cel

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Component tests: These tests verify the interaction between internal components
// (Adapter + CEL Environment) without external dependencies.
// They run with `go test` (no build tags) as they use in-memory implementations.

// TestComponents_CompileAndEvaluate tests the full compile → evaluate flow.
func TestComponents_CompileAndEvaluate(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()
	expression := "amount > 1000"

	// Step 1: Compile
	program, err := adapter.Compile(ctx, expression)
	require.NoError(t, err, "Compile should succeed")
	assert.NotNil(t, program)

	// Step 2: Evaluate
	req := newTestRequest()
	result, err := adapter.Evaluate(ctx, program, req)
	require.NoError(t, err, "Evaluate should succeed")
	assert.True(t, result, "1500 > 1000 should be true")
}

// TestComponents_AllTransactionFields tests that all transaction fields work correctly
// in the full compile → evaluate flow. This validates that CEL environment variable names
// match the activation keys for every field.
func TestComponents_AllTransactionFields(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	// newTestRequest() returns:
	// - TransactionType: "PIX"
	// - SubType: "instant"
	// - Amount: 1500
	// - Currency: "BRL"
	// - TransactionTimestamp: time.Now()
	// - Account: {ID: testAccountID, Type: "checking", Status: "active"}
	// - Merchant: {ID: testMerchantID, Name: "Test Store", Category: "5411", Country: "BR"}
	// - Segment: {ID: ..., Name: "retail"}
	// - Portfolio: {ID: ..., Name: "premium"}
	// - Metadata: {"channel": "mobile", "risk_score": 75}

	tests := []struct {
		name        string
		expression  string
		expectTrue  bool
		description string
	}{
		// Basic transaction fields
		{
			name:        "transactionType equals PIX",
			expression:  `transactionType == "PIX"`,
			expectTrue:  true,
			description: "transactionType should match PIX",
		},
		{
			name:        "transactionType not equals CARD",
			expression:  `transactionType == "CARD"`,
			expectTrue:  false,
			description: "transactionType should not match CARD",
		},
		{
			name:        "subType equals instant",
			expression:  `subType == "instant"`,
			expectTrue:  true,
			description: "subType should match instant",
		},
		{
			name:        "amount greater than threshold",
			expression:  `amount > 1000`,
			expectTrue:  true,
			description: "amount 1500 should be > 1000",
		},
		{
			name:        "amount less than threshold",
			expression:  `amount < 1000`,
			expectTrue:  false,
			description: "amount 1500 should not be < 1000",
		},
		{
			name:        "currency equals BRL",
			expression:  `currency == "BRL"`,
			expectTrue:  true,
			description: "currency should match BRL",
		},
		{
			name:        "transactionTimestamp positive",
			expression:  `transactionTimestamp > 0`,
			expectTrue:  true,
			description: "transactionTimestamp should be positive",
		},

		// Account context fields
		{
			name:        "account type checking",
			expression:  `account["type"] == "checking"`,
			expectTrue:  true,
			description: "account.type should match checking",
		},
		{
			name:        "account status active",
			expression:  `account["status"] == "active"`,
			expectTrue:  true,
			description: "account.status should match active",
		},
		{
			name:        "account accountId is not empty",
			expression:  `account["accountId"] != ""`,
			expectTrue:  true,
			description: "account.accountId should not be empty",
		},

		// Merchant context fields
		{
			name:        "merchant name",
			expression:  `merchant["name"] == "Test Store"`,
			expectTrue:  true,
			description: "merchant.name should match Test Store",
		},
		{
			name:        "merchant category",
			expression:  `merchant["category"] == "5411"`,
			expectTrue:  true,
			description: "merchant.category should match 5411",
		},
		{
			name:        "merchant country",
			expression:  `merchant["country"] == "BR"`,
			expectTrue:  true,
			description: "merchant.country should match BR",
		},

		// Segment context fields
		{
			name:        "segment name",
			expression:  `segment["name"] == "retail"`,
			expectTrue:  true,
			description: "segment.name should match retail",
		},
		{
			name:        "segment segmentId is not empty",
			expression:  `segment["segmentId"] != ""`,
			expectTrue:  true,
			description: "segment.segmentId should not be empty",
		},

		// Portfolio context fields
		{
			name:        "portfolio name",
			expression:  `portfolio["name"] == "premium"`,
			expectTrue:  true,
			description: "portfolio.name should match premium",
		},
		{
			name:        "portfolio portfolioId is not empty",
			expression:  `portfolio["portfolioId"] != ""`,
			expectTrue:  true,
			description: "portfolio.portfolioId should not be empty",
		},

		// Metadata fields
		{
			name:        "metadata channel",
			expression:  `metadata["channel"] == "mobile"`,
			expectTrue:  true,
			description: "metadata.channel should match mobile",
		},
		{
			name:        "metadata risk_score",
			expression:  `metadata["risk_score"] == 75`,
			expectTrue:  true,
			description: "metadata.risk_score should match 75",
		},
		{
			name:        "metadata channel is not empty",
			expression:  `metadata["channel"] != ""`,
			expectTrue:  true,
			description: "metadata.channel should not be empty",
		},

		// Amount equality and fractional comparisons
		{
			name:        "amount equals integer literal",
			expression:  `amount == 1500`,
			expectTrue:  true,
			description: "amount 1500 should == 1500 (cross-type equality via DynType)",
		},
		{
			name:        "amount equals double literal",
			expression:  `amount == 1500.0`,
			expectTrue:  true,
			description: "amount 1500 should == 1500.0",
		},
		{
			name:        "amount not equals integer literal",
			expression:  `amount != 999`,
			expectTrue:  true,
			description: "amount 1500 should != 999 (cross-type inequality via DynType)",
		},
		{
			name:        "fractional amount greater than threshold",
			expression:  `amount > 12.34`,
			expectTrue:  true,
			description: "amount 1500 should be > 12.34 (cross-type numeric comparison)",
		},

		// Complex expressions combining multiple fields
		{
			name:        "complex - high value PIX",
			expression:  `transactionType == "PIX" && amount > 1000 && currency == "BRL"`,
			expectTrue:  true,
			description: "complex expression with multiple fields should match",
		},
		{
			name:        "complex - account and merchant",
			expression:  `account["status"] == "active" && merchant["country"] == "BR"`,
			expectTrue:  true,
			description: "complex expression with account and merchant should match",
		},
		{
			name:        "complex - in operator",
			expression:  `transactionType in ["PIX", "CARD", "WIRE"]`,
			expectTrue:  true,
			description: "transactionType should be in allowed list",
		},
		{
			name:        "complex - not in operator",
			expression:  `transactionType in ["CARD", "WIRE"]`,
			expectTrue:  false,
			description: "PIX should not be in CARD/WIRE list",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Compile
			program, err := adapter.Compile(ctx, tc.expression)
			require.NoError(t, err, "Compile should succeed for: %s", tc.expression)
			require.NotNil(t, program)

			// Evaluate
			req := newTestRequest()
			result, err := adapter.Evaluate(ctx, program, req)
			require.NoError(t, err, "Evaluate should succeed for: %s", tc.expression)

			if tc.expectTrue {
				assert.True(t, result, tc.description)
			} else {
				assert.False(t, result, tc.description)
			}
		})
	}
}

// TestComponents_CompileDeterministic tests that compiling the same expression
// produces programs with the same hash.
func TestComponents_CompileDeterministic(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()
	expression := `transactionType == "PIX" && amount > 1000`

	// Compile multiple times
	programs := make([]*CompiledProgram, 5)

	for i := 0; i < 5; i++ {
		program, err := adapter.Compile(ctx, expression)
		require.NoError(t, err)
		programs[i] = program
	}

	// All programs should have the same hash
	for i := 1; i < 5; i++ {
		assert.Equal(t, programs[0].ExpressionHash, programs[i].ExpressionHash,
			"All programs should have the same hash")
	}
}

// TestComponents_MultipleExpressions tests handling multiple different expressions.
func TestComponents_MultipleExpressions(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	expressions := []string{
		"amount > 1000",
		`transactionType == "PIX"`,
		`account["status"] == "active"`,
		`merchant["category"] == "5411"`,
		"currency == \"BRL\"",
	}

	programs := make([]*CompiledProgram, len(expressions))

	// Compile all expressions
	for i, expr := range expressions {
		program, err := adapter.Compile(ctx, expr)
		require.NoError(t, err, "Should compile: %s", expr)
		programs[i] = program
	}

	// Evaluate all with same request
	req := newTestRequest()

	for i, program := range programs {
		result, err := adapter.Evaluate(ctx, program, req)
		require.NoError(t, err, "Should evaluate expression %d", i)
		assert.True(t, result, "Expression %d should return true", i)
	}
}

// TestComponents_ConcurrentAccess tests thread-safe concurrent access.
func TestComponents_ConcurrentAccess(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	expressions := []string{
		"amount > 1000",
		`transactionType == "PIX"`,
		`account["status"] == "active"`,
	}

	req := newTestRequest()

	const goroutineCount = 50

	var wg sync.WaitGroup

	errChan := make(chan error, goroutineCount)

	// Run concurrent compilations and evaluations
	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			expr := expressions[idx%len(expressions)]

			// Compile
			program, err := adapter.Compile(ctx, expr)
			if err != nil {
				errChan <- err

				return
			}

			// Evaluate
			_, err = adapter.Evaluate(ctx, program, req)
			if err != nil {
				errChan <- err

				return
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Collect and report all errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	require.Empty(t, errors, "Expected no errors in concurrent operations, got: %v", errors)
}

// TestComponents_ErrorRecovery tests that errors don't corrupt state.
func TestComponents_ErrorRecovery(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	// Compile valid expression
	program, err := adapter.Compile(ctx, "amount > 1000")
	require.NoError(t, err)

	// Try to compile invalid expression
	_, err = adapter.Compile(ctx, "invalid syntax !!!")
	assert.Error(t, err, "Should fail on invalid syntax")

	// Valid program should still work
	req := newTestRequest()
	result, err := adapter.Evaluate(ctx, program, req)
	require.NoError(t, err)
	assert.True(t, result)
}

// TestComponents_EndToEndWorkflow tests a realistic workflow using all components together.
func TestComponents_EndToEndWorkflow(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx := context.Background()

	// Simulate a rule engine workflow:
	// 1. Load rules (compile expressions)
	rules := []struct {
		name       string
		expression string
	}{
		{"high_value", "amount > 1000"},
		{"fractional_threshold", "amount > 1499.99"},
		{"pix_transaction", `transactionType == "PIX"`},
		{"active_account", `account["status"] == "active"`},
		{"domestic_merchant", `merchant["country"] == "BR"`},
	}

	programs := make(map[string]*CompiledProgram)

	for _, rule := range rules {
		program, err := adapter.Compile(ctx, rule.expression)
		require.NoError(t, err, "Should compile rule: %s", rule.name)
		programs[rule.name] = program
	}

	// 2. Evaluate transaction against all rules
	req := newTestRequest()
	results := make(map[string]bool)

	for name, program := range programs {
		result, err := adapter.Evaluate(ctx, program, req)
		require.NoError(t, err, "Should evaluate rule: %s", name)
		results[name] = result
	}

	// 3. Verify results
	assert.True(t, results["high_value"], "1500 > 1000")
	assert.True(t, results["fractional_threshold"], "1500 > 1499.99")
	assert.True(t, results["pix_transaction"], "Transaction type is PIX")
	assert.True(t, results["active_account"], "Account status is active")
	assert.True(t, results["domestic_merchant"], "Merchant country is BR")
}
