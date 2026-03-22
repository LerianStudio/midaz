// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCountTransactionsByRoute(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	route := uuid.New().String()
	status := "APPROVED"
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	filter := transaction.CountFilter{
		Route:  route,
		Status: status,
		From:   from,
		To:     to,
	}

	tests := []struct {
		name        string
		setupMocks  func(mockTxRepo *transaction.MockRepository)
		expectedErr error
		expected    *CountResponse
	}{
		{
			name: "success returns count",
			setupMocks: func(mockTxRepo *transaction.MockRepository) {
				mockTxRepo.EXPECT().
					CountByRoute(gomock.Any(), organizationID, ledgerID, filter).
					Return(int64(773), nil)
			},
			expectedErr: nil,
			expected: &CountResponse{
				Period: Period{
					From: from,
					To:   to,
				},
				Route:      route,
				Status:     status,
				TotalCount: 773,
			},
		},
		{
			name: "zero count",
			setupMocks: func(mockTxRepo *transaction.MockRepository) {
				mockTxRepo.EXPECT().
					CountByRoute(gomock.Any(), organizationID, ledgerID, filter).
					Return(int64(0), nil)
			},
			expectedErr: nil,
			expected: &CountResponse{
				Period: Period{
					From: from,
					To:   to,
				},
				Route:      route,
				Status:     status,
				TotalCount: 0,
			},
		},
		{
			name: "repository error",
			setupMocks: func(mockTxRepo *transaction.MockRepository) {
				mockTxRepo.EXPECT().
					CountByRoute(gomock.Any(), organizationID, ledgerID, filter).
					Return(int64(0), errors.New("database connection error"))
			},
			expectedErr: errors.New("database connection error"),
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTxRepo := transaction.NewMockRepository(ctrl)
			tt.setupMocks(mockTxRepo)

			uc := &UseCase{
				TransactionRepo: mockTxRepo,
			}

			result, err := uc.CountTransactionsByRoute(context.Background(), organizationID, ledgerID, filter)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expected.TotalCount, result.TotalCount)
			assert.Equal(t, tt.expected.Route, result.Route)
			assert.Equal(t, tt.expected.Status, result.Status)
			assert.Equal(t, tt.expected.Period.From, result.Period.From)
			assert.Equal(t, tt.expected.Period.To, result.Period.To)
		})
	}
}
