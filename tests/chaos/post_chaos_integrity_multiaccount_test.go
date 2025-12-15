package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Multi-account integrity across chaos: batch inflows/outflows/transfers on A and B, inject restarts/pause, then reconcile balances.
func TestChaos_PostChaosIntegrity_MultiAccount(t *testing.T) {
	shouldRunChaos(t)
	// auto log capture for correlation
	defer h.StartLogCapture([]string{"midaz-transaction", "midaz-onboarding", "midaz-postgres-primary"}, "PostChaosIntegrity_MultiAccount")()

	env := h.LoadEnvironment()
	_ = h.WaitForHTTP200(env.OnboardingURL+"/health", 60*time.Second)
	_ = h.WaitForHTTP200(env.TransactionURL+"/health", 60*time.Second)
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/accounts A and B
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Chaos Org "+h.RandString(6), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-int"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: %d %s", code, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("asset: %v", err)
	}

	aliasA := "intA-" + h.RandString(4)
	aliasB := "intB-" + h.RandString(4)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": aliasA})
	if err != nil || code != 201 {
		t.Fatalf("create A: %d %s", code, string(body))
	}
	var accA struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &accA)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "B", "assetCode": "USD", "type": "deposit", "alias": aliasB})
	if err != nil || code != 201 {
		t.Fatalf("create B: %d %s", code, string(body))
	}
	var accB struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &accB)
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, accA.ID, headers); err != nil {
		t.Fatalf("ensure default A: %v", err)
	}
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, accB.ID, headers); err != nil {
		t.Fatalf("ensure default B: %v", err)
	}
	// Enable default balances for both accounts (by alias)
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasA, headers); err != nil {
		t.Fatalf("enable default A: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasB, headers); err != nil {
		t.Fatalf("enable default B: %v", err)
	}

	// Seed A: 100
	seedHeaders := make(map[string]string)
	for k, v := range headers {
		seedHeaders[k] = v
	}
	seedHeaders["X-Idempotency"] = fmt.Sprintf("seed-A-%s", h.RandHex(8))
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), seedHeaders, map[string]any{"send": map[string]any{"asset": "USD", "value": "100.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "100.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, aliasA, "USD", headers, decimal.RequireFromString("100.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Batch operations with resiliency: use RequestFullWithRetry to tolerate 429/502/503/504
	inA, outA, trAB, outB := 0, 0, 0, 0
	type acc struct{ Kind, ID string }
	accepted := make([]acc, 0, 64)

	// 6 inflows to A (2 each)
	for i := 0; i < 6; i++ {
		// Generate unique idempotency key to avoid collisions during retries
		reqHeaders := make(map[string]string)
		for k, v := range headers {
			reqHeaders[k] = v
		}
		reqHeaders["X-Idempotency"] = fmt.Sprintf("inflow-A-%d-%s", i, h.RandHex(8))

		p := map[string]any{"send": map[string]any{"asset": "USD", "value": "2.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "2.00"}}}}}}
		c, b, _, _ := trans.RequestFullWithRetry(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), reqHeaders, p, 4, 200*time.Millisecond)
		if c == 201 {
			inA++
			var m struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(b, &m)
			if m.ID != "" {
				accepted = append(accepted, acc{Kind: "inflowA", ID: m.ID})
			}
		}
		if i == 2 { // inject DB pause mid-batch
			_ = h.DockerAction("pause", "midaz-postgres-primary")
			time.Sleep(1000 * time.Millisecond)
			_ = h.DockerAction("unpause", "midaz-postgres-primary")
		}
	}

	// 5 transfers A->B (1 each)
	for i := 0; i < 5; i++ {
		// Generate unique idempotency key to avoid collisions during retries
		reqHeaders := make(map[string]string)
		for k, v := range headers {
			reqHeaders[k] = v
		}
		reqHeaders["X-Idempotency"] = fmt.Sprintf("transfer-AB-%d-%s", i, h.RandHex(8))

		p := map[string]any{"send": map[string]any{
			"asset": "USD", "value": "1.00",
			"source":     map[string]any{"from": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}},
			"distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasB, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}},
		}}
		c, b, _, _ := trans.RequestFullWithRetry(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", org.ID, ledger.ID), reqHeaders, p, 4, 200*time.Millisecond)
		if c == 201 {
			trAB++
			var m struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(b, &m)
			if m.ID != "" {
				accepted = append(accepted, acc{Kind: "transferAB", ID: m.ID})
			}
		}
		if i == 1 { // inject service restart during transfers
			_ = h.RestartWithWait("midaz-transaction", 4*time.Second)
			// Additional stabilization - poll health then wait for connection pools to warm up
			_ = h.WaitForHTTP200(env.TransactionURL+"/health", 10*time.Second)
			time.Sleep(2 * time.Second) // Extra buffer for PostgreSQL/Redis pool initialization
		}
	}

	// 3 outflows from A (1 each)
	for i := 0; i < 3; i++ {
		// Generate unique idempotency key to avoid collisions during retries
		reqHeaders := make(map[string]string)
		for k, v := range headers {
			reqHeaders[k] = v
		}
		reqHeaders["X-Idempotency"] = fmt.Sprintf("outflow-A-%d-%s", i, h.RandHex(8))

		p := map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "source": map[string]any{"from": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}}
		c, b, _, _ := trans.RequestFullWithRetry(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), reqHeaders, p, 4, 200*time.Millisecond)
		if c == 201 {
			outA++
			var m struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(b, &m)
			if m.ID != "" {
				accepted = append(accepted, acc{Kind: "outflowA", ID: m.ID})
			}
		}
	}

	// 2 outflows from B (1 each)
	for i := 0; i < 2; i++ {
		// Generate unique idempotency key to avoid collisions during retries
		reqHeaders := make(map[string]string)
		for k, v := range headers {
			reqHeaders[k] = v
		}
		reqHeaders["X-Idempotency"] = fmt.Sprintf("outflow-B-%d-%s", i, h.RandHex(8))

		p := map[string]any{"send": map[string]any{"asset": "USD", "value": "1.00", "source": map[string]any{"from": []map[string]any{{"accountAlias": aliasB, "amount": map[string]any{"asset": "USD", "value": "1.00"}}}}}}
		c, b, _, _ := trans.RequestFullWithRetry(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID), reqHeaders, p, 4, 200*time.Millisecond)
		if c == 201 {
			outB++
			var m struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(b, &m)
			if m.ID != "" {
				accepted = append(accepted, acc{Kind: "outflowB", ID: m.ID})
			}
		}
	}

	// Wait for DLQ consumer to replay any messages that went to DLQ during chaos
	dlqMgmtURL := "http://localhost:3004"
	queueNames := []string{
		os.Getenv("RABBITMQ_BALANCE_CREATE_QUEUE"),
		os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"),
	}
	if queueNames[0] == "" {
		queueNames[0] = "transaction.balance_create.queue"
	}
	if queueNames[1] == "" {
		queueNames[1] = "transaction.transaction_balance_operation.queue"
	}

	// Log DLQ counts
	// TODO(review): Consider using environment variables for RabbitMQ credentials instead of hardcoded values - code-reviewer on 2025-12-14
	dlqCounts, err := h.GetAllDLQCounts(ctx, dlqMgmtURL, "midaz", "lerian", queueNames)
	if err != nil {
		t.Logf("Warning: could not get DLQ counts: %v", err)
	} else {
		t.Logf("CHAOS_TEST_DLQ_CHECK: balance_create_dlq=%d, transaction_ops_dlq=%d, total=%d",
			dlqCounts.BalanceCreateDLQ, dlqCounts.TransactionOpsDLQ, dlqCounts.TotalDLQMessages)
	}

	// Wait for all DLQs to empty (indicating replay completion)
	// Timeout: 5 minutes to account for exponential backoff (max delay 30s + processing time)
	for _, queueName := range queueNames {
		if err := h.WaitForDLQEmpty(ctx, dlqMgmtURL, queueName, "midaz", "lerian", 5*time.Minute); err != nil {
			t.Logf("Warning: DLQ wait timed out for %s: %v", queueName, err)
		}
	}

	// Log HTTP 201 counts for informational purposes (these may differ from actual committed due to ghost transactions)
	t.Logf("CHAOS_TEST_HTTP_201_COUNTS: inA=%d, outA=%d, trAB=%d, outB=%d (totalAccepted=%d)",
		inA, outA, trAB, outB, len(accepted))

	// Calculate what we THOUGHT the balance should be based on HTTP 201 responses
	httpExpA := decimal.RequireFromString("100").Add(decimal.NewFromInt(int64(inA * 2))).Sub(decimal.NewFromInt(int64(trAB))).Sub(decimal.NewFromInt(int64(outA)))
	httpExpB := decimal.NewFromInt(int64(trAB)).Sub(decimal.NewFromInt(int64(outB)))
	t.Logf("CHAOS_TEST_HTTP_EXPECTED: A=%s (100 + %d*2 - %d - %d), B=%s (%d - %d)",
		httpExpA.String(), inA, trAB, outA, httpExpB.String(), trAB, outB)

	// Query-based verification: Calculate expected balance from actual transaction history
	// This accounts for "ghost transactions" that committed but didn't return HTTP 201
	seedA := decimal.RequireFromString("100")
	seedB := decimal.Zero

	// Verify Account A using transaction history
	actualA, expectedA, summaryA, errA := h.VerifyBalanceConsistencyWithInfo(ctx, trans, org.ID, ledger.ID, aliasA, "USD", seedA, headers)
	if errA != nil {
		// Dump accepted sample for debugging
		lines := []string{}
		max := 30
		for i, a := range accepted {
			if i >= max {
				break
			}
			c, b, _ := trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", org.ID, ledger.ID, a.ID), headers, nil)
			lines = append(lines, fmt.Sprintf("%d %s %s %s", c, a.Kind, a.ID, string(b)))
		}
		logPath := fmt.Sprintf("reports/logs/post_chaos_multiaccount_accepted_%d.log", time.Now().Unix())
		_ = h.WriteTextFile(logPath, strings.Join(lines, "\n"))
		t.Logf("accepted sample saved: %s (totalAccepted=%d)", logPath, len(accepted))
		t.Fatalf("A query verification failed: %v", errA)
	}

	// Log ghost transaction analysis for Account A
	ghostCountA := summaryA.InflowCount - inA + (summaryA.OutflowCount - outA) + (summaryA.TransferOutCount - trAB)
	t.Logf("CHAOS_TEST_ACCOUNT_A: actual=%s expected_from_history=%s | HTTP_201: inflows=%d outflows=%d transfers=%d | Actual: inflows=%d outflows=%d transfers_out=%d | Ghosts=~%d",
		actualA.String(), expectedA.String(), inA, outA, trAB, summaryA.InflowCount, summaryA.OutflowCount, summaryA.TransferOutCount, ghostCountA)

	// Verify actual matches expected from history (the test passes if system is internally consistent)
	if !actualA.Equal(expectedA) {
		t.Fatalf("A balance inconsistent with transaction history: actual=%s expected=%s (diff=%s)",
			actualA.String(), expectedA.String(), actualA.Sub(expectedA).String())
	}

	// Verify Account B using transaction history
	actualB, expectedB, summaryB, errB := h.VerifyBalanceConsistencyWithInfo(ctx, trans, org.ID, ledger.ID, aliasB, "USD", seedB, headers)
	if errB != nil {
		t.Fatalf("B query verification failed: %v", errB)
	}

	// Log ghost transaction analysis for Account B
	ghostCountB := summaryB.TransferInCount - trAB + (summaryB.OutflowCount - outB)
	t.Logf("CHAOS_TEST_ACCOUNT_B: actual=%s expected_from_history=%s | HTTP_201: transfers=%d outflows=%d | Actual: transfers_in=%d outflows=%d | Ghosts=~%d",
		actualB.String(), expectedB.String(), trAB, outB, summaryB.TransferInCount, summaryB.OutflowCount, ghostCountB)

	// Verify actual matches expected from history
	if !actualB.Equal(expectedB) {
		t.Fatalf("B balance inconsistent with transaction history: actual=%s expected=%s (diff=%s)",
			actualB.String(), expectedB.String(), actualB.Sub(expectedB).String())
	}

	// Log summary: HTTP expectations vs reality
	t.Logf("CHAOS_TEST_SUMMARY: System internally consistent. HTTP_201 may differ from actual committed due to ghost transactions during chaos.")
	if ghostCountA > 0 || ghostCountB > 0 {
		t.Logf("CHAOS_TEST_GHOST_TRANSACTIONS: Detected ~%d ghost transactions (committed but no 201 received)", ghostCountA+ghostCountB)
	}
}
