// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAliasByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)

	holderID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7().String()
	ledgerID := libCommons.GenerateUUIDv7().String()
	holderDocument := "90217469051"

	uc := &UseCase{
		HolderRepo: mockHolderRepo,
		AliasRepo:  mockAliasRepo,
	}

	testCases := []struct {
		name           string
		id             uuid.UUID
		holderID       uuid.UUID
		mockSetup      func()
		expectedErr    error
		expectedResult *mmodel.Alias
	}{
		{
			name:     "Success retrieving alias by ID",
			holderID: holderID,
			id:       id,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(&mmodel.Alias{
						ID:        &id,
						Document:  &holderDocument,
						LedgerID:  &ledgerID,
						HolderID:  &holderID,
						AccountID: &accountID,
					}, nil)
			},
			expectedErr: nil,
			expectedResult: &mmodel.Alias{
				ID:        &id,
				Document:  &holderDocument,
				LedgerID:  &ledgerID,
				HolderID:  &holderID,
				AccountID: &accountID,
			},
		},
		{
			name:     "Error when alias not found by ID",
			holderID: uuid.New(),
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
					Return(nil, cn.ErrAliasNotFound)
			},
			expectedErr:    cn.ErrAliasNotFound,
			expectedResult: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAliasByID(ctx, uuid.New().String(), testCase.holderID, testCase.id, false)

			if testCase.expectedErr != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedResult, result)
			}
		})
	}
}
