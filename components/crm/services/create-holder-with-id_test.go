// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

func TestCreateHolderWithID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := holder.NewMockRepository(ctrl)

	holderID := uuid.New()
	name := "John Smith"
	document := "90217469051"
	organizationID := "0194ffee-e14f-70f5-b400-04b7b7434131"

	duplicateKeyErr := mongo.WriteException{
		WriteErrors: mongo.WriteErrors{
			{Code: 11000, Message: "E11000 duplicate key error collection: holders index: _id_"},
		},
	}

	uc := &UseCase{
		HolderRepo: mockRepo,
	}

	testCases := []struct {
		name           string
		mockSetup      func()
		expectErr      bool
		expectedHolder *mmodel.Holder
	}{
		{
			name: "Success persists holder with caller-supplied id",
			mockSetup: func() {
				mockRepo.EXPECT().
					Create(gomock.Any(), organizationID, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, h *mmodel.Holder) (*mmodel.Holder, error) {
						// The caller-supplied id must be used verbatim, not a fresh v7.
						assert.Equal(t, holderID, *h.ID)
						return h, nil
					})
			},
			expectErr: false,
			expectedHolder: &mmodel.Holder{
				ID:       &holderID,
				Name:     &name,
				Document: &document,
			},
		},
		{
			name: "Idempotent success re-fetches existing holder on duplicate _id",
			mockSetup: func() {
				mockRepo.EXPECT().
					Create(gomock.Any(), organizationID, gomock.Any()).
					Return(nil, duplicateKeyErr)
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, holderID, false).
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
		{
			name: "Error propagated on duplicate _id when re-fetch fails",
			mockSetup: func() {
				mockRepo.EXPECT().
					Create(gomock.Any(), organizationID, gomock.Any()).
					Return(nil, duplicateKeyErr)
				mockRepo.EXPECT().
					Find(gomock.Any(), organizationID, holderID, false).
					Return(nil, errors.New("find failed"))
			},
			expectErr:      true,
			expectedHolder: nil,
		},
		{
			name: "Error propagated on non-duplicate repository failure",
			mockSetup: func() {
				mockRepo.EXPECT().
					Create(gomock.Any(), organizationID, gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectErr:      true,
			expectedHolder: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			input := &mmodel.CreateHolderInput{
				Name:     name,
				Document: document,
			}

			result, err := uc.CreateHolderWithID(ctx, organizationID, holderID, input)

			if testCase.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedHolder.ID, result.ID)
				assert.Equal(t, testCase.expectedHolder.Name, result.Name)
				assert.Equal(t, testCase.expectedHolder.Document, result.Document)
			}
		})
	}
}
