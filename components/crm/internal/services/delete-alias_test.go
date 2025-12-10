package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	holderlink "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder-link"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteAliasByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAliasRepo := alias.NewMockRepository(ctrl)
	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		AliasRepo:      mockAliasRepo,
		HolderLinkRepo: mockHolderLinkRepo,
	}

	id := libCommons.GenerateUUIDv7()
	holderID := libCommons.GenerateUUIDv7()

	testCases := []struct {
		name          string
		holderID      uuid.UUID
		id            uuid.UUID
		mockSetup     func()
		expectedError error
	}{
		{
			name:     "Success deleting alias by ID",
			holderID: holderID,
			id:       id,
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasID(gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.HolderLink{
						{
							ID: &id,
						},
					}, nil)
				mockHolderLinkRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil)
				mockAliasRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:     "Success when no holder links found for alias",
			holderID: holderID,
			id:       id,
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasID(gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil, nil)
				mockAliasRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil)
			},
			expectedError: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			err := uc.DeleteAliasByID(ctx, libCommons.GenerateUUIDv7().String(), testCase.holderID, testCase.id, false)

			if testCase.expectedError != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
