package services

import (
	"context"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func Test_GetAuditInfo(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()

	mockedAudit := audit.Audit{
		ID: audit.AuditID{
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			ID:             id.String(),
		},
		TreeID: int64(9080682816463212189),
	}

	uc := UseCase{
		AuditRepo: audit.NewMockRepository(gomock.NewController(t)),
	}

	uc.AuditRepo.(*audit.MockRepository).
		EXPECT().
		FindByID(gomock.Any(), audit.TreeCollection, mockedAudit.ID).
		Return(&mockedAudit, nil).
		Times(1)

	info, err := uc.GetAuditInfo(context.TODO(), organizationID, ledgerID, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, mockedAudit, *info)
}
