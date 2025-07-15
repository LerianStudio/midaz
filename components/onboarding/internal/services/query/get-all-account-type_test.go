package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/mock/gomock"
)

// TestGetAllAccountTypeSuccess tests getting all account types successfully with metadata
func TestGetAllAccountTypeSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID1 := libCommons.GenerateUUIDv7()
	accountTypeID2 := libCommons.GenerateUUIDv7()

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	expectedAccountTypes := []*mmodel.AccountType{
		{
			ID:             accountTypeID1,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Asset Account",
			Description:    "Asset account description",
			KeyValue:       "asset_account",
		},
		{
			ID:             accountTypeID2,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Liability Account",
			Description:    "Liability account description",
			KeyValue:       "liability_account",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			EntityID: accountTypeID1.String(),
			Data: mongodb.JSON{
				"category": "current",
				"priority": 1,
			},
		},
		{
			EntityID: accountTypeID2.String(),
			Data: mongodb.JSON{
				"category": "long-term",
				"priority": 2,
			},
		},
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedAccountTypes, expectedCursor, nil).
		Times(1)

	expectedMetadataFilter := filter
	expectedMetadataFilter.Metadata = &bson.M{}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), expectedMetadataFilter).
		Return(expectedMetadata, nil).
		Times(1)

	result, cur, err := uc.GetAllAccountType(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 2)

	// Verify metadata was attached correctly
	assert.Equal(t, expectedAccountTypes[0].ID, result[0].ID)
	assert.Equal(t, expectedAccountTypes[0].Name, result[0].Name)
	assert.Equal(t, map[string]any(expectedMetadata[0].Data), result[0].Metadata)

	assert.Equal(t, expectedAccountTypes[1].ID, result[1].ID)
	assert.Equal(t, expectedAccountTypes[1].Name, result[1].Name)
	assert.Equal(t, map[string]any(expectedMetadata[1].Data), result[1].Metadata)
}

// TestGetAllAccountTypeSuccessWithoutMetadata tests getting all account types successfully without metadata
func TestGetAllAccountTypeSuccessWithoutMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	expectedAccountTypes := []*mmodel.AccountType{
		{
			ID:             libCommons.GenerateUUIDv7(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Revenue Account",
			Description:    "Revenue account description",
			KeyValue:       "revenue_account",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedAccountTypes, expectedCursor, nil).
		Times(1)

	expectedMetadataFilter := filter
	expectedMetadataFilter.Metadata = &bson.M{}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), expectedMetadataFilter).
		Return([]*mongodb.Metadata{}, nil).
		Times(1)

	result, cur, err := uc.GetAllAccountType(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedAccountTypes, result)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 1)
	assert.Nil(t, result[0].Metadata)
}

// TestGetAllAccountTypeNotFound tests getting all account types when no results found
func TestGetAllAccountTypeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	expectedError := services.ErrDatabaseItemNotFound
	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrNoAccountTypesFound, reflect.TypeOf(mmodel.AccountType{}).Name())

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
	}

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(nil, libHTTP.CursorPagination{}, expectedError).
		Times(1)

	result, cur, err := uc.GetAllAccountType(context.Background(), organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Equal(t, expectedBusinessError, err)
	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cur)
}

// TestGetAllAccountTypeRepoError tests getting all account types with database error
func TestGetAllAccountTypeRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	expectedError := errors.New("database connection error")

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
	}

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(nil, libHTTP.CursorPagination{}, expectedError).
		Times(1)

	result, cur, err := uc.GetAllAccountType(context.Background(), organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cur)
}

