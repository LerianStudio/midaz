package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v4/tests/helpers"
	"github.com/shopspring/decimal"
)

// Repeat identical X-Idempotency requests over restarts/timeouts; expect single net effect.
func TestChaos_IdempotencyAcrossChaos_SingleNetEffect(t *testing.T) {
	shouldRunChaos(t)

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.LedgerURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/asset/account & seed 5
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Idem Chaos "+h.RandString(5), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L-idem"})
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
	alias := "idem-" + h.RandString(4)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: %d %s", code, string(body))
	}
	var acc struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &acc)
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc.ID, headers); err != nil {
		t.Fatalf("ensure default: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "5.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "5.00"}}}}}})
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("5.00"), 10*time.Second); err != nil {
		t.Fatalf("seed wait: %v", err)
	}

	// Prepare idempotent inflow 3.00
	idem := "idk-" + h.RandHex(10)
	idemHeaders := h.AuthHeaders(h.RandHex(6))
	idemHeaders["X-Idempotency"] = idem
	idemHeaders["X-TTL"] = "120"
	p := map[string]any{"send": map[string]any{"asset": "USD", "value": "3.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "3.00"}}}}}}
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID)

	// First call should apply effect
	code, _, _, err = trans.RequestFull(ctx, "POST", path, idemHeaders, p)
	if err != nil || (code != 201 && code != 409) {
		t.Fatalf("first idem call: code=%d err=%v", code, err)
	}

	// Pause service to simulate timeout, call again (likely network error)
	_ = h.DockerAction("pause", "midaz-ledger")
	_, _, _, _ = trans.RequestFull(ctx, "POST", path, idemHeaders, p)
	_ = h.DockerAction("unpause", "midaz-ledger")
	_ = h.WaitForHTTP200(env.LedgerURL+"/health", 30*time.Second)

	// Restart service and call again; expect replay/409 but no second effect
	_ = h.DockerAction("restart", "midaz-ledger")
	_ = h.WaitForHTTP200(env.LedgerURL+"/health", 60*time.Second)
	code, _, _, _ = trans.RequestFull(ctx, "POST", path, idemHeaders, p)
	if !(code == 201 || code == 409) {
		t.Fatalf("post-restart idem call code=%d", code)
	}

	// Final should be 8.00 (5 + 3) not 11
	if _, err := h.WaitForAvailableSumByAlias(ctx, trans, org.ID, ledger.ID, alias, "USD", headers, decimal.RequireFromString("8.00"), 30*time.Second); err != nil {
		t.Fatalf("final wait after idempotent chaos: %v", err)
	}
}
