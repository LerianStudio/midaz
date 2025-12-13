package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	holderlink "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder-link"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteHolderByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)
	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		AliasRepo:      mockAliasRepo,
		HolderLinkRepo: mockHolderLinkRepo,
	}

	holderID := libCommons.GenerateUUIDv7()

	testCases := []struct {
		name        string
		holderID    uuid.UUID
		mockSetup   func()
		expectError bool
	}{
		{
			name:     "Success deleting holder by ID",
			holderID: holderID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
				mockHolderLinkRepo.EXPECT().
					FindByHolderID(gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.HolderLink{
						{
							ID: &holderID,
						},
					}, nil)
				mockHolderLinkRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil)
				mockHolderRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:     "Error when holder not found by ID",
			holderID: holderID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
				mockHolderLinkRepo.EXPECT().
					FindByHolderID(gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return([]*mmodel.HolderLink{
						{
							ID: &holderID,
						},
					}, nil)
				mockHolderLinkRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil)
				mockHolderRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(cn.ErrHolderNotFound)
			},
			expectError: true,
		},
		{
			name:     "Error when holder has linked accounts",
			holderID: holderID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(1), nil)
			},
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			err := uc.DeleteHolderByID(ctx, uuid.New().String(), holderID, false)

			if testCase.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