// TestGetAllAccountTypeMetadataError tests getting all account types with metadata error
func TestGetAllAccountTypeMetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	expectedAccountTypes := []*mmodel.AccountType{
		{
			ID:             libCommons.GenerateUUIDv7(),
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Test Account",
			Description:    "Test account description",
			KeyValue:       "test_account",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	metadataError := errors.New("metadata service error")
	expectedBusinessError := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedAccountTypes, expectedCursor, nil).
		Times(1)

	expectedMetadataFilter := filter
	expectedMetadataFilter.Metadata = &bson.M{}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), expectedMetadataFilter).
		Return(nil, metadataError).
		Times(1)

	result, cur, err := uc.GetAllAccountType(context.Background(), organizationID, ledgerID, filter)

	assert.Error(t, err)
	assert.Equal(t, expectedBusinessError, err)
	assert.Nil(t, result)
	assert.Equal(t, libHTTP.CursorPagination{}, cur)
}

// TestGetAllAccountTypeEmpty tests getting all account types when empty results
func TestGetAllAccountTypeEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
	}

	expectedAccountTypes := []*mmodel.AccountType{}
	expectedCursor := libHTTP.CursorPagination{}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedAccountTypes, expectedCursor, nil).
		Times(1)

	expectedMetadataFilter := filter
	expectedMetadataFilter.Metadata = &bson.M{}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), expectedMetadataFilter).
		Return([]*mongodb.Metadata{}, nil).
		Times(1)

	result, cur, err := uc.GetAllAccountType(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedAccountTypes, result)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 0)
}

// TestGetAllAccountTypeWithDifferentPagination tests getting all account types with different pagination settings
func TestGetAllAccountTypeWithDifferentPagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID := libCommons.GenerateUUIDv7()

	filter := http.QueryHeader{
		Limit:     5,
		SortOrder: "desc",
		Cursor:    "test_cursor",
	}

	expectedAccountTypes := []*mmodel.AccountType{
		{
			ID:             accountTypeID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Expense Account",
			Description:    "Expense account description",
			KeyValue:       "expense_account",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			EntityID: accountTypeID.String(),
			Data: mongodb.JSON{
				"department": "finance",
			},
		},
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedAccountTypes, expectedCursor, nil).
		Times(1)

	expectedMetadataFilter := filter
	expectedMetadataFilter.Metadata = &bson.M{}

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), expectedMetadataFilter).
		Return(expectedMetadata, nil).
		Times(1)

	result, cur, err := uc.GetAllAccountType(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 1)
	assert.Equal(t, expectedAccountTypes[0].ID, result[0].ID)
	assert.Equal(t, map[string]any(expectedMetadata[0].Data), result[0].Metadata)
}

// TestGetAllAccountTypeWithMetadataFilter tests getting all account types with metadata filter
func TestGetAllAccountTypeWithMetadataFilter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID := libCommons.GenerateUUIDv7()

	metadataFilter := &bson.M{
		"category": "current",
	}

	filter := http.QueryHeader{
		Limit:     10,
		SortOrder: "asc",
		Metadata:  metadataFilter,
	}

	expectedAccountTypes := []*mmodel.AccountType{
		{
			ID:             accountTypeID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Current Asset",
			Description:    "Current asset description",
			KeyValue:       "current_asset",
		},
	}

	expectedCursor := libHTTP.CursorPagination{
		Next: "next_cursor",
		Prev: "prev_cursor",
	}

	expectedMetadata := []*mongodb.Metadata{
		{
			EntityID: accountTypeID.String(),
			Data: mongodb.JSON{
				"category": "current",
				"subtype":  "cash",
			},
		},
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		FindAll(gomock.Any(), organizationID, ledgerID, filter.ToCursorPagination()).
		Return(expectedAccountTypes, expectedCursor, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindList(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), filter).
		Return(expectedMetadata, nil).
		Times(1)

	result, cur, err := uc.GetAllAccountType(context.Background(), organizationID, ledgerID, filter)

	assert.NoError(t, err)
	assert.Equal(t, expectedCursor, cur)
	assert.Len(t, result, 1)
	assert.Equal(t, expectedAccountTypes[0].ID, result[0].ID)
	assert.Equal(t, map[string]any(expectedMetadata[0].Data), result[0].Metadata)
}
