package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCountAccounts(t *testing.T) {
	testCases := []struct {
		name              string
		setupMock         func(mockRepo *account.MockRepository)
		organizationID    uuid.UUID
		ledgerID          uuid.UUID
		expectedCount     int64
		expectInternalErr bool
	}{
		{
			name: "Success - Count accounts",
			setupMock: func(mockRepo *account.MockRepository) {
				mockRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(5), nil)
			},
			organizationID:    uuid.New(),
			ledgerID:          uuid.New(),
			expectedCount:     5,
			expectInternalErr: false,
		},
		{
			name: "Success - No accounts found",
			setupMock: func(mockRepo *account.MockRepository) {
				mockRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), nil)
			},
			organizationID:    uuid.New(),
			ledgerID:          uuid.New(),
			expectedCount:     0,
			expectInternalErr: false,
		},
		{
			name: "Error - Database error",
			setupMock: func(mockRepo *account.MockRepository) {
				mockRepo.EXPECT().
					Count(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(int64(0), errors.New("database error"))
			},
			organizationID:    uuid.New(),
			ledgerID:          uuid.New(),
			expectedCount:     0,
			expectInternalErr: true,
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

			if tc.expectInternalErr {
				assert.Error(t, err)
				var internalErr pkg.InternalServerError
				assert.True(t, errors.As(err, &internalErr), "expected InternalServerError type")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedCount, count)
			}
		})
	}
}
