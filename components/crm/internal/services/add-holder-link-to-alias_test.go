package services

import (
	"context"
	"errors"
	"testing"

	holderlink "github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/holder-link"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestAddHolderLinkToAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderLinkRepo: mockHolderLinkRepo,
	}

	ctx := context.Background()
	organizationID := "org-123"
	aliasID := uuid.New()
	holderID := uuid.New()
	linkType := string(mmodel.LinkTypePrimaryHolder)

	// Mock: no existing link (constraint check passes)
	mockHolderLinkRepo.EXPECT().
		FindByAliasIDAndLinkType(gomock.Any(), organizationID, aliasID, linkType, false).
		Return(nil, nil)

	// Mock: successful create
	mockHolderLinkRepo.EXPECT().
		Create(gomock.Any(), organizationID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, input *mmodel.HolderLink) (*mmodel.HolderLink, error) {
			return input, nil
		})

	result, err := uc.AddHolderLinkToAlias(ctx, organizationID, aliasID, holderID, linkType)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.ID)
	assert.Equal(t, &holderID, result.HolderID)
	assert.Equal(t, &aliasID, result.AliasID)
	assert.Equal(t, &linkType, result.LinkType)
	assert.NotNil(t, result.Metadata)
}

func TestAddHolderLinkToAlias_InvalidLinkType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderLinkRepo: mockHolderLinkRepo,
	}

	ctx := context.Background()
	organizationID := "org-123"
	aliasID := uuid.New()
	holderID := uuid.New()
	invalidLinkType := "INVALID_TYPE"

	result, err := uc.AddHolderLinkToAlias(ctx, organizationID, aliasID, holderID, invalidLinkType)

	assert.Nil(t, result)
	assert.Error(t, err)

	validationErr, ok := err.(pkg.ValidationError)
	assert.True(t, ok, "error should be ValidationError")
	assert.Equal(t, cn.ErrInvalidLinkType.Error(), validationErr.Code)
}

func TestAddHolderLinkToAlias_PrimaryHolderAlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderLinkRepo: mockHolderLinkRepo,
	}

	ctx := context.Background()
	organizationID := "org-123"
	aliasID := uuid.New()
	holderID := uuid.New()
	linkType := string(mmodel.LinkTypePrimaryHolder)

	existingLinkID := uuid.New()
	existingHolderID := uuid.New()

	// Mock: existing PRIMARY_HOLDER link found
	mockHolderLinkRepo.EXPECT().
		FindByAliasIDAndLinkType(gomock.Any(), organizationID, aliasID, linkType, false).
		Return(&mmodel.HolderLink{
			ID:       &existingLinkID,
			HolderID: &existingHolderID,
			AliasID:  &aliasID,
			LinkType: &linkType,
		}, nil)

	result, err := uc.AddHolderLinkToAlias(ctx, organizationID, aliasID, holderID, linkType)

	assert.Nil(t, result)
	assert.Error(t, err)

	conflictErr, ok := err.(pkg.EntityConflictError)
	assert.True(t, ok, "error should be EntityConflictError")
	assert.Equal(t, cn.ErrPrimaryHolderAlreadyExists.Error(), conflictErr.Code)
}

func TestAddHolderLinkToAlias_DuplicateHolderLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderLinkRepo: mockHolderLinkRepo,
	}

	ctx := context.Background()
	organizationID := "org-123"
	aliasID := uuid.New()
	holderID := uuid.New()
	linkType := string(mmodel.LinkTypeLegalRepresentative)

	existingLinkID := uuid.New()

	// Mock: existing link with same type found (non-PRIMARY_HOLDER)
	mockHolderLinkRepo.EXPECT().
		FindByAliasIDAndLinkType(gomock.Any(), organizationID, aliasID, linkType, false).
		Return(&mmodel.HolderLink{
			ID:       &existingLinkID,
			HolderID: &holderID,
			AliasID:  &aliasID,
			LinkType: &linkType,
		}, nil)

	result, err := uc.AddHolderLinkToAlias(ctx, organizationID, aliasID, holderID, linkType)

	assert.Nil(t, result)
	assert.Error(t, err)

	conflictErr, ok := err.(pkg.EntityConflictError)
	assert.True(t, ok, "error should be EntityConflictError")
	assert.Equal(t, cn.ErrDuplicateHolderLink.Error(), conflictErr.Code)
}

func TestAddHolderLinkToAlias_CreateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderLinkRepo: mockHolderLinkRepo,
	}

	ctx := context.Background()
	organizationID := "org-123"
	aliasID := uuid.New()
	holderID := uuid.New()
	linkType := string(mmodel.LinkTypePrimaryHolder)

	repoErr := errors.New("database connection failed")

	// Mock: no existing link (constraint check passes)
	mockHolderLinkRepo.EXPECT().
		FindByAliasIDAndLinkType(gomock.Any(), organizationID, aliasID, linkType, false).
		Return(nil, nil)

	// Mock: create fails
	mockHolderLinkRepo.EXPECT().
		Create(gomock.Any(), organizationID, gomock.Any()).
		Return(nil, repoErr)

	result, err := uc.AddHolderLinkToAlias(ctx, organizationID, aliasID, holderID, linkType)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Equal(t, repoErr, err)
}
