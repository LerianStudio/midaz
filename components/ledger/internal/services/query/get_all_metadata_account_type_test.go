// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAllMetadataAccountType_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	filter := http.QueryHeader{}
	validUUID := uuid.New()

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
		}, nil)

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return([]*mmodel.AccountType{
			{ID: validUUID, Name: "Test Account Type", Description: "Test Description"},
		}, libHTTP.CursorPagination{}, nil)

	ctx := context.Background()
	result, pagination, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, pagination)
	assert.Len(t, result, 1)
	assert.Equal(t, "Test Account Type", result[0].Name)
	assert.Equal(t, map[string]any{"key": "value"}, result[0].Metadata)
}

func TestGetAllMetadataAccountType_MetadataRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	filter := http.QueryHeader{}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return(nil, errors.New("metadata repo error"))

	ctx := context.Background()
	result, _, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetAllMetadataAccountType_AccountTypeRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	filter := http.QueryHeader{}
	validUUID := uuid.New()

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
		}, nil)

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, errors.New("account type repo error"))

	ctx := context.Background()
	result, _, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetAllMetadataAccountType_DatabaseItemNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	filter := http.QueryHeader{}
	validUUID := uuid.New()

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: validUUID.String(), Data: map[string]any{"key": "value"}},
		}, nil)

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, libHTTP.CursorPagination{}, services.ErrDatabaseItemNotFound)

	ctx := context.Background()
	result, _, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGetAllMetadataAccountType_MultipleAccountTypes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	filter := http.QueryHeader{}
	validUUID1 := uuid.New()
	validUUID2 := uuid.New()

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: validUUID1.String(), Data: map[string]any{"key1": "value1"}},
			{EntityID: validUUID2.String(), Data: map[string]any{"key2": "value2"}},
		}, nil)

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return([]*mmodel.AccountType{
			{ID: validUUID1, Name: "Test Account Type 1", Description: "Test Description 1"},
			{ID: validUUID2, Name: "Test Account Type 2", Description: "Test Description 2"},
		}, libHTTP.CursorPagination{}, nil)

	ctx := context.Background()
	result, pagination, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, pagination)
	assert.Len(t, result, 2)
	assert.Equal(t, "Test Account Type 1", result[0].Name)
	assert.Equal(t, map[string]any{"key1": "value1"}, result[0].Metadata)
	assert.Equal(t, "Test Account Type 2", result[1].Name)
	assert.Equal(t, map[string]any{"key2": "value2"}, result[1].Metadata)
}

func TestGetAllMetadataAccountType_PartialMetadataMatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	filter := http.QueryHeader{}
	validUUID1 := uuid.New()
	validUUID2 := uuid.New()

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: validUUID1.String(), Data: map[string]any{"key1": "value1"}},
		}, nil)

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return([]*mmodel.AccountType{
			{ID: validUUID1, Name: "Test Account Type 1", Description: "Test Description 1"},
			{ID: validUUID2, Name: "Test Account Type 2", Description: "Test Description 2"},
		}, libHTTP.CursorPagination{}, nil)

	ctx := context.Background()
	result, pagination, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, pagination)
	assert.Len(t, result, 2)
	assert.Equal(t, "Test Account Type 1", result[0].Name)
	assert.Equal(t, map[string]any{"key1": "value1"}, result[0].Metadata)
	assert.Equal(t, "Test Account Type 2", result[1].Name)
	assert.Nil(t, result[1].Metadata)
}

func TestGetAllMetadataAccountType_MetadataWithStatusFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	validUUID := uuid.New()
	filter := http.QueryHeader{
		UseMetadata: true,
		Status:      func() *string { s := "ACTIVE"; return &s }(),
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: validUUID.String(), Data: map[string]any{"category": "savings"}},
		}, nil)

	// entityIDs AND status filter are both passed to FindAll
	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return([]*mmodel.AccountType{
			{ID: validUUID, Name: "Savings Account Type", Description: "For savings accounts"},
		}, libHTTP.CursorPagination{}, nil)

	ctx := context.Background()
	result, pagination, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, pagination)
	assert.Len(t, result, 1)
	assert.Equal(t, "Savings Account Type", result[0].Name)
	assert.Equal(t, "savings", result[0].Metadata["category"])
}

func TestGetAllMetadataAccountType_MetadataWithNameFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	validUUID := uuid.New()
	filter := http.QueryHeader{
		UseMetadata: true,
		Name:        func() *string { s := "Checking"; return &s }(),
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: validUUID.String(), Data: map[string]any{"access": "standard"}},
		}, nil)

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return([]*mmodel.AccountType{
			{ID: validUUID, Name: "Checking Account Type", Description: "For checking accounts"},
		}, libHTTP.CursorPagination{}, nil)

	ctx := context.Background()
	result, pagination, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, pagination)
	assert.Len(t, result, 1)
	assert.Equal(t, "Checking Account Type", result[0].Name)
	assert.Equal(t, "standard", result[0].Metadata["access"])
}

func TestGetAllMetadataAccountType_MetadataWithMultipleFilters(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	validUUID := uuid.New()
	filter := http.QueryHeader{
		UseMetadata: true,
		Status:      func() *string { s := "ACTIVE"; return &s }(),
		Name:        func() *string { s := "Investment"; return &s }(),
	}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), "AccountType", gomock.Any()).
		Return([]*mongodb.Metadata{
			{EntityID: validUUID.String(), Data: map[string]any{"risk_level": "high"}},
		}, nil)

	// All filters combined: entityIDs + status + name
	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return([]*mmodel.AccountType{
			{ID: validUUID, Name: "Investment Account Type", Description: "For investment accounts"},
		}, libHTTP.CursorPagination{}, nil)

	ctx := context.Background()
	result, pagination, err := uc.GetAllMetadataAccountType(ctx, organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, pagination)
	assert.Len(t, result, 1)
	assert.Equal(t, "Investment Account Type", result[0].Name)
	assert.Equal(t, "high", result[0].Metadata["risk_level"])
}
