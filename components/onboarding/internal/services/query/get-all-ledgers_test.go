package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestGetAllLedgers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo:   mockLedgerRepo,
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()

	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	tests := []struct {
		name            string
		setupMocks      func()
		expectedErr     error
		expectedLedgers []*mmodel.Ledger
	}{
		{
			name: "success - ledgers retrieved with metadata",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, filter.ToOffsetPagination()).
					Return([]*mmodel.Ledger{
						{ID: "ledger1"},
						{ID: "ledger2"},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Ledger", filter).
					Return([]*mongodb.Metadata{
						{EntityID: "ledger1", Data: map[string]any{"key1": "value1"}},
						{EntityID: "ledger2", Data: map[string]any{"key2": "value2"}},
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			expectedLedgers: []*mmodel.Ledger{
				{ID: "ledger1", Metadata: map[string]any{"key1": "value1"}},
				{ID: "ledger2", Metadata: map[string]any{"key2": "value2"}},
			},
		},
		{
			name: "failure - ledgers not found",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, filter.ToOffsetPagination()).
					Return(nil, errors.New("No ledgers were found in the search. Please review the search criteria and try again.")).
					Times(1)
			},
			expectedErr:     errors.New("No ledgers were found in the search. Please review the search criteria and try again."),
			expectedLedgers: nil,
		},
		{
			name: "failure - repository error retrieving ledgers",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, filter.ToOffsetPagination()).
					Return(nil, errors.New("failed to retrieve ledgers")).
					Times(1)
			},
			expectedErr:     errors.New("failed to retrieve ledgers"),
			expectedLedgers: nil,
		},
		{
			name: "failure - metadata retrieval error",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, filter.ToOffsetPagination()).
					Return([]*mmodel.Ledger{
						{ID: "ledger1"},
						{ID: "ledger2"},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Ledger", filter).
					Return(nil, errors.New("failed to retrieve metadata")).
					Times(1)
			},
			expectedErr:     errors.New("No ledgers were found in the search. Please review the search criteria and try again."),
			expectedLedgers: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.GetAllLedgers(ctx, organizationID, filter)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, len(tt.expectedLedgers), len(result))
				for i, ledger := range result {
					assert.Equal(t, tt.expectedLedgers[i].ID, ledger.ID)
					assert.Equal(t, tt.expectedLedgers[i].Metadata, ledger.Metadata)
				}
			}
		})
	}
}
