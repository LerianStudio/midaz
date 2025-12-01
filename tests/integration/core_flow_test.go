package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Core integration: org → ledger → account → simple transactions
func TestIntegration_CoreOrgLedgerAccountAndTransactions(t *testing.T) {
	t.Parallel()
	env := h.LoadEnvironment()
	ctx := context.Background()

	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// 1) Create Organization
	orgPayload := h.OrgPayload(fmt.Sprintf("Test Org %s", h.RandString(5)), h.RandString(12))
	orgPayload["metadata"] = map[string]any{"env": "test"}
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, orgPayload)
	if err != nil || code != 201 {
		t.Fatalf("create organization failed: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &org); err != nil || org.ID == "" {
		t.Fatalf("parse organization: %v body=%s", err, string(body))
	}

	// 2) Create Ledger
	ledgerPayload := map[string]any{
		"name": fmt.Sprintf("Ledger %s", h.RandString(5)),
	}
	pathLedger := fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID)
	code, body, err = onboard.Request(ctx, "POST", pathLedger, headers, ledgerPayload)
	if err != nil || code != 201 {
		t.Fatalf("create ledger failed: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &ledger); err != nil || ledger.ID == "" {
		t.Fatalf("parse ledger: %v body=%s", err, string(body))
	}

	// Ensure USD asset exists for this ledger
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// 3) Create Account (with alias and assetCode)
	alias := fmt.Sprintf("cash-%s", h.RandString(6))
	accountPayload := map[string]any{
		"name":      "Cash Account",
		"assetCode": "USD",
		"type":      "deposit",
		"alias":     alias,
	}
	pathAccount := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID)
	code, body, err = onboard.Request(ctx, "POST", pathAccount, headers, accountPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account failed: code=%d err=%v body=%s", code, err, string(body))
	}
	var account struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &account); err != nil || account.ID == "" {
		t.Fatalf("parse account: %v body=%s", err, string(body))
	}

	// 4) Simple inflow
	inflow := map[string]any{
		"code":        fmt.Sprintf("TR-INF-%s", h.RandString(5)),
		"description": "test inflow",
		"send": map[string]any{
			"asset": "USD",
			"value": "100.00",
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "100.00"},
					"description":  "credit cash",
				}},
			},
		},
		"metadata": map[string]any{"env": "test"},
	}
	pathInflow := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)
	code, body, err = trans.Request(ctx, "POST", pathInflow, headers, inflow)
	if err != nil || code != 201 {
		t.Fatalf("inflow failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 5) Simple outflow
	outflow := map[string]any{
		"code":        fmt.Sprintf("TR-OUT-%s", h.RandString(5)),
		"description": "test outflow",
		"pending":     false,
		"send": map[string]any{
			"asset": "USD",
			"value": "40.00",
			"source": map[string]any{
				"from": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "40.00"},
					"description":  "debit cash",
				}},
			},
		},
		"metadata": map[string]any{"env": "test"},
	}
	pathOutflow := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID)
	code, body, err = trans.Request(ctx, "POST", pathOutflow, headers, outflow)
	if err != nil || code != 201 {
		t.Fatalf("outflow failed: code=%d err=%v body=%s", code, err, string(body))
	}

	// 6) Verify balances by alias endpoint returns 200 and payload shape
	pathBalances := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/alias/%s/balances", org.ID, ledger.ID, alias)
	code, body, err = trans.Request(ctx, "GET", pathBalances, headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("balances by alias failed: code=%d err=%v body=%s", code, err, string(body))
	}
	var paged struct {
		Items []struct {
			AssetCode string          `json:"assetCode"`
			Available decimal.Decimal `json:"available"`
			OnHold    decimal.Decimal `json:"onHold"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &paged); err != nil {
		t.Fatalf("parse balances: %v body=%s", err, string(body))
	}
	if len(paged.Items) == 0 {
		t.Fatalf("no balances returned for alias %s", alias)
	}

	// Wait until the available USD sum equals 60.00
	expected, _ := decimal.NewFromString("60.00")
	timeout := 5 * time.Second
	if td, ok := t.Deadline(); ok {
		remaining := time.Until(td)
		// If deadline already passed, fail fast and avoid negative durations
		// to prevent panics in helpers that use time.NewTicker.
		if remaining <= 0 {
			timeout = 1 * time.Millisecond
		} else if d := remaining / 2; d < timeout {
			timeout = d
		}
	}
	sum, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, expected, timeout)
	if err != nil {
		t.Fatalf("unexpected USD available sum: %v (last=%s expected=%s)", err, sum.String(), expected.String())
	}
}
