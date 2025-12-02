package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestIntegration_Accounts_FilterByMetadata(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// org + ledger
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
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

	// accounts with metadata
	_, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{
		"name": "Cash", "assetCode": "USD", "type": "deposit", "alias": "f-m1-" + h.RandString(4),
		"metadata": map[string]any{"group": "cash"},
	})
	_, _, _ = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{
		"name": "Ops", "assetCode": "USD", "type": "deposit", "alias": "f-m2-" + h.RandString(4),
		"metadata": map[string]any{"group": "ops"},
	})

	// filter by metadata.group=cash (allow a short wait for metadata indexing)
	var list struct {
		Items []struct{ Metadata map[string]any } `json:"items"`
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts?metadata.group=cash", org.ID, ledger.ID), headers, nil)
		if err == nil && code == 200 {
			_ = json.Unmarshal(body, &list)
			if len(list.Items) > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if len(list.Items) == 0 {
		t.Fatalf("no accounts returned via metadata filter within timeout")
	}
	for _, it := range list.Items {
		if g, ok := it.Metadata["group"].(string); !ok || g != "cash" {
			t.Fatalf("unexpected account in filtered result: %+v", it.Metadata)
		}
	}
}
