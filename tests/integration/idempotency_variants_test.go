package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

// Outflow idempotency behavior mirrors inflow.
func TestIntegration_Idempotency_Outflow(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup org/ledger/account
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
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
	alias := fmt.Sprintf("acc-%s", h.RandString(5))
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": alias})
	if err != nil || code != 201 {
		t.Fatalf("create account: code=%d err=%v body=%s", code, err, string(body))
	}
	var acct struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &acct)
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acct.ID, headers); err != nil {
		t.Fatalf("ensure default ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, alias, headers); err != nil {
		t.Fatalf("enable default: %v", err)
	}

	// Ensure some funds exist by an inflow
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "5.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "5.00"}}}}}})

	outflow := map[string]any{"send": map[string]any{"asset": "USD", "value": "2.00", "source": map[string]any{"from": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "2.00"}}}}}}
	reqHeaders := h.AuthHeaders(h.RandHex(8))
	reqHeaders[headerIdempotencyKey] = "i-" + h.RandHex(6)
	reqHeaders[headerIdempotencyTTL] = "60"
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/outflow", org.ID, ledger.ID)

	code, body1, err := trans.Request(ctx, "POST", path, reqHeaders, outflow)
	if err != nil || code != 201 {
		t.Fatalf("first outflow: code=%d err=%v body=%s", code, err, string(body1))
	}
	time.Sleep(150 * time.Millisecond)
	code, body2, hdr, err := trans.RequestFull(ctx, "POST", path, reqHeaders, outflow)
	if err != nil {
		t.Fatalf("second outflow err: %v", err)
	}
	switch {
	case code == 201:
		if strings.ToLower(hdr.Get(headerReplayed)) != "true" || string(body1) != string(body2) {
			t.Fatalf("expected replay true with identical body")
		}
	case code == 409:
		time.Sleep(250 * time.Millisecond)
		code3, _, hdr3, err3 := trans.RequestFull(ctx, "POST", path, reqHeaders, outflow)
		if err3 != nil || code3 != 201 || strings.ToLower(hdr3.Get(headerReplayed)) != "true" {
			t.Fatalf("expected replay after conflict: code=%d hdr=%s err=%v", code3, hdr3.Get(headerReplayed), err3)
		}
	default:
		t.Fatalf("unexpected status: %d body=%s", code, string(body2))
	}
}

func TestIntegration_Idempotency_JSON(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
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

	aliasA := fmt.Sprintf("a-%s", h.RandString(4))
	aliasB := fmt.Sprintf("b-%s", h.RandString(4))
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": aliasA})
	if err != nil || code != 201 {
		t.Fatalf("create account A: code=%d err=%v body=%s", code, err, string(body))
	}
	var accA struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &accA)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "B", "assetCode": "USD", "type": "deposit", "alias": aliasB})
	if err != nil || code != 201 {
		t.Fatalf("create account B: code=%d err=%v body=%s", code, err, string(body))
	}
	var accB struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &accB)
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, accA.ID, headers); err != nil {
		t.Fatalf("ensure default A ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasA, headers); err != nil {
		t.Fatalf("enable default A: %v", err)
	}
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, accB.ID, headers); err != nil {
		t.Fatalf("ensure default B ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasB, headers); err != nil {
		t.Fatalf("enable default B: %v", err)
	}

	// Seed funds to A
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "9.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "9.00"}}}}}})

	jsonTxn := map[string]any{
		"send": map[string]any{
			"asset":      "USD",
			"value":      "3.00",
			"source":     map[string]any{"from": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "3.00"}}}},
			"distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasB, "amount": map[string]any{"asset": "USD", "value": "3.00"}}}},
		},
	}

	reqHeaders := h.AuthHeaders(h.RandHex(8))
	reqHeaders[headerIdempotencyKey] = "i-" + h.RandHex(6)
	reqHeaders[headerIdempotencyTTL] = "60"
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/json", org.ID, ledger.ID)

	code, body1, err := trans.Request(ctx, "POST", path, reqHeaders, jsonTxn)
	if err != nil || code != 201 {
		t.Fatalf("first json txn: code=%d err=%v body=%s", code, err, string(body1))
	}
	time.Sleep(150 * time.Millisecond)
	code, body2, hdr, err := trans.RequestFull(ctx, "POST", path, reqHeaders, jsonTxn)
	if err != nil {
		t.Fatalf("second json txn err: %v", err)
	}
	switch {
	case code == 201:
		if strings.ToLower(hdr.Get(headerReplayed)) != "true" || string(body1) != string(body2) {
			t.Fatalf("expected replay true with identical body")
		}
	case code == 409:
		time.Sleep(250 * time.Millisecond)
		code3, _, hdr3, err3 := trans.RequestFull(ctx, "POST", path, reqHeaders, jsonTxn)
		if err3 != nil || code3 != 201 || strings.ToLower(hdr3.Get(headerReplayed)) != "true" {
			t.Fatalf("expected replay after conflict: code=%d hdr=%s err=%v", code3, hdr3.Get(headerReplayed), err3)
		}
	default:
		t.Fatalf("unexpected status: %d body=%s", code, string(body2))
	}
}

