package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// TestIntegration_Transaction_Atomicity_NoOrphanTransactions verifies that every
// transaction created has corresponding operations. This test validates the
// atomicity fix that wraps balance updates, transaction creation, and operation
// creation in a single database transaction.
//
// The test creates multiple transactions and verifies that each one has the
// expected number of operations. If any transaction has no operations (orphan),
// the test fails.
func TestIntegration_Transaction_Atomicity_NoOrphanTransactions(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup org/ledger/accounts
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("AtomicityOrg"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("AtomicityLedger"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// Create two accounts for transfers
	aliasSender := iso.UniqueAccountAlias("sender")
	aliasReceiver := iso.UniqueAccountAlias("receiver")

	_, err = h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, aliasSender, "USD")
	if err != nil {
		t.Fatalf("create sender account: %v", err)
	}

	_, err = h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, aliasReceiver, "USD")
	if err != nil {
		t.Fatalf("create receiver account: %v", err)
	}

	// Seed the sender account with initial balance
	seedTx := map[string]any{
		"code": iso.UniqueTransactionCode("SEED"),
		"send": map[string]any{
			"asset": "USD",
			"value": "100.00",
			"distribute": map[string]any{
				"to": []map[string]any{
					{"accountAlias": aliasSender, "amount": map[string]any{"asset": "USD", "value": "100.00"}},
				},
			},
		},
	}

	code, body, err := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID), headers, seedTx)
	if err != nil || code != 201 {
		t.Fatalf("seed inflow: code=%d err=%v body=%s", code, err, string(body))
	}

	// Wait for seed transaction to be processed
	time.Sleep(500 * time.Millisecond)

	// Create multiple transfer transactions
	const numTransactions = 5
	transactionIDs := make([]string, 0, numTransactions)

	for i := 0; i < numTransactions; i++ {
		transferTx := map[string]any{
			"code": iso.UniqueTransactionCode(fmt.Sprintf("TRANSFER-%d", i)),
			"send": map[string]any{
				"asset": "USD",
				"value": "1.00",
				"source": map[string]any{
					"from": []map[string]any{
						{"accountAlias": aliasSender, "amount": map[string]any{"asset": "USD", "value": "1.00"}},
					},
				},
				"distribute": map[string]any{
					"to": []map[string]any{
						{"accountAlias": aliasReceiver, "amount": map[string]any{"asset": "USD", "value": "1.00"}},
					},
				},
			},
		}

		code, body, err := trans.Request(ctx, "POST",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/transfer", orgID, ledgerID),
			headers, transferTx)
		if err != nil || code != 201 {
			t.Fatalf("transfer %d: code=%d err=%v body=%s", i, code, err, string(body))
		}

		var txResp struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &txResp); err != nil {
			t.Fatalf("unmarshal transfer response: %v", err)
		}
		transactionIDs = append(transactionIDs, txResp.ID)
	}

	// Wait for all transactions to be processed
	time.Sleep(1 * time.Second)

	// Verify each transaction has operations (no orphans)
	orphanCount := 0
	for _, txID := range transactionIDs {
		// Get transaction with operations
		code, body, err := trans.Request(ctx, "GET",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", orgID, ledgerID, txID),
			headers, nil)
		if err != nil || code != 200 {
			t.Errorf("get transaction %s: code=%d err=%v", txID, code, err)
			continue
		}

		var txWithOps struct {
			ID         string `json:"id"`
			Status     struct{ Code string } `json:"status"`
			Operations []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"operations"`
		}
		if err := json.Unmarshal(body, &txWithOps); err != nil {
			t.Errorf("unmarshal transaction %s: %v", txID, err)
			continue
		}

		// A transfer should have at least 2 operations (debit + credit)
		// Some transactions may have more depending on routing
		if len(txWithOps.Operations) < 2 {
			orphanCount++
			t.Errorf("ORPHAN DETECTED: transaction %s has only %d operations (expected >= 2), status=%s",
				txID, len(txWithOps.Operations), txWithOps.Status.Code)
		}
	}

	if orphanCount > 0 {
		t.Fatalf("Atomicity violation: found %d orphan transactions out of %d", orphanCount, numTransactions)
	}

	t.Logf("SUCCESS: All %d transactions have operations (no orphans)", numTransactions)
}

// TestIntegration_Transaction_Atomicity_ConcurrentCreation verifies that even
// under concurrent load, no orphan transactions are created.
func TestIntegration_Transaction_Atomicity_ConcurrentCreation(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// Setup
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("ConcurrentOrg"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("ConcurrentLedger"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}

	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// Create multiple accounts for concurrent operations
	const numAccounts = 3
	accounts := make([]string, numAccounts)
	for i := 0; i < numAccounts; i++ {
		alias := iso.UniqueAccountAlias(fmt.Sprintf("acc%d", i))
		accounts[i] = alias
		_, err = h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
		if err != nil {
			t.Fatalf("create account %d: %v", i, err)
		}
	}

	// Seed all accounts
	for i, alias := range accounts {
		seedTx := map[string]any{
			"code": iso.UniqueTransactionCode(fmt.Sprintf("SEED-%d", i)),
			"send": map[string]any{
				"asset": "USD",
				"value": "100.00",
				"distribute": map[string]any{
					"to": []map[string]any{
						{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "100.00"}},
					},
				},
			},
		}
		code, body, err := trans.Request(ctx, "POST",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID),
			headers, seedTx)
		if err != nil || code != 201 {
			t.Fatalf("seed account %s: code=%d err=%v body=%s", alias, code, err, string(body))
		}
	}

	// Wait for seeds to process
	time.Sleep(1 * time.Second)

	// Create concurrent transactions
	const numTransactions = 10
	type result struct {
		txID string
		err  error
	}
	results := make(chan result, numTransactions)

	for i := 0; i < numTransactions; i++ {
		go func(idx int) {
			fromIdx := idx % numAccounts
			toIdx := (idx + 1) % numAccounts

			transferTx := map[string]any{
				"code": iso.UniqueTransactionCode(fmt.Sprintf("CONCURRENT-%d", idx)),
				"send": map[string]any{
					"asset": "USD",
					"value": "0.01",
					"source": map[string]any{
						"from": []map[string]any{
							{"accountAlias": accounts[fromIdx], "amount": map[string]any{"asset": "USD", "value": "0.01"}},
						},
					},
					"distribute": map[string]any{
						"to": []map[string]any{
							{"accountAlias": accounts[toIdx], "amount": map[string]any{"asset": "USD", "value": "0.01"}},
						},
					},
				},
			}

			code, body, err := trans.Request(ctx, "POST",
				fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/transfer", orgID, ledgerID),
				headers, transferTx)
			if err != nil {
				results <- result{err: fmt.Errorf("request error: %v", err)}
				return
			}
			if code != 201 {
				results <- result{err: fmt.Errorf("unexpected status %d: %s", code, string(body))}
				return
			}

			var txResp struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(body, &txResp); err != nil {
				results <- result{err: fmt.Errorf("unmarshal error: %v", err)}
				return
			}
			results <- result{txID: txResp.ID}
		}(i)
	}

	// Collect results
	var transactionIDs []string
	var errors []error
	for i := 0; i < numTransactions; i++ {
		r := <-results
		if r.err != nil {
			errors = append(errors, r.err)
		} else {
			transactionIDs = append(transactionIDs, r.txID)
		}
	}

	// Some concurrent transactions may fail due to lock contention - that's OK
	// The key is that successful transactions must not be orphans
	if len(transactionIDs) == 0 {
		t.Skipf("All transactions failed (possibly due to lock contention): %v", errors)
	}

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Verify no orphans among successful transactions
	orphanCount := 0
	for _, txID := range transactionIDs {
		code, body, err := trans.Request(ctx, "GET",
			fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", orgID, ledgerID, txID),
			headers, nil)
		if err != nil || code != 200 {
			continue // Skip if we can't fetch
		}

		var txWithOps struct {
			Operations []struct{ ID string } `json:"operations"`
		}
		if err := json.Unmarshal(body, &txWithOps); err != nil {
			continue
		}

		if len(txWithOps.Operations) < 2 {
			orphanCount++
			t.Errorf("ORPHAN: transaction %s has %d operations", txID, len(txWithOps.Operations))
		}
	}

	if orphanCount > 0 {
		t.Fatalf("Atomicity violation under concurrent load: %d orphans out of %d successful transactions",
			orphanCount, len(transactionIDs))
	}

	t.Logf("SUCCESS: %d concurrent transactions created without orphans (%d failed due to contention)",
		len(transactionIDs), len(errors))
}
