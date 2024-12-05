package services

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/grpc/out"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func createMockedData() (audit.Audit, []mmodel.QueueData, map[string]string) {
	mockedQueueData := []mmodel.QueueData{
		{ID: pkg.GenerateUUIDv7(), Value: json.RawMessage("Log A")},
		{ID: pkg.GenerateUUIDv7(), Value: json.RawMessage("Log B")},
	}

	mockedLeaves := map[string]string{
		mockedQueueData[0].ID.String(): "18240592D5594FB370E73AC81DD5C8E5EC5AB07D5874F40154A8D54BDC390B3C",
		mockedQueueData[1].ID.String(): "4EF0C3FAA84E1D3001658CC9A4CA78463B042CA42909456F3DAEF4A94B77FEB7",
	}

	mockedAudit := audit.Audit{
		ID: audit.AuditID{
			OrganizationID: pkg.GenerateUUIDv7().String(),
			LedgerID:       pkg.GenerateUUIDv7().String(),
			ID:             pkg.GenerateUUIDv7().String(),
		},
		TreeID:    int64(9080682816463212189),
		Leaves:    mockedLeaves,
		CreatedAt: time.Now(),
	}

	return mockedAudit, mockedQueueData, mockedLeaves
}

func validateAudit(actual, expected *audit.Audit) error {
	if actual.TreeID != expected.TreeID ||
		actual.ID.OrganizationID != expected.ID.OrganizationID ||
		actual.ID.LedgerID != expected.ID.LedgerID ||
		actual.ID.ID != expected.ID.ID {
		return fmt.Errorf("audit ID fields did not match")
	}

	if len(actual.Leaves) != len(expected.Leaves) {
		return fmt.Errorf("leaves length did not match")
	}

	for k, v := range actual.Leaves {
		if v != expected.Leaves[k] {
			return fmt.Errorf("leaf mismatch for key: %s", k)
		}
	}

	return nil
}

func Test_CreateLog(t *testing.T) {
	mockedAudit, mockedQueueData, mockedLeaves := createMockedData()

	uc := UseCase{
		AuditRepo:    audit.NewMockRepository(gomock.NewController(t)),
		TrillianRepo: out.NewMockRepository(gomock.NewController(t)),
	}

	// Audit already exists for the organization and ledger
	uc.AuditRepo.(*audit.MockRepository).
		EXPECT().
		FindOne(gomock.Any(), audit.TreeCollection, mockedAudit.ID).
		Return(&mockedAudit, nil).
		Times(1)

	// Must not call because the tree already exists
	uc.TrillianRepo.(*out.MockRepository).
		EXPECT().
		CreateTree(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	for _, data := range mockedQueueData {
		uc.TrillianRepo.(*out.MockRepository).
			EXPECT().
			CreateLog(gomock.Any(), mockedAudit.TreeID, data.Value).
			Return(mockedLeaves[data.ID.String()], nil).
			Times(1)
	}

	uc.AuditRepo.(*audit.MockRepository).
		EXPECT().
		Create(gomock.Any(), audit.TreeCollection, gomock.AssignableToTypeOf(&audit.Audit{})).
		DoAndReturn(func(ctx context.Context, collection string, o *audit.Audit) error {
			return validateAudit(o, &mockedAudit)
		}).
		Times(1)

	err := uc.CreateLog(context.TODO(), uuid.MustParse(mockedAudit.ID.OrganizationID), uuid.MustParse(mockedAudit.ID.LedgerID), uuid.MustParse(mockedAudit.ID.ID), mockedQueueData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_CreateLog_NewTree(t *testing.T) {
	mockedAudit, mockedQueueData, mockedLeaves := createMockedData()

	uc := UseCase{
		AuditRepo:    audit.NewMockRepository(gomock.NewController(t)),
		TrillianRepo: out.NewMockRepository(gomock.NewController(t)),
	}

	// Audit does not exist yet
	uc.AuditRepo.(*audit.MockRepository).
		EXPECT().
		FindOne(gomock.Any(), audit.TreeCollection, mockedAudit.ID).
		Return(nil, mongo.ErrNoDocuments).
		Times(1)

	// Mock tree creation
	ledgerID := mockedAudit.ID.LedgerID
	treeName := ledgerID[len(ledgerID)-12:]
	uc.TrillianRepo.(*out.MockRepository).
		EXPECT().
		CreateTree(gomock.Any(), "Tree "+treeName, ledgerID).
		Return(mockedAudit.TreeID, nil).
		Times(1)

	for _, data := range mockedQueueData {
		uc.TrillianRepo.(*out.MockRepository).
			EXPECT().
			CreateLog(gomock.Any(), mockedAudit.TreeID, data.Value).
			Return(mockedLeaves[data.ID.String()], nil).
			Times(1)
	}

	uc.AuditRepo.(*audit.MockRepository).
		EXPECT().
		Create(gomock.Any(), audit.TreeCollection, gomock.AssignableToTypeOf(&audit.Audit{})).
		DoAndReturn(func(ctx context.Context, collection string, o *audit.Audit) error {
			return validateAudit(o, &mockedAudit)
		}).
		Times(1)

	err := uc.CreateLog(context.TODO(), uuid.MustParse(mockedAudit.ID.OrganizationID), uuid.MustParse(mockedAudit.ID.LedgerID), uuid.MustParse(mockedAudit.ID.ID), mockedQueueData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_CreateLog_ErrorFindAudit(t *testing.T) {
	mockedAudit, mockedQueueData, _ := createMockedData()

	uc := UseCase{
		AuditRepo:    audit.NewMockRepository(gomock.NewController(t)),
		TrillianRepo: out.NewMockRepository(gomock.NewController(t)),
	}

	uc.AuditRepo.(*audit.MockRepository).
		EXPECT().
		FindOne(gomock.Any(), audit.TreeCollection, mockedAudit.ID).
		Return(nil, mongo.ErrClientDisconnected).
		Times(1)

	uc.TrillianRepo.(*out.MockRepository).
		EXPECT().
		CreateTree(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc.TrillianRepo.(*out.MockRepository).
		EXPECT().
		CreateLog(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc.AuditRepo.(*audit.MockRepository).
		EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	err := uc.CreateLog(context.TODO(), uuid.MustParse(mockedAudit.ID.OrganizationID), uuid.MustParse(mockedAudit.ID.LedgerID), uuid.MustParse(mockedAudit.ID.ID), mockedQueueData)

	assert.NotNil(t, err)
}
