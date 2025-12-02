package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
)

func TestIntegration_Portfolio_UpdateAndGet(t *testing.T) {
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

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
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

	// create portfolio
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/portfolios", org.ID, ledger.ID), headers, map[string]any{"name": "Main Portfolio"})
	if err != nil || code != 201 {
		t.Fatalf("create portfolio: code=%d err=%v body=%s", code, err, string(body))
	}
	var portfolio struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &portfolio)

	// get portfolio by id
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/portfolios/%s", org.ID, ledger.ID, portfolio.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get portfolio: code=%d err=%v body=%s", code, err, string(body))
	}

	// update portfolio
	code, body, err = onboard.Request(ctx, "PATCH", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/portfolios/%s", org.ID, ledger.ID, portfolio.ID), headers, map[string]any{"name": "Portfolio Updated", "metadata": map[string]any{"stage": "prod"}})
	if err != nil || code != 200 {
		t.Fatalf("update portfolio: code=%d err=%v body=%s", code, err, string(body))
	}
}

func TestIntegration_Segment_UpdateAndGet(t *testing.T) {
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

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
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

	// create segment
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/segments", org.ID, ledger.ID), headers, map[string]any{"name": "Retail"})
	if err != nil || code != 201 {
		t.Fatalf("create segment: code=%d err=%v body=%s", code, err, string(body))
	}
	var segment struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &segment)

	// get segment by id
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/segments/%s", org.ID, ledger.ID, segment.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("get segment: code=%d err=%v body=%s", code, err, string(body))
	}

	// update segment
	code, body, err = onboard.Request(ctx, "PATCH", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/segments/%s", org.ID, ledger.ID, segment.ID), headers, map[string]any{"name": "Retail-Updated"})
	if err != nil || code != 200 {
		t.Fatalf("update segment: code=%d err=%v body=%s", code, err, string(body))
	}
}
