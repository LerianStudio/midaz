package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreateHolder(t *testing.T) {
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
		input          *mmodel.CreateHolderInput
		mockSetup      func()
		expectErr      bool
		expectedHolder *mmodel.Holder
	}{
		{
			name: "Success with required fields provided",
			input: &mmodel.CreateHolderInput{
				Name:     name,
				Document: document,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Name:     &name,
						Document: &document,
					}, nil)
			},
			expectErr: false,
			expectedHolder: &mmodel.Holder{
				ID:       &holderID,
				Name:     &name,
				Document: &document,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			result, err := uc.CreateHolder(ctx, "0194ffee-e14f-70f5-b400-04b7b7434131", testCase.input)

			if testCase.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedHolder.Name, result.Name)
				assert.Equal(t, testCase.expectedHolder.Document, result.Document)
			}
		})
	}
}
