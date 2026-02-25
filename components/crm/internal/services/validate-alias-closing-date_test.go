// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestValidateAliasClosingDate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAliasRepo := alias.NewMockRepository(ctrl)

	organizationID := libCommons.GenerateUUIDv7().String()
	holderID := libCommons.GenerateUUIDv7()
	aliasID := libCommons.GenerateUUIDv7()
	createdAt := time.Now().Add(-24 * time.Hour)

	uc := &UseCase{
		AliasRepo: mockAliasRepo,
	}

	testCases := []struct {
		name          string
		holderID      uuid.UUID
		aliasID       uuid.UUID
		closingDate   *mmodel.Date
		mockSetup     func()
		expectError   bool
		expectedError error
	}{
		{
			name:        "Success when closing date is nil",
			holderID:    holderID,
			aliasID:     aliasID,
			closingDate: nil,
			mockSetup:   func() {},
			expectError: false,
		},
		{
			name:     "Error when closing date is before creation date",
			holderID: holderID,
			aliasID:  aliasID,
			closingDate: func() *mmodel.Date {
				return &mmodel.Date{Time: time.Now().Add(-48 * time.Hour)}
			}(),
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Find(gomock.Any(), organizationID, holderID, aliasID, false).
					Return(&mmodel.Alias{
						ID:        &aliasID,
						HolderID:  &holderID,
						CreatedAt: createdAt,
					}, nil)
			},
			expectError:   true,
			expectedError: cn.ErrAliasClosingDateBeforeCreation,
		},
		{
			name:     "Success when closing date is after creation date",
			holderID: holderID,
			aliasID:  aliasID,
			closingDate: func() *mmodel.Date {
				return &mmodel.Date{Time: time.Now().Add(24 * time.Hour)}
			}(),
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Find(gomock.Any(), organizationID, holderID, aliasID, false).
					Return(&mmodel.Alias{
						ID:        &aliasID,
						HolderID:  &holderID,
						CreatedAt: createdAt,
					}, nil)
			},
			expectError: false,
		},
		{
			name:     "Error when alias not found",
			holderID: holderID,
			aliasID:  aliasID,
			closingDate: func() *mmodel.Date {
				return &mmodel.Date{Time: time.Now()}
			}(),
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Find(gomock.Any(), organizationID, holderID, aliasID, false).
					Return(nil, cn.ErrAliasNotFound)
			},
			expectError:   true,
			expectedError: cn.ErrAliasNotFound,
		},
		{
			name:     "Error when repository returns generic error",
			holderID: holderID,
			aliasID:  aliasID,
			closingDate: func() *mmodel.Date {
				return &mmodel.Date{Time: time.Now()}
			}(),
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Find(gomock.Any(), organizationID, holderID, aliasID, false).
					Return(nil, errors.New("database error"))
			},
			expectError: true,
		},
		{
			name:     "Success when closing date equals creation date",
			holderID: holderID,
			aliasID:  aliasID,
			closingDate: func() *mmodel.Date {
				return &mmodel.Date{Time: createdAt}
			}(),
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					Find(gomock.Any(), organizationID, holderID, aliasID, false).
					Return(&mmodel.Alias{
						ID:        &aliasID,
						HolderID:  &holderID,
						CreatedAt: createdAt,
					}, nil)
			},
			expectError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			err := uc.validateAliasClosingDate(ctx, organizationID, testCase.holderID, testCase.aliasID, testCase.closingDate)

			if testCase.expectError {
				assert.Error(t, err)
				if testCase.expectedError != nil {
					assert.Contains(t, err.Error(), testCase.expectedError.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
