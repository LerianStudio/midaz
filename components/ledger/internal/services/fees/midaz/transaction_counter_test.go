// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package midaz

import (
	"context"
	"errors"
	"testing"
	"time"

	pkg "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// newTestTransactionCounter creates a midazTransactionCounter with a mock MidazResolver for testing.
func newTestTransactionCounter(t *testing.T) (TransactionCounter, *pkg.MockMidazResolver) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockResolver := pkg.NewMockMidazResolver(ctrl)

	counter, err := NewTransactionCounter(mockResolver)
	assert.NoError(t, err)
	assert.NotNil(t, counter)

	return counter, mockResolver
}

func TestCountByRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		params        CountParams
		mockReturn    int64
		mockErr       error
		expectedCount int64
		expectErr     bool
	}{
		{
			name: "success - returns transaction count",
			params: CountParams{
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				Route:          uuid.New().String(),
				Status:         "APPROVED",
				StartDate:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:        time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC),
			},
			mockReturn:    773,
			mockErr:       nil,
			expectedCount: 773,
			expectErr:     false,
		},
		{
			name: "error - resolver returns error",
			params: CountParams{
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				Route:          uuid.New().String(),
				Status:         "APPROVED",
				StartDate:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:        time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC),
			},
			mockReturn:    0,
			mockErr:       errors.New("count failed"),
			expectedCount: 0,
			expectErr:     true,
		},
		{
			name: "success - zero count",
			params: CountParams{
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				Route:          uuid.New().String(),
				Status:         "APPROVED",
				StartDate:      time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				EndDate:        time.Date(2026, 3, 31, 23, 59, 59, 0, time.UTC),
			},
			mockReturn:    0,
			mockErr:       nil,
			expectedCount: 0,
			expectErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			counter, mockResolver := newTestTransactionCounter(t)

			mockResolver.EXPECT().
				CountTransactionsByRoute(
					gomock.Any(),
					tt.params.OrganizationID,
					tt.params.LedgerID,
					tt.params.Route,
					tt.params.Status,
					tt.params.StartDate,
					tt.params.EndDate,
				).
				Return(tt.mockReturn, tt.mockErr).
				Times(1)

			count, err := counter.CountByRoute(context.Background(), tt.params)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

func TestNewTransactionCounter_NilResolver(t *testing.T) {
	t.Parallel()

	counter, err := NewTransactionCounter(nil)

	assert.Nil(t, counter)
	assert.Error(t, err)
	assert.Equal(t, ErrNilResolverCounter, err)
}
