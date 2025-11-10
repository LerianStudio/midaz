package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	h "github.com/LerianStudio/midaz/v3/tests/helpers"
)

func TestIntegration_Transaction_GetByID(t *testing.T) {
	t.Parallel()

	env := h.LoadEnvironment()
	ctx := context.Background()
	onboard := h.NewHTTPClient(env.OnboardingURL, env.HTTPTimeout)
	trans := h.NewHTTPClient(env.TransactionURL, env.HTTPTimeout)

	iso := h.NewTestIsolation()
	headers := iso.MakeTestHeaders()

	// setup org/ledger/account via helpers
	orgID, err := h.SetupOrganization(ctx, onboard, headers, iso.UniqueOrgName("Org"))
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	ledgerID, err := h.SetupLedger(ctx, onboard, headers, orgID, iso.UniqueLedgerName("Ledger"))
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := h.CreateUSDAsset(ctx, onboard, orgID, ledgerID, headers); err != nil {
		t.Fatalf("create USD asset: %v", err)
	}
	alias := iso.UniqueAccountAlias("tget")
	_, err = h.SetupAccount(ctx, onboard, headers, orgID, ledgerID, alias, "USD")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// create inflow with unique code
	inflow := map[string]any{
		"code": iso.UniqueTransactionCode("INF"),
		"send": map[string]any{
			"asset": "USD", "value": "2.00",
			"distribute": map[string]any{"to": []map[string]any{{"accountAlias": alias, "amount": map[string]any{"asset": "USD", "value": "2.00"}}}},
		},
	}
	code, body, err := trans.Request(ctx, "POST", fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/inflow", orgID, ledgerID), headers, inflow)
	if err != nil || code != 201 {
		t.Fatalf("create inflow: code=%d err=%v body=%s", code, err, string(body))
	}
	var tx struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &tx)

	// Eventually GET by ID until available
	getPath := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", orgID, ledgerID, tx.ID)
	deadline := time.Now().Add(5 * time.Second)
	if dl, ok := t.Deadline(); ok {
		if d := time.Until(dl) / 2; d < 5*time.Second {
			deadline = time.Now().Add(d)
		}
	}
	var lastCode int
	var lastBody []byte
	for {
		c, b, e := trans.Request(ctx, "GET", getPath, headers, nil)
		if e == nil && c == 200 {
			lastCode, lastBody = c, b
			break
		}
		lastCode, lastBody = c, b
		if time.Now().After(deadline) {
			t.Fatalf("get transaction by id not ready: code=%d body=%s", lastCode, string(lastBody))
		}
		time.Sleep(75 * time.Millisecond)
	}

	// Assert response fields
	var txGet struct {
		ID             string `json:"id"`
		OrganizationID string `json:"organizationId"`
		LedgerID       string `json:"ledgerId"`
		Status         struct {
			Code string `json:"code"`
		} `json:"status"`
	}
	if err := json.Unmarshal(lastBody, &txGet); err != nil {
		t.Fatalf("parse tx: %v", err)
	}
	if txGet.ID != tx.ID {
		t.Fatalf("transaction id mismatch: want=%s got=%s", tx.ID, txGet.ID)
	}
	if txGet.OrganizationID == "" || txGet.LedgerID == "" || txGet.Status.Code == "" {
		t.Fatalf("unexpected empty fields in transaction: %+v", txGet)
	}
}
