// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteHolderByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := instrument.NewMockRepository(ctrl)

	holderID := uuid.Must(libCommons.GenerateUUIDv7())

	testCases := []struct {
		name         string
		holderID     uuid.UUID
		accountCount int64
		mockSetup    func()
		expectError  bool
	}{
		{
			name:         "Success deleting holder with no instruments and no accounts",
			holderID:     holderID,
			accountCount: 0,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
				mockHolderRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:         "Error when holder not found by ID",
			holderID:     holderID,
			accountCount: 0,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
				mockHolderRepo.EXPECT().
					Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(cn.ErrHolderNotFound)
			},
			expectError: true,
		},
		{
			name:         "Error when holder has linked instruments",
			holderID:     holderID,
			accountCount: 0,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(1), nil)
			},
			expectError: true,
		},
		{
			name:         "Error when holder owns active accounts",
			holderID:     holderID,
			accountCount: 1,
			mockSetup: func() {
				// Instrument guard passes (no instruments); the account-ownership
				// guard fires on the owned account and blocks the delete.
				mockAliasRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			uc := &UseCase{
				HolderRepo:     mockHolderRepo,
				InstrumentRepo: mockAliasRepo,
				LedgerAccounts: &stubLedgerAccountReader{accountCount: testCase.accountCount},
			}

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
