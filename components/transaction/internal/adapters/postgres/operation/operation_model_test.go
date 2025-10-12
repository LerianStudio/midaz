package operation

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestOperationModel_RoundTrip(t *testing.T) {
	now := time.Now().UTC()
	amt := decimal.NewFromInt(123)
	bal := decimal.NewFromInt(77)
	route := "R-1"
	desc := "ok"

	src := &Operation{
		ID:              "op-1",
		TransactionID:   "tx-1",
		Description:     "desc",
		Type:            "DEBIT",
		AssetCode:       "USD",
		ChartOfAccounts: "1000",
		Amount:          Amount{Value: &amt},
		Balance:         Balance{Available: &bal, OnHold: &bal},
		BalanceAfter:    Balance{Available: &bal, OnHold: &bal},
		Status:          Status{Code: "ACTIVE", Description: &desc},
		AccountID:       "acc-1",
		AccountAlias:    "@a",
		BalanceKey:      "default",
		BalanceID:       "bal-1",
		OrganizationID:  "org-1",
		LedgerID:        "led-1",
		Route:           route,
		BalanceAffected: true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	var model OperationPostgreSQLModel
	model.FromEntity(src)
	dst := model.ToEntity()

	if dst.ID != src.ID || dst.TransactionID != src.TransactionID || dst.AssetCode != src.AssetCode {
		t.Fatalf("ids/asset mismatch: %+v vs %+v", dst, src)
	}
	if dst.Route != src.Route {
		t.Fatalf("route mismatch: %q vs %q", dst.Route, src.Route)
	}
	if dst.Status.Code != src.Status.Code || (dst.Status.Description == nil || *dst.Status.Description != *src.Status.Description) {
		t.Fatalf("status mismatch: %+v vs %+v", dst.Status, src.Status)
	}
	if dst.Amount.Value == nil || src.Amount.Value == nil || dst.Amount.Value.Cmp(*src.Amount.Value) != 0 {
		t.Fatalf("amount mismatch: %s vs %s", dst.Amount.Value, src.Amount.Value)
	}
}
