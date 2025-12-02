package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
)

func TestIntegration_CountEndpoints(t *testing.T) {
	env := h.LoadEnvironment()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))
	ctx := context.Background()

	// Create isolated org
	orgPayload := h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12))
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, orgPayload)
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	// HEAD organizations count
	code, _, hdr, err := onboard.RequestFull(ctx, "HEAD", "/v1/organizations/metrics/count", headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("org count head: code=%d err=%v", code, err)
	}
	if _, err := strconv.Atoi(hdr.Get("X-Total-Count")); err != nil {
		t.Fatalf("missing or invalid X-Total-Count header")
	}

	// Create ledger and verify ledger count header under this org
	ledgerPayload := map[string]any{"name": fmt.Sprintf("Ledger %s", h.RandString(4))}
	code, ledgerBody, err := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, ledgerPayload)
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(ledgerBody))
	}
	ledgerID := getID(ledgerBody)

	code, _, hdr, err = onboard.RequestFull(ctx, "HEAD", fmt.Sprintf("/v1/organizations/%s/ledgers/metrics/count", org.ID), headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("ledger count head: code=%d err=%v", code, err)
	}
	if _, err := strconv.Atoi(hdr.Get("X-Total-Count")); err != nil {
		t.Fatalf("missing or invalid X-Total-Count header for ledgers")
	}

	// Accounts count before creating new account
	// Poll HEAD count before creating account
	var beforeCount int
	{
		deadline := time.Now().Add(2 * time.Second)
		for {
			code, _, hdr, err = onboard.RequestFull(ctx, "HEAD", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/metrics/count", org.ID, ledgerID), headers, nil)
			if err == nil && code == 204 {
				if v, conv := strconv.Atoi(hdr.Get("X-Total-Count")); conv == nil {
					beforeCount = v
					break
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("accounts count head (before) not ready")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Create asset USD required for accounts
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// Create account and verify accounts count header increases
	alias := fmt.Sprintf("acc-%s", h.RandString(5))
	accountPayload := map[string]any{"name": "Cash", "assetCode": "USD", "type": "deposit", "alias": alias}
	code, acctBody, err := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledgerID), headers, accountPayload)
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(acctBody))
	}

	// Poll until count increases
	var afterCount int
	{
		deadline := time.Now().Add(3 * time.Second)
		for {
			code, _, hdr, err = onboard.RequestFull(ctx, "HEAD", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/metrics/count", org.ID, ledgerID), headers, nil)
			if err == nil && code == 204 {
				if v, conv := strconv.Atoi(hdr.Get("X-Total-Count")); conv == nil {
					afterCount = v
					if afterCount >= beforeCount+1 {
						break
					}
				}
			}
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
	if afterCount < beforeCount+1 {
		t.Fatalf("expected accounts count to increase by at least 1: before=%d after=%d", beforeCount, afterCount)
	}
}

func TestIntegration_LedgersPaginationAndValidation(t *testing.T) {
	env := h.LoadEnvironment()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))
	ctx := context.Background()

	// Create org
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(4)), h.RandString(10)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	orgID := getID(body)

	// Create 3 ledgers
	for i := 0; i < 3; i++ {
		_, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", orgID), headers, map[string]any{"name": fmt.Sprintf("L-%d-%s", i, h.RandString(3))})
	}

	// GET with limit=2
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers?limit=2", orgID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list ledgers limit=2: code=%d err=%v body=%s", code, err, string(body))
	}
	var resp struct {
		Items []any `json:"items"`
		Limit int   `json:"limit"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("parse list: %v body=%s", err, string(body))
	}
	if len(resp.Items) > 2 {
		t.Fatalf("expected <=2 items, got %d", len(resp.Items))
	}

	// Invalid limit (exceeds max) -> 400
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers?limit=1000", orgID), headers, nil)
	if err != nil || code != 400 {
		t.Fatalf("expected 400 for limit>max, got %d err=%v body=%s", code, err, string(body))
	}

	// Invalid sort_order -> 400
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers?sort_order=sideways", orgID), headers, nil)
	if err != nil || code != 400 {
		t.Fatalf("expected 400 for invalid sort_order, got %d err=%v body=%s", code, err, string(body))
	}

	// start_date without end_date -> 400
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers?start_date=2024-01-01", orgID), headers, nil)
	if err != nil || code != 400 {
		t.Fatalf("expected 400 for start_date without end_date, got %d err=%v body=%s", code, err, string(body))
	}
}

// getID extracts {"id":"..."} from response body.
func getID(body []byte) string {
	var m struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &m)
	return m.ID
}
