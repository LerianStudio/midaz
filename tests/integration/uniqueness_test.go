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

func TestIntegration_AccountAliasUniqueness_Conflict(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// fetch assets to verify USD presence
	cdbg, bdbg, _ := onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/assets", org.ID, ledger.ID), headers, nil)
	if cdbg != 200 {
		t.Fatalf("assets listing status=%d body=%s", cdbg, string(bdbg))
	}
	var assets struct {
		Items []struct {
			Code string `json:"code"`
		} `json:"items"`
	}
	_ = json.Unmarshal(bdbg, &assets)
	hasUSD := false
	for _, it := range assets.Items {
		if it.Code == "USD" {
			hasUSD = true
			break
		}
	}
	if !hasUSD {
		t.Fatalf("USD asset not visible before account creation; assets=%s", string(bdbg))
	}
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	alias := fmt.Sprintf("dup-%s", h.RandString(5))
	payload := map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias}
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, payload)
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}

	// duplicate alias in same ledger should conflict
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, payload)
	if err != nil || code != 409 {
		t.Fatalf("expected 409 for duplicated alias, got %d err=%v body=%s", code, err, string(body))
	}
}

func TestIntegration_AccountsHeadCount(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(6)), h.RandString(14)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": fmt.Sprintf("L-%s", h.RandString(4))})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	// ensure USD asset exists before creating accounts
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	// head count before
	code, _, hdr, err := onboard.RequestFull(ctx, "HEAD", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/metrics/count", org.ID, ledger.ID), headers, nil)
	if err != nil || code != 204 {
		t.Fatalf("accounts head before: code=%d err=%v", code, err)
	}
	before, _ := strconv.Atoi(hdr.Get("X-Total-Count"))

	// create two accounts
	for i := 0; i < 2; i++ {
		alias := fmt.Sprintf("u-%d-%s", i, h.RandString(3))
		c, b, e := onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
		if e != nil || c != 201 {
			t.Fatalf("create account %d: code=%d err=%v body=%s", i, c, e, string(b))
		}
	}

	// poll to observe the increase
	var after int
	{
		deadline := time.Now().Add(3 * time.Second)
		for {
			code, _, hdr, err = onboard.RequestFull(ctx, "HEAD", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts/metrics/count", org.ID, ledger.ID), headers, nil)
			if err == nil && code == 204 {
				if v, conv := strconv.Atoi(hdr.Get("X-Total-Count")); conv == nil {
					after = v
					if after >= before+2 {
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
	if after < before+2 {
		t.Fatalf("accounts head expected increase by >=2, before=%d after=%d", before, after)
	}
}
