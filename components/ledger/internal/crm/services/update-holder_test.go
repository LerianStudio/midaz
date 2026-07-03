// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdateHolderByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := holder.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	name := "John Smith"
	document := "90217469051"

	uc := &UseCase{
		HolderRepo: mockRepo,
	}

	testCases := []struct {
		name           string
		input          *mmodel.UpdateHolderInput
		mockSetup      func()
		expectErr      bool
		expectedHolder *mmodel.Holder
	}{
		{
			name: "Success with single field provided",
			input: &mmodel.UpdateHolderInput{
				Name: &name,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
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
			name: "Error when repository fails to update holder",
			input: &mmodel.UpdateHolderInput{
				Name: &name,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectErr:      true,
			expectedHolder: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			fieldsToRemove := []string{"field1", "field2"}

			ctx := context.Background()
			result, err := uc.UpdateHolderByID(ctx, "0194ffee-e14f-70f5-b400-04b7b7434131", holderID, testCase.input, fieldsToRemove)

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

func TestUpdateHolderByID_EmitsHolderUpdated(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := holder.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	name := "John Smith"
	document := "90217469051"

	emitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		HolderRepo: mockRepo,
		Streaming:  emitter,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Holder{ID: &holderID, Name: &name, Document: &document}, nil)

	ctx := context.Background()
	result, err := uc.UpdateHolderByID(ctx, "0194ffee-e14f-70f5-b400-04b7b7434131", holderID, &mmodel.UpdateHolderInput{Name: &name}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	emitted := emitter.Events()
	require.Len(t, emitted, 1)
	assert.Equal(t, events.HolderUpdatedDefinition.Key(), emitted[0].DefinitionKey)
	assert.Equal(t, holderID.String(), emitted[0].Subject)
	pkgStreaming.AssertEventEmitted(t, emitter, "holder", "updated")
}

func TestUpdateHolderByID_NilEmitterSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := holder.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	name := "John Smith"
	document := "90217469051"

	uc := &UseCase{
		HolderRepo: mockRepo,
		Streaming:  nil,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Holder{ID: &holderID, Name: &name, Document: &document}, nil)

	ctx := context.Background()
	result, err := uc.UpdateHolderByID(ctx, "0194ffee-e14f-70f5-b400-04b7b7434131", holderID, &mmodel.UpdateHolderInput{Name: &name}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestUpdateHolderByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := holder.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())
	name := "John Smith"
	document := "90217469051"

	emitter := pkgStreaming.NewMockEmitter()
	emitter.SetError(errors.New("broker unavailable"))

	uc := &UseCase{
		HolderRepo: mockRepo,
		Streaming:  emitter,
	}

	mockRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Holder{ID: &holderID, Name: &name, Document: &document}, nil)

	ctx := context.Background()
	result, err := uc.UpdateHolderByID(ctx, "0194ffee-e14f-70f5-b400-04b7b7434131", holderID, &mmodel.UpdateHolderInput{Name: &name}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
}
