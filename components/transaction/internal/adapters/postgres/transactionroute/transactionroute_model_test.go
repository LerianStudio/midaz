package transactionroute

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

func TestTransactionRouteModel_RoundTrip(t *testing.T) {
	now := time.Now().UTC()
	src := &mmodel.TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "t",
		Description:    "d",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	var model TransactionRoutePostgreSQLModel
	model.FromEntity(src)
	dst := model.ToEntity()

	if dst.ID != src.ID || dst.Title != src.Title || dst.Description != src.Description {
		t.Fatalf("mismatch: %+v vs %+v", dst, src)
	}
}
