package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
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
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
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
		ListByIDs(gomock.Any(), organizationID, ledgerID, gomock.Eq([]uuid.UUID{validUUID})).
		Return([]*mmodel.AccountType{
			{ID: validUUID, Name: "Test Account Type", Description: "Test Description"},
		}, nil)

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
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
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
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
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
		ListByIDs(gomock.Any(), organizationID, ledgerID, gomock.Eq([]uuid.UUID{validUUID})).
		Return(nil, errors.New("account type repo error"))

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
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
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
		ListByIDs(gomock.Any(), organizationID, ledgerID, gomock.Eq([]uuid.UUID{validUUID})).
		Return(nil, services.ErrDatabaseItemNotFound)

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
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
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
		ListByIDs(gomock.Any(), organizationID, ledgerID, gomock.Eq([]uuid.UUID{validUUID1, validUUID2})).
		Return([]*mmodel.AccountType{
			{ID: validUUID1, Name: "Test Account Type 1", Description: "Test Description 1"},
			{ID: validUUID2, Name: "Test Account Type 2", Description: "Test Description 2"},
		}, nil)

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
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
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
		ListByIDs(gomock.Any(), organizationID, ledgerID, gomock.Eq([]uuid.UUID{validUUID1})).
		Return([]*mmodel.AccountType{
			{ID: validUUID1, Name: "Test Account Type 1", Description: "Test Description 1"},
			{ID: validUUID2, Name: "Test Account Type 2", Description: "Test Description 2"},
		}, nil)

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
