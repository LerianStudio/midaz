package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// \1 performs an operation
func TestCreateSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := segment.NewMockRepository(ctrl)

	uc := &UseCase{
		SegmentRepo: mockRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		input          *mmodel.CreateSegmentInput
		mockSetup      func()
		expectErr      bool
		expectedProd   *mmodel.Segment
	}{
		{
			name:           "Success with all fields",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateSegmentInput{
				Name: "Test Segment",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Test Segment").
					Return(true, nil)
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{
						ID:             "123",
						OrganizationID: "org123",
						LedgerID:       "ledger123",
						Name:           "Test Segment",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Metadata:       nil,
					}, nil) // Produto criado com sucesso
			},
			expectErr: false,
			expectedProd: &mmodel.Segment{
				Name:   "Test Segment",
				Status: mmodel.Status{Code: "ACTIVE"},
			},
		},
		{
			name:           "Error when FindByName fails",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateSegmentInput{
				Name: "Failing Segment",
				Status: mmodel.Status{
					Code: "ACTIVE",
				},
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Failing Segment").
					Return(false, errors.New("repository error"))
			},
			expectErr:    true,
			expectedProd: nil,
		},
		{
			name:           "Success with default status",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			input: &mmodel.CreateSegmentInput{
				Name:     "Default Status Segment",
				Status:   mmodel.Status{}, // Empty status
				Metadata: nil,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByName(gomock.Any(), gomock.Any(), gomock.Any(), "Default Status Segment").
					Return(true, nil)
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(&mmodel.Segment{
						ID:             "124",
						OrganizationID: "org124",
						LedgerID:       "ledger124",
						Name:           "Default Status Segment",
						Status:         mmodel.Status{Code: "ACTIVE"},
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
						Metadata:       nil,
					}, nil)
			},
			expectErr: false,
			expectedProd: &mmodel.Segment{
				Name:   "Default Status Segment",
				Status: mmodel.Status{Code: "ACTIVE"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.CreateSegment(ctx, tt.organizationID, tt.ledgerID, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedProd.Name, result.Name)
				assert.Equal(t, tt.expectedProd.Status.Code, result.Status.Code)
			}
		})
	}
}
