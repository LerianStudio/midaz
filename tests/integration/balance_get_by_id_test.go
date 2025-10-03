package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
	"github.com/shopspring/decimal"
)

// Integration tests for GET /v1/organizations/{org}/ledgers/{ledger}/balances/{balance_id}
// Scenarios: no-cache overlay and cache overlay after an inflow.
func TestIntegration_Balance_GetByID_NoCacheAndOverlay(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup: organization
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	// Ledger
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	// Ensure USD asset exists
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// Account
	alias := fmt.Sprintf("bid-%s", h.RandString(5))
	accPayload := map[string]any{"name": alias, "assetCode": "USD", "type": "deposit", "alias": alias}
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, accPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var acct struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &acct)

	// Wait default balance and enable
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acct.ID, headers); err != nil {
		t.Fatalf("ensure default ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}

	// List balances by account to get the default balance ID (no overlay yet expected)
	code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/%s/balances", org.ID, ledger.ID, acct.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list balances: code=%d err=%v body=%s", code, err, string(body))
	}
	var list struct {
		Items []struct {
			ID        string          `json:"id"`
			Key       string          `json:"key"`
			Available decimal.Decimal `json:"available"`
			OnHold    decimal.Decimal `json:"onHold"`
		} `json:"items"`
	}
	_ = json.Unmarshal(body, &list)
	var balanceID string
	for _, it := range list.Items {
		if it.Key == "default" {
			balanceID = it.ID
			break
		}
	}
	if balanceID == "" {
		t.Fatalf("default balance id not found in listing")
	}

	// GET by ID should succeed; before inflow, available may be zero
	code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", org.ID, ledger.ID, balanceID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get balance by id (pre): code=%d err=%v body=%s", code, err, string(body))
	}
	var bal struct {
		ID             string          `json:"id"`
		OrganizationID string          `json:"organizationId"`
		LedgerID       string          `json:"ledgerId"`
		Available      decimal.Decimal `json:"available"`
		OnHold         decimal.Decimal `json:"onHold"`
		Version        int64           `json:"version"`
	}
	if e := json.Unmarshal(body, &bal); e != nil {
		t.Fatalf("parse balance: %v", e)
	}
	if bal.ID != balanceID {
		t.Fatalf("balance id mismatch: want=%s got=%s", balanceID, bal.ID)
	}

	// Inflow to create overlay values in Redis
	inflow := map[string]any{
		"code":        fmt.Sprintf("TR-BID-%s", url.QueryEscape(h.RandString(6))),
		"description": "credit default",
		"send": map[string]any{
			"asset": "USD",
			"value": "25.00",
			"distribute": map[string]any{
				"to": []map[string]any{{
					"accountAlias": alias,
					"amount":       map[string]any{"asset": "USD", "value": "25.00"},
				}},
			},
		},
	}
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, inflow)
	if err != nil || code != 201 {
		t.Fatalf("inflow: code=%d err=%v body=%s", code, err, string(body))
	}

	// Poll GET by ID until available reflects overlay or timeout
	deadline := time.Now().Add(10 * time.Second)
	for {
		code, body, err = trans.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/balances/%s", org.ID, ledger.ID, balanceID), headers, nil)
		if err == nil && code == 200 {
			var got struct {
				Available decimal.Decimal `json:"available"`
			}
			_ = json.Unmarshal(body, &got)
			if got.Available.GreaterThan(decimal.Zero) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting overlay; last attempt code=%d err=%v body=%s", code, err, string(body))
		}
		time.Sleep(150 * time.Millisecond)
	}
}
