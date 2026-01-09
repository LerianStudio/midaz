package services

import (
	"context"
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

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
