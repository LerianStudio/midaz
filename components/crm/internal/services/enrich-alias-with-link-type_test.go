package services

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	holderlink "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder-link"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestEnrichAliasWithLinkType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	organizationID := libCommons.GenerateUUIDv7().String()
	aliasID := libCommons.GenerateUUIDv7()
	holderLinkID := libCommons.GenerateUUIDv7()
	holderLinkID2 := libCommons.GenerateUUIDv7()
	holderID := libCommons.GenerateUUIDv7()
	holderID2 := libCommons.GenerateUUIDv7()
	linkTypePrimaryHolder := string(mmodel.LinkTypePrimaryHolder)
	linkTypeLegalRepresentative := string(mmodel.LinkTypeLegalRepresentative)
	now := time.Now()

	uc := &UseCase{
		HolderLinkRepo: mockHolderLinkRepo,
	}

	testCases := []struct {
		name                string
		alias               *mmodel.Alias
		mockSetup           func()
		expectedErr         error
		expectedHolderLinks []*mmodel.HolderLink
	}{
		{
			name: "Success enriching alias with single holder link",
			alias: &mmodel.Alias{
				ID: &aliasID,
			},
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasID(gomock.Any(), organizationID, aliasID, false).
					Return([]*mmodel.HolderLink{
						{
							ID:        &holderLinkID,
							HolderID:  &holderID,
							AliasID:   &aliasID,
							LinkType:  &linkTypePrimaryHolder,
							CreatedAt: now,
							UpdatedAt: now,
						},
					}, nil)
			},
			expectedErr: nil,
			expectedHolderLinks: []*mmodel.HolderLink{
				{
					ID:        &holderLinkID,
					LinkType:  &linkTypePrimaryHolder,
					CreatedAt: now,
					UpdatedAt: now,
				},
			},
		},
		{
			name: "Success enriching alias with multiple holder links",
			alias: &mmodel.Alias{
				ID: &aliasID,
			},
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasID(gomock.Any(), organizationID, aliasID, false).
					Return([]*mmodel.HolderLink{
						{
							ID:        &holderLinkID,
							HolderID:  &holderID,
							AliasID:   &aliasID,
							LinkType:  &linkTypePrimaryHolder,
							CreatedAt: now,
							UpdatedAt: now,
						},
						{
							ID:        &holderLinkID2,
							HolderID:  &holderID2,
							AliasID:   &aliasID,
							LinkType:  &linkTypeLegalRepresentative,
							CreatedAt: now,
							UpdatedAt: now,
						},
					}, nil)
			},
			expectedErr: nil,
			expectedHolderLinks: []*mmodel.HolderLink{
				{
					ID:        &holderLinkID,
					LinkType:  &linkTypePrimaryHolder,
					CreatedAt: now,
					UpdatedAt: now,
				},
				{
					ID:        &holderLinkID2,
					LinkType:  &linkTypeLegalRepresentative,
					CreatedAt: now,
					UpdatedAt: now,
				},
			},
		},
		{
			name: "Success when alias ID is nil",
			alias: &mmodel.Alias{
				ID: nil,
			},
			mockSetup:           func() {},
			expectedErr:         nil,
			expectedHolderLinks: nil,
		},
		{
			name: "Success when no holder links found",
			alias: &mmodel.Alias{
				ID: &aliasID,
			},
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasID(gomock.Any(), organizationID, aliasID, false).
					Return([]*mmodel.HolderLink{}, nil)
			},
			expectedErr:         nil,
			expectedHolderLinks: nil,
		},
		{
			name: "Success when repository returns error",
			alias: &mmodel.Alias{
				ID: &aliasID,
			},
			mockSetup: func() {
				mockHolderLinkRepo.EXPECT().
					FindByAliasID(gomock.Any(), organizationID, aliasID, false).
					Return(nil, errors.New("database error"))
			},
			expectedErr:         nil,
			expectedHolderLinks: nil,
		},
		{
			name: "Success enriching alias with holder link that has deleted_at",
			alias: &mmodel.Alias{
				ID: &aliasID,
			},
			mockSetup: func() {
				deletedAt := time.Now()
				mockHolderLinkRepo.EXPECT().
					FindByAliasID(gomock.Any(), organizationID, aliasID, false).
					Return([]*mmodel.HolderLink{
						{
							ID:        &holderLinkID,
							HolderID:  &holderID,
							AliasID:   &aliasID,
							LinkType:  &linkTypePrimaryHolder,
							CreatedAt: now,
							UpdatedAt: now,
							DeletedAt: &deletedAt,
						},
					}, nil)
			},
			expectedErr: nil,
			expectedHolderLinks: []*mmodel.HolderLink{
				{
					ID:        &holderLinkID,
					LinkType:  &linkTypePrimaryHolder,
					CreatedAt: now,
					UpdatedAt: now,
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.mockSetup()

			ctx := context.Background()
			err := uc.enrichAliasWithLinkType(ctx, organizationID, testCase.alias)

			if testCase.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, testCase.expectedErr, err)
			} else {
				assert.NoError(t, err)
			}

			if testCase.expectedHolderLinks != nil {
				assert.NotNil(t, testCase.alias.HolderLinks)
				assert.Len(t, testCase.alias.HolderLinks, len(testCase.expectedHolderLinks))

				for i, expectedLink := range testCase.expectedHolderLinks {
					assert.Equal(t, expectedLink.ID, testCase.alias.HolderLinks[i].ID)
					assert.Equal(t, expectedLink.LinkType, testCase.alias.HolderLinks[i].LinkType)
					assert.Equal(t, expectedLink.CreatedAt, testCase.alias.HolderLinks[i].CreatedAt)
					assert.Equal(t, expectedLink.UpdatedAt, testCase.alias.HolderLinks[i].UpdatedAt)
				}
			} else {
				assert.Nil(t, testCase.alias.HolderLinks)
			}
		})
	}
}
