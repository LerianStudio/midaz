// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCountTransactionsByFilters(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		setupMock      func(mockRepo *transaction.MockRepository)
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		filter         transaction.CountFilter
		expectedCount  int64
		expectedError  bool
	}{
		{
			name: "success with filters",
			setupMock: func(mockRepo *transaction.MockRepository) {
				mockRepo.EXPECT().
					CountByFilters(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(42), nil)
			},
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter: transaction.CountFilter{
				Route:     "payment",
				Status:    "APPROVED",
				StartDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC),
			},
			expectedCount: 42,
			expectedError: false,
		},
		{
			name: "success with zero count",
			setupMock: func(mockRepo *transaction.MockRepository) {
				mockRepo.EXPECT().
					CountByFilters(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter:         transaction.CountFilter{},
			expectedCount:  0,
			expectedError:  false,
		},
		{
			name: "repository error",
			setupMock: func(mockRepo *transaction.MockRepository) {
				mockRepo.EXPECT().
					CountByFilters(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), errors.New("database error"))
			},
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			filter:         transaction.CountFilter{},
			expectedCount:  0,
			expectedError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := transaction.NewMockRepository(ctrl)
			tc.setupMock(mockRepo)

			uc := &UseCase{
				TransactionRepo: mockRepo,
			}

			count, err := uc.CountTransactionsByFilters(context.Background(), tc.organizationID, tc.ledgerID, tc.filter)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedCount, count)
			}
		})
	}
}
