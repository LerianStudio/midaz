package operationroute

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

func TestOperationRouteModel_RoundTrip_StringValidIf(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()

	src := &mmodel.OperationRoute{
		ID:             id,
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "t",
		Description:    "d",
		Code:           "CODE",
		OperationType:  "source",
		Account:        &mmodel.AccountRule{RuleType: "alias", ValidIf: "@a"},
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	var model OperationRoutePostgreSQLModel
	model.FromEntity(src)
	dst := model.ToEntity()

	if dst.ID != src.ID || dst.Title != src.Title || dst.Code != src.Code {
		t.Fatalf("basic mismatch: %+v vs %+v", dst, src)
	}
	if dst.Account == nil || dst.Account.RuleType != "alias" || dst.Account.ValidIf.(string) != "@a" {
		t.Fatalf("account rule mismatch: %+v", dst.Account)
	}
}

func TestOperationRouteModel_RoundTrip_AccountTypeSliceValidIf(t *testing.T) {
	src := &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "t",
		Description:    "d",
		OperationType:  "destination",
		Account:        &mmodel.AccountRule{RuleType: "account_type", ValidIf: []string{"deposit", "internal"}},
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	var model OperationRoutePostgreSQLModel
	model.FromEntity(src)
	dst := model.ToEntity()

	if dst.Account == nil {
		t.Fatalf("expected account rule")
	}
	vals, ok := dst.Account.ValidIf.([]string)
	if !ok || len(vals) != 2 {
		t.Fatalf("expected []string validIf, got: %#v", dst.Account.ValidIf)
	}
}
