package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetLedgerByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:   mockLedgerRepo,
		MetadataRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult *mmodel.Ledger
	}{
		{
			name:           "Success - Retrieve ledger with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				ledgerID := uuid.New()
				mockLedgerRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Test Ledger", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"key": "value"}}, nil)
			},
			expectErr: false,
			expectedResult: &mmodel.Ledger{
				ID:       "valid-uuid",
				Name:     "Test Ledger",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
			},
		},
		{
			name:           "Error - Ledger not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			mockSetup: func() {
				ledgerID := uuid.New()
				mockLedgerRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Ledger{ID: ledgerID.String(), Name: "Test Ledger", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("metadata retrieval error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetLedgerByID(ctx, tt.organizationID, tt.ledgerID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestGetLedgerByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetLedgerByID(ctx, uuid.Nil, uuid.New())
}

func TestGetLedgerByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetLedgerByID(ctx, uuid.New(), uuid.Nil)
}
