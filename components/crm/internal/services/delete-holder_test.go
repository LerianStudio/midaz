package services

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"plugin-crm/v2/internal/adapters/mongodb/alias"
	"plugin-crm/v2/internal/adapters/mongodb/holder"
	cn "plugin-crm/v2/pkg/constant"
	"testing"
)

func TestDeleteHolderByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAccountRepo := alias.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderRepo: mockHolderRepo,
		AliasRepo:  mockAccountRepo,
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
				mockAccountRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
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
				mockAccountRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
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
				mockAccountRepo.EXPECT().
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