func TestIntegration_Idempotency_DSL(t *testing.T) {
	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)
	headers := h.AuthHeaders(h.RandHex(8))

	// Setup
	code, body, err := onboard.Request(ctx, "POST", "/v1/organizations", headers, h.OrgPayload(fmt.Sprintf("Org %s", h.RandString(5)), h.RandString(12)))
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
	aliasA := fmt.Sprintf("a-%s", h.RandString(4))
	aliasB := fmt.Sprintf("b-%s", h.RandString(4))
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "A", "assetCode": "USD", "type": "deposit", "alias": aliasA})
	if err != nil || code != 201 {
		t.Fatalf("create account A: code=%d err=%v body=%s", code, err, string(body))
	}
	var acc1 struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &acc1)
	code, body, err = onboard.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/accounts", org.ID, ledger.ID), headers, map[string]any{"name": "B", "assetCode": "USD", "type": "deposit", "alias": aliasB})
	if err != nil || code != 201 {
		t.Fatalf("create account B: code=%d err=%v body=%s", code, err, string(body))
	}
	var acc2 struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &acc2)
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc1.ID, headers); err != nil {
		t.Fatalf("ensure default A ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasA, headers); err != nil {
		t.Fatalf("enable default A: %v", err)
	}
	if err := h.EnsureDefaultBalanceRecord(ctx, trans, org.ID, ledger.ID, acc2.ID, headers); err != nil {
		t.Fatalf("ensure default B ready: %v", err)
	}
	if err := h.EnableDefaultBalance(ctx, trans, org.ID, ledger.ID, aliasB, headers); err != nil {
		t.Fatalf("enable default B: %v", err)
	}

	// Seed funds to A
	_, _, _ = trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", org.ID, ledger.ID), headers, map[string]any{"send": map[string]any{"asset": "USD", "value": "13.00", "distribute": map[string]any{"to": []map[string]any{{"accountAlias": aliasA, "amount": map[string]any{"asset": "USD", "value": "13.00"}}}}}})

	// Build simple DSL (integer units)
	// (transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 9|9 (source (from @A :amount USD 9|9)) (distribute (to @B :amount USD 9|9))))
	dsl := fmt.Sprintf("(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 9|9 (source (from @%s :amount USD 9|9)) (distribute (to @%s :amount USD 9|9))))", aliasA, aliasB)

	reqHeaders := h.AuthHeaders(h.RandHex(8))
	reqHeaders[headerIdempotencyKey] = "i-" + h.RandHex(6)
	reqHeaders[headerIdempotencyTTL] = "60"
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/dsl", org.ID, ledger.ID)

	code, body1, err := func() (int, []byte, error) {
		code, body, _, err := trans.PostDSL(ctx, path, reqHeaders, dsl)
		return code, body, err
	}()
	if err != nil {
		t.Fatalf("first dsl txn err: %v", err)
	}
	if !(code == 201 || code == 200) {
		t.Fatalf("first dsl txn unexpected status: code=%d body=%s", code, string(body1))
	}
	time.Sleep(150 * time.Millisecond)
	code, body2, hdr, err := trans.PostDSL(ctx, path, reqHeaders, dsl)
	if err != nil {
		t.Fatalf("second dsl txn err: %v", err)
	}
	if code == 201 || code == 200 {
		if strings.ToLower(hdr.Get(headerReplayed)) != "true" || string(body1) != string(body2) {
			t.Fatalf("expected replay true with identical body (dsl)")
		}
	} else if code == 409 {
		time.Sleep(250 * time.Millisecond)
		code3, _, hdr3, err3 := trans.PostDSL(ctx, path, reqHeaders, dsl)
		if err3 != nil || (code3 != 201 && code3 != 200) || strings.ToLower(hdr3.Get(headerReplayed)) != "true" {
			t.Fatalf("expected replay after conflict (dsl): code=%d hdr=%s err=%v", code3, hdr3.Get(headerReplayed), err3)
		}
	} else {
		t.Fatalf("unexpected status (dsl): %d body=%s", code, string(body2))
	}
}
