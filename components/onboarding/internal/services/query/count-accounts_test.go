// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
)

func TestCountAccounts(t *testing.T) {
	testCases := []struct {
		name           string
		setupMock      func(mockRepo *account.MockRepository)
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		expectedCount  int64
		expectedError  error
	}{
		{
			name: "Success - Count accounts",
			setupMock: func(mockRepo *account.MockRepository) {
				mockRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(5), nil)
			},
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			expectedCount:  5,
			expectedError:  nil,
		},
		{
			name: "Success - No accounts found",
			setupMock: func(mockRepo *account.MockRepository) {
				mockRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			expectedCount:  0,
			expectedError:  nil,
		},
		{
			name: "Error - Database error",
			setupMock: func(mockRepo *account.MockRepository) {
				mockRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), errDatabaseError)
			},
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			expectedCount:  0,
			expectedError:  errDatabaseError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := account.NewMockRepository(ctrl)
			tc.setupMock(mockRepo)

			uc := &UseCase{
				AccountRepo: mockRepo,
			}

			count, err := uc.CountAccounts(context.Background(), tc.organizationID, tc.ledgerID)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedCount, count)
			}
		})
	}
}
