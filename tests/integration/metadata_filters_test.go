package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestIntegration_MetadataFilters_Organizations(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Create org with metadata (include valid country)
	payload := h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12))
	payload["metadata"] = map[string]any{"tier": "gold", "region": "emea"}
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, payload)
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	// Filter with metadata.tier=gold (allow a short wait for metadata indexing)
	var list struct {
		Items []struct {
			ID       string         `json:"id"`
			Metadata map[string]any `json:"metadata"`
		} `json:"items"`
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		code, body, err = onboard.Request(ctx, "GET", "/v1/organizations?metadata.tier=gold", headers, nil)
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
	found := false
	for _, it := range list.Items {
		if it.ID == org.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("organization not found via metadata filter within timeout")
	}
}

func TestIntegration_MetadataFilters_Ledgers(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// org + ledger with metadata
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: code=%d err=%v body=%s", code, err, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)

	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-Meta", "metadata": map[string]any{"purpose": "test"}})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: code=%d err=%v body=%s", code, err, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)

	// Filter with metadata.purpose=test
	code, body, err = onboard.Request(ctx, "GET", fmt.Sprintf("/v1/organizations/%s/ledgers?metadata.purpose=test", org.ID), headers, nil)
	if err != nil || code != 200 {
		t.Fatalf("list ledgers by metadata: code=%d err=%v body=%s", code, err, string(body))
	}
	var list struct {
		Items []struct {
			ID       string         `json:"id"`
			Metadata map[string]any `json:"metadata"`
		} `json:"items"`
	}
	_ = json.Unmarshal(body, &list)
	found := false
	for _, it := range list.Items {
		if it.ID == ledger.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ledger not found via metadata filter")
	}
}
