package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreateLedger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()

	tests := []struct {
		name        string
		input       *mmodel.CreateLedgerInput
		mockSetup   func()
		expectedErr error
		expectedRes *mmodel.Ledger
	}{

		{
			name: "success - ledger created",
			input: &mmodel.CreateLedgerInput{
				Name: "Finance Ledger",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: libPointers.String("Ledger for financial transactions"),
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					FindByName(gomock.Any(), organizationID, "Finance Ledger").
					Return(true, nil).
					Times(1)

				mockLedgerRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Ledger{
						ID:             uuid.New().String(),
						OrganizationID: organizationID.String(),
						Name:           "Finance Ledger",
						Status: mmodel.Status{
							Code:        "ACTIVE",
							Description: libPointers.String("Ledger for financial transactions"),
						},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			expectedRes: &mmodel.Ledger{
				Name: "Finance Ledger",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
			},
		},
		{
			name: "error - failed to find ledger by name",
			input: &mmodel.CreateLedgerInput{
				Name: "Finance Ledger",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					FindByName(gomock.Any(), organizationID, "Finance Ledger").
					Return(false, errors.New("database error")).
					Times(1)
			},
			expectedErr: errors.New("database error"),
			expectedRes: nil,
		},
		{
			name: "error - failed to create ledger",
			input: &mmodel.CreateLedgerInput{
				Name: "Finance Ledger",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					FindByName(gomock.Any(), organizationID, "Finance Ledger").
					Return(false, nil).
					Times(1)

				mockLedgerRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("failed to insert ledger")).
					Times(1)
			},
			expectedErr: errors.New("failed to insert ledger"),
			expectedRes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CreateLedger(ctx, organizationID, tt.input)
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedRes.Name, result.Name)
				assert.Equal(t, tt.expectedRes.Status.Code, result.Status.Code)
			}
		})
	}
}
