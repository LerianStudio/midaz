package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Confirms DSL vs JSON parity on identical transfer; currently DSL returns 422 (Account Ineligibility) while JSON succeeds.
func TestDiagnostic_DSLvsJSONParity(t *testing.T) {
	// Always run as we converge DSL/JSON parity
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/assets/accounts
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload("Diag Org "+h.RandString(5), h.RandString(12)))
	if err != nil || code != 201 {
		t.Fatalf("create org: %d %s", code, string(body))
	}
	var org struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &org)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers", org.ID), headers, map[string]any{"name": "L"})
	if err != nil || code != 201 {
		t.Fatalf("create ledger: %d %s", code, string(body))
	}
	var ledger struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &ledger)
	if err := h.CreateUSDAsset(ctx, onboard, org.ID, ledger.ID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}

	aliasA := "diagA-" + h.RandString(4)
	aliasB := "diagB-" + h.RandString(4)
	// create accounts
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
	// enable balances
	for _, al := range []string{aliasA, aliasB} {
		if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, al, headers); err != nil {
			t.Fatalf("enable default %s: %v", al, err)
		}
	}
	// seed funds to A
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "9.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "9.00"}}}}}})

	// JSON transfer 3.00 A->B
	jsonTxn := map[string]any{"send": map[string]any{"asset": "USD", "value": "3.00", "source": map[string]any{"from": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "3.00"}}}}, "distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasB, "amount": map[string]any{"asset": "USD", "value": "3.00"}}}}}}
	code, body, err = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", org.ID, ledger.ID), headers, jsonTxn)
	if err != nil || code != 201 {
		t.Fatalf("json transfer: %d %s", code, string(body))
	}

	// DSL with same semantics
	dsl := fmt.Sprintf("(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 3|0 (source (from @%s :amount USD 3|0)) (distribute (to @%s :amount USD 3|0))))", aliasA, aliasB)
	code, body, hdr, err := trans.PostDSL(ctx, fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/dsl", org.ID, ledger.ID), headers, dsl)
	if err != nil {
		t.Fatalf("DSL request error: %v", err)
	}
	t.Logf("DSL status=%d replay=%s body=%s", code, hdr.Get("X-Idempotency-Replayed"), string(body))
	if code == 201 || code == 200 {
		t.Log("DSL accepted; parity behavior OK")
	} else {
		t.Fatalf("DSL rejected status=%d body=%s", code, string(body))
	}
}
