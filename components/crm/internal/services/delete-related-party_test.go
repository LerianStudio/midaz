// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/alias"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteRelatedPartyByID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockAliasRepo := alias.NewMockRepository(ctrl)

	uc := &UseCase{
		AliasRepo: mockAliasRepo,
	}

	organizationID := libCommons.GenerateUUIDv7().String()
	holderID := libCommons.GenerateUUIDv7()
	aliasID := libCommons.GenerateUUIDv7()
	relatedPartyID := libCommons.GenerateUUIDv7()

	errRepoGeneric := errors.New("connection refused")

	testCases := []struct {
		name           string
		organizationID string
		holderID       uuid.UUID
		aliasID        uuid.UUID
		relatedPartyID uuid.UUID
		mockSetup      func()
		expectedError  error
		errContains    string
	}{
		{
			name:           "success_deleting_related_party",
			organizationID: organizationID,
			holderID:       holderID,
			aliasID:        aliasID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, aliasID, relatedPartyID).
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "error_alias_not_found",
			organizationID: organizationID,
			holderID:       holderID,
			aliasID:        aliasID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, aliasID, relatedPartyID).
					Return(cn.ErrAliasNotFound)
			},
			expectedError: cn.ErrAliasNotFound,
			errContains:   "CRM-0008",
		},
		{
			name:           "error_related_party_not_found",
			organizationID: organizationID,
			holderID:       holderID,
			aliasID:        aliasID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, aliasID, relatedPartyID).
					Return(cn.ErrRelatedPartyNotFound)
			},
			expectedError: cn.ErrRelatedPartyNotFound,
			errContains:   "CRM-0024",
		},
		{
			name:           "error_repository_timeout",
			organizationID: organizationID,
			holderID:       holderID,
			aliasID:        aliasID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, aliasID, relatedPartyID).
					Return(context.DeadlineExceeded)
			},
			expectedError: context.DeadlineExceeded,
			errContains:   "deadline exceeded",
		},
		{
			name:           "error_repository_generic",
			organizationID: organizationID,
			holderID:       holderID,
			aliasID:        aliasID,
			relatedPartyID: relatedPartyID,
			mockSetup: func() {
				mockAliasRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), organizationID, holderID, aliasID, relatedPartyID).
					Return(errRepoGeneric)
			},
			expectedError: errRepoGeneric,
			errContains:   "connection refused",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()

			ctx := context.Background()
			err := uc.DeleteRelatedPartyByID(ctx, tc.organizationID, tc.holderID, tc.aliasID, tc.relatedPartyID)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tc.expectedError)
				assert.Contains(t, err.Error(), tc.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
