package balance

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

func TestModelConversion_RoundTrip(t *testing.T) {
	now := time.Now().UTC()

	src := &mmodel.Balance{
		ID:             "id-1",
		OrganizationID: "org-1",
		LedgerID:       "led-1",
		AccountID:      "acc-1",
		Alias:          "alias-1",
		Key:            "key-1",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.NewFromInt(5),
		Version:        2,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	var model BalancePostgreSQLModel
	model.FromEntity(src)

	if model.Key != src.Key {
		t.Fatalf("key mismatch: want %q got %q", src.Key, model.Key)
	}
	if model.Available.Cmp(src.Available) != 0 || model.OnHold.Cmp(src.OnHold) != 0 {
		t.Fatalf("amount mismatch: want %s/%s got %s/%s", src.Available, src.OnHold, model.Available, model.OnHold)
	}

	dst := model.ToEntity()
	if dst.ID != src.ID || dst.OrganizationID != src.OrganizationID || dst.LedgerID != src.LedgerID {
		t.Fatalf("ids mismatch: %+v vs %+v", dst, src)
	}
	if dst.Key != src.Key {
		t.Fatalf("roundtrip key mismatch: want %q got %q", src.Key, dst.Key)
	}
	if dst.Available.Cmp(src.Available) != 0 || dst.OnHold.Cmp(src.OnHold) != 0 {
		t.Fatalf("roundtrip amount mismatch: want %s/%s got %s/%s", src.Available, src.OnHold, dst.Available, dst.OnHold)
	}
	if dst.AllowSending != src.AllowSending || dst.AllowReceiving != src.AllowReceiving {
		t.Fatalf("permissions mismatch: want %v/%v got %v/%v", src.AllowSending, src.AllowReceiving, dst.AllowSending, dst.AllowReceiving)
	}
}

func TestModelConversion_DefaultKeyWhenEmpty(t *testing.T) {
	src := &mmodel.Balance{
		ID:             "id-2",
		OrganizationID: "org-1",
		LedgerID:       "led-1",
		AccountID:      "acc-1",
		Alias:          "alias-1",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(0),
		OnHold:         decimal.Zero,
		Version:        0,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	var model BalancePostgreSQLModel
	model.FromEntity(src)
	if model.Key != "default" {
		t.Fatalf("expected default key, got %q", model.Key)
	}
}
