package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Mixed inflow/outflow burst on a single account should converge to the
// deterministic final available balance computed from successful operations.
func TestIntegration_Transactions_MixedBurstFinalBalanceConsistent(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()

	iso := h.NewTestIsolation()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := iso.MakeTestHeaders()

	// Setup: org -> ledger -> USD asset -> account -> ensure default -> enable default
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("Org"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("Ledger"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}
	alias := iso.UniqueAccountAlias("diagC")
	accountID, err := h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, orgID, ledgerID, accountID, headers); err != nil {
		t.Fatalf("ensure default: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, orgID, ledgerID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}

	// Seed 100 with unique transaction code and wait briefly until observed
	seedPayload := map[string]any{
		"code": iso.UniqueTransactionCode("SEED"),
		"send": map[string]any{
			"asset": "USD",
			"value": "100.00",
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "100.00"},
				}},
			},
		},
	}
	seedPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID)
	if code, body, err := trans.Request(ctx, "POST", seedPath, headers, seedPayload); err != nil || code != 201 {
		t.Fatalf("seed inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}
	seedWant := decimal.RequireFromString("100.00")
	seedDeadline := time.Now().Add(5 * time.Second)
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) / 2; d < 5*time.Second {
			seedDeadline = time.Now().Add(d)
		}
	}
	for {
		cur, err := h.GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers)
		if err == nil && cur.Equal(seedWant) {
			break
		}
		if time.Now().After(seedDeadline) {
			t.Fatalf("seed mismatch: want=100.00 not observed")
		}
		time.Sleep(75 * time.Millisecond)
	}

	// Burst: 10 outflows of 5, 20 inflows of 2
	outflow := func(val string) (int, []byte, error) {
		p := map[string]any{
			"code": iso.UniqueTransactionCode("OUT"),
			"send": map[string]any{
				"asset":  "USD",
				"value":  val,
				"source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": val}}}},
			},
		}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", orgID, ledgerID), headers, p)
	}
	inflow := func(val string) (int, []byte, error) {
		p := map[string]any{
			"code": iso.UniqueTransactionCode("INF"),
			"send": map[string]any{
				"asset": "USD",
				"value": val,
				"distribute": map[string]any{
					"to": []map[string]any{{
						"accountAlias": alias,
						"amount":       map[string]any{"asset": "USD", "value": val},
					}},
				},
			},
		}
		return trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID), headers, p)
	}

	var wg sync.WaitGroup
	outSucc, inSucc := 0, 0
	mu := sync.Mutex{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, _ := outflow("5.00")
			if c == 201 {
				mu.Lock()
				outSucc++
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, _ := inflow("2.00")
			if c == 201 {
				mu.Lock()
				inSucc++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	exp := decimal.RequireFromString("100").Sub(decimal.NewFromInt(int64(outSucc * 5))).Add(decimal.NewFromInt(int64(inSucc * 2)))

	// Eventually read final balance (cache-aware) and verify
	deadline := time.Now().Add(5 * time.Second)
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) / 2; d < 5*time.Second {
			deadline = time.Now().Add(d)
		}
	}
	var cur decimal.Decimal
	for time.Now().Before(deadline) {
		var err error
		cur, err = h.GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, alias, "USD", headers)
		if err == nil && cur.Equal(exp) {
			break
		}
		time.Sleep(75 * time.Millisecond)
	}
	t.Logf("outSucc=%d inSucc=%d expected=%s got=%s", outSucc, inSucc, exp.String(), cur.String())
	if !cur.Equal(exp) {
		t.Fatalf("final mismatch")
	}
}
