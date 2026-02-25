// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteAliasByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAliasRepo := alias.NewMockRepository(ctrl)

	uc := &UseCase{
		AliasRepo: mockAliasRepo,
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
				mockAliasRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:     "Error when repository fails to delete alias",
			holderID: holderID,
			id:       id,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(errors.New("database error"))
			},
			expectedError: errors.New("database error"),
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
