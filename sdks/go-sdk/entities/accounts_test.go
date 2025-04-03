package entities

import (
	"context"
	"testing"

	"fmt"
	"github.com/LerianStudio/midaz/sdks/go-sdk/entities/mocks"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func TestListAccounts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock accounts service
	mockService := mocks.NewMockAccountsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"

	// Create test account list response
	accountsList := &models.ListResponse[models.Account]{
		Items: []models.Account{
			{
				ID:             "acc-123",
				Name:           "Test Account 1",
				AssetCode:      "USD",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Type:           "LIABILITY",
				Status: models.Status{
					Code: "ACTIVE",
				},
			},
			{
				ID:             "acc-456",
				Name:           "Test Account 2",
				AssetCode:      "EUR",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Type:           "ASSET",
				Status: models.Status{
					Code: "ACTIVE",
				},
			},
		},
		Pagination: models.Pagination{
			Total:  2,
			Limit:  10,
			Offset: 0,
		},
	}

	// Setup expectations
	mockService.EXPECT().
		ListAccounts(gomock.Any(), orgID, ledgerID, gomock.Any()).
		Return(accountsList, nil)

	// Test with default options
	result, err := mockService.ListAccounts(ctx, orgID, ledgerID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "acc-123", result.Items[0].ID)
	assert.Equal(t, "Test Account 1", result.Items[0].Name)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, "LIABILITY", result.Items[0].Type)
	assert.Equal(t, "ACTIVE", result.Items[0].Status.Code)

	// Test validation for empty orgID
	mockService.EXPECT().
		ListAccounts(gomock.Any(), "", ledgerID, gomock.Any()).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.ListAccounts(ctx, "", ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test validation for empty ledgerID
	mockService.EXPECT().
		ListAccounts(gomock.Any(), orgID, "", gomock.Any()).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.ListAccounts(ctx, orgID, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")
}

// \1 performs an operation
func TestGetAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock accounts service
	mockService := mocks.NewMockAccountsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	accountID := "acc-123"
	alias := "test-account-1"

	// Create test account
	account := &models.Account{
		ID:             accountID,
		Name:           "Test Account 1",
		AssetCode:      "USD",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Type:           "LIABILITY",
		Alias:          &alias,
		Status: models.Status{
			Code: "ACTIVE",
		},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetAccount(gomock.Any(), orgID, ledgerID, accountID).
		Return(account, nil)

	// Test getting an account by ID
	result, err := mockService.GetAccount(ctx, orgID, ledgerID, accountID)
	assert.NoError(t, err)
	assert.Equal(t, accountID, result.ID)
	assert.Equal(t, "Test Account 1", result.Name)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, "LIABILITY", result.Type)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.NotNil(t, result.Alias)
	assert.Equal(t, alias, *result.Alias)

	// Test with empty organizationID
	mockService.EXPECT().
		GetAccount(gomock.Any(), "", ledgerID, accountID).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetAccount(ctx, "", ledgerID, accountID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetAccount(gomock.Any(), orgID, "", accountID).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetAccount(ctx, orgID, "", accountID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty accountID
	mockService.EXPECT().
		GetAccount(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("account ID is required"))

	_, err = mockService.GetAccount(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account ID is required")

	// Test with not found
	mockService.EXPECT().
		GetAccount(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Account not found"))

	_, err = mockService.GetAccount(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestGetAccountByAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock accounts service
	mockService := mocks.NewMockAccountsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	accountID := "acc-123"
	alias := "test-account-1"

	// Create test account
	account := &models.Account{
		ID:             accountID,
		Name:           "Test Account 1",
		AssetCode:      "USD",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Type:           "LIABILITY",
		Alias:          &alias,
		Status: models.Status{
			Code: "ACTIVE",
		},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetAccountByAlias(gomock.Any(), orgID, ledgerID, alias).
		Return(account, nil)

	// Test getting an account by alias
	result, err := mockService.GetAccountByAlias(ctx, orgID, ledgerID, alias)
	assert.NoError(t, err)
	assert.Equal(t, accountID, result.ID)
	assert.Equal(t, "Test Account 1", result.Name)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, "LIABILITY", result.Type)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.NotNil(t, result.Alias)
	assert.Equal(t, alias, *result.Alias)

	// Test with empty organizationID
	mockService.EXPECT().
		GetAccountByAlias(gomock.Any(), "", ledgerID, alias).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetAccountByAlias(ctx, "", ledgerID, alias)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetAccountByAlias(gomock.Any(), orgID, "", alias).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetAccountByAlias(ctx, orgID, "", alias)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty alias
	mockService.EXPECT().
		GetAccountByAlias(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("account alias is required"))

	_, err = mockService.GetAccountByAlias(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account alias is required")

	// Test with not found
	mockService.EXPECT().
		GetAccountByAlias(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Account not found"))

	_, err = mockService.GetAccountByAlias(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCreateAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock accounts service
	mockService := mocks.NewMockAccountsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	accountID := "acc-new"
	alias := "custom-alias"

	// Create test input
	input := models.NewCreateAccountInput("New Account", "USD", "ASSET").
		WithAlias(alias).
		WithStatus(models.NewStatus("ACTIVE")).
		WithMetadata(map[string]any{"key": "value"})

	// Create expected output
	account := &models.Account{
		ID:             accountID,
		Name:           "New Account",
		AssetCode:      "USD",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Type:           "ASSET",
		Alias:          &alias,
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata: map[string]any{"key": "value"},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreateAccount(gomock.Any(), orgID, ledgerID, input).
		Return(account, nil)

	// Test creating a new account
	result, err := mockService.CreateAccount(ctx, orgID, ledgerID, input)
	assert.NoError(t, err)
	assert.Equal(t, accountID, result.ID)
	assert.Equal(t, "New Account", result.Name)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, "ASSET", result.Type)
	assert.Equal(t, "ACTIVE", result.Status.Code)
	assert.NotNil(t, result.Alias)
	assert.Equal(t, alias, *result.Alias)
	assert.Equal(t, "value", result.Metadata["key"])

	// Test with empty organizationID
	mockService.EXPECT().
		CreateAccount(gomock.Any(), "", ledgerID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.CreateAccount(ctx, "", ledgerID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		CreateAccount(gomock.Any(), orgID, "", input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.CreateAccount(ctx, orgID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with nil input
	mockService.EXPECT().
		CreateAccount(gomock.Any(), orgID, ledgerID, nil).
		Return(nil, fmt.Errorf("account input cannot be nil"))

	_, err = mockService.CreateAccount(ctx, orgID, ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account input cannot be nil")
}

// \1 performs an operation
func TestUpdateAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock accounts service
	mockService := mocks.NewMockAccountsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	accountID := "acc-123"
	alias := "test-account-1"

	// Create test input
	input := models.NewUpdateAccountInput().
		WithName("Updated Account").
		WithStatus(models.NewStatus("INACTIVE")).
		WithMetadata(map[string]any{"key": "updated"})

	// Create expected output
	account := &models.Account{
		ID:             accountID,
		Name:           "Updated Account",
		AssetCode:      "USD",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Type:           "LIABILITY",
		Alias:          &alias,
		Status: models.Status{
			Code: "INACTIVE",
		},
		Metadata: map[string]any{"key": "updated"},
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		UpdateAccount(gomock.Any(), orgID, ledgerID, accountID, input).
		Return(account, nil)

	// Test updating an account
	result, err := mockService.UpdateAccount(ctx, orgID, ledgerID, accountID, input)
	assert.NoError(t, err)
	assert.Equal(t, accountID, result.ID)
	assert.Equal(t, "Updated Account", result.Name)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, "LIABILITY", result.Type)
	assert.Equal(t, "INACTIVE", result.Status.Code)
	assert.Equal(t, "updated", result.Metadata["key"])

	// Test with empty organizationID
	mockService.EXPECT().
		UpdateAccount(gomock.Any(), "", ledgerID, accountID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.UpdateAccount(ctx, "", ledgerID, accountID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		UpdateAccount(gomock.Any(), orgID, "", accountID, input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.UpdateAccount(ctx, orgID, "", accountID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty accountID
	mockService.EXPECT().
		UpdateAccount(gomock.Any(), orgID, ledgerID, "", input).
		Return(nil, fmt.Errorf("account ID is required"))

	_, err = mockService.UpdateAccount(ctx, orgID, ledgerID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account ID is required")

	// Test with nil input
	mockService.EXPECT().
		UpdateAccount(gomock.Any(), orgID, ledgerID, accountID, nil).
		Return(nil, fmt.Errorf("account input cannot be nil"))

	_, err = mockService.UpdateAccount(ctx, orgID, ledgerID, accountID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account input cannot be nil")

	// Test with not found
	mockService.EXPECT().
		UpdateAccount(gomock.Any(), orgID, ledgerID, "not-found", input).
		Return(nil, fmt.Errorf("Account not found"))

	_, err = mockService.UpdateAccount(ctx, orgID, ledgerID, "not-found", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestDeleteAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock accounts service
	mockService := mocks.NewMockAccountsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	accountID := "acc-123"

	// Setup expectation for successful case
	mockService.EXPECT().
		DeleteAccount(gomock.Any(), orgID, ledgerID, accountID).
		Return(nil)

	// Test deleting an account
	err := mockService.DeleteAccount(ctx, orgID, ledgerID, accountID)
	assert.NoError(t, err)

	// Test with empty organizationID
	mockService.EXPECT().
		DeleteAccount(gomock.Any(), "", ledgerID, accountID).
		Return(fmt.Errorf("organization ID is required"))

	err = mockService.DeleteAccount(ctx, "", ledgerID, accountID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		DeleteAccount(gomock.Any(), orgID, "", accountID).
		Return(fmt.Errorf("ledger ID is required"))

	err = mockService.DeleteAccount(ctx, orgID, "", accountID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty accountID
	mockService.EXPECT().
		DeleteAccount(gomock.Any(), orgID, ledgerID, "").
		Return(fmt.Errorf("account ID is required"))

	err = mockService.DeleteAccount(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account ID is required")

	// Test with not found
	mockService.EXPECT().
		DeleteAccount(gomock.Any(), orgID, ledgerID, "not-found").
		Return(fmt.Errorf("Account not found"))

	err = mockService.DeleteAccount(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestGetBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock accounts service
	mockService := mocks.NewMockAccountsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	accountID := "acc-123"
	accountAlias := "test-account-1"

	// Create test balance
	balance := &models.Balance{
		ID:             "bal-123",
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		Alias:          accountAlias,
		AssetCode:      "USD",
		Available:      1000000,
		OnHold:         0,
		Scale:          100,
		Version:        1,
		AccountType:    "LIABILITY",
		AllowSending:   true,
		AllowReceiving: true,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, ledgerID, accountID).
		Return(balance, nil)

	// Test getting an account's balance
	result, err := mockService.GetBalance(ctx, orgID, ledgerID, accountID)
	assert.NoError(t, err)
	assert.Equal(t, "bal-123", result.ID)
	assert.Equal(t, accountID, result.AccountID)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, "LIABILITY", result.AccountType)
	assert.Equal(t, int64(1000000), result.Available)
	assert.Equal(t, int64(0), result.OnHold)
	assert.Equal(t, int64(100), result.Scale)
	assert.Equal(t, true, result.AllowSending)
	assert.Equal(t, true, result.AllowReceiving)

	// Test with empty organizationID
	mockService.EXPECT().
		GetBalance(gomock.Any(), "", ledgerID, accountID).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetBalance(ctx, "", ledgerID, accountID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, "", accountID).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetBalance(ctx, orgID, "", accountID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty accountID
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("account ID is required"))

	_, err = mockService.GetBalance(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account ID is required")

	// Test with not found
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Balance not found"))

	_, err = mockService.GetBalance(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
