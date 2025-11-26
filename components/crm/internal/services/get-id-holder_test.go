package services

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"plugin-crm/v2/internal/adapters/mongodb/holder"
	cn "plugin-crm/v2/pkg/constant"
	"plugin-crm/v2/pkg/model"
	"testing"
)

func TestGetHolderByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := holder.NewMockRepository(ctrl)

	holderID := libCommons.GenerateUUIDv7()
	name := "John Smith"
	document := "90217469051"

	uc := &UseCase{
		HolderRepo: mockRepo,
	}

	testCases := []struct {
		name           string
		holderID       uuid.UUID
		mockSetup      func()
		expectError    bool
		expectedResult *model.Holder
	}{
		{
			name:     "Success retrieving holder by ID",
			holderID: holderID,
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&model.Holder{
						ID:       &holderID,
						Name:     &name,
						Document: &document,
					}, nil)
			},
			expectError: false,
			expectedResult: &model.Holder{
				ID:       &holderID,
				Name:     &name,
				Document: &document,
			},
		},
		{
			name:     "Error when holder not found by ID",
			holderID: uuid.New(),
			mockSetup: func() {
				mockRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, cn.ErrHolderNotFound)
			},
			expectError:    true,
			expectedResult: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			result, err := uc.GetHolderByID(ctx, uuid.New().String(), holderID, false)

			if testCase.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedResult, result)
			}
		})
	}
}
