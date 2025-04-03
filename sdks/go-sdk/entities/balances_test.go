package entities

import (
	"context"
	"testing"
	"time"

	"fmt"
	"github.com/LerianStudio/midaz/sdks/go-sdk/entities/mocks"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func TestListBalances(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test balances list response
	balancesList := &models.ListResponse[models.Balance]{
		Items: []models.Balance{
			{
				ID:             "bal-123",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "acc-1",
				Alias:          "user-account",
				AssetCode:      "USD",
				Available:      1000000,
				OnHold:         0,
				Scale:          100,
				Version:        1,
				AccountType:    "LIABILITY",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			{
				ID:             "bal-456",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "acc-2",
				Alias:          "platform-account",
				AssetCode:      "USD",
				Available:      5000000,
				OnHold:         200000,
				Scale:          100,
				Version:        2,
				AccountType:    "ASSET",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		Pagination: models.Pagination{
			Total:  2,
			Limit:  10,
			Offset: 0,
		},
	}

	// Setup expectations for default options
	mockService.EXPECT().
		ListBalances(gomock.Any(), orgID, ledgerID, gomock.Nil()).
		Return(balancesList, nil)

	// Test listing balances with default options
	result, err := mockService.ListBalances(ctx, orgID, ledgerID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "bal-123", result.Items[0].ID)
	assert.Equal(t, "acc-1", result.Items[0].AccountID)
	assert.Equal(t, "user-account", result.Items[0].Alias)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, int64(1000000), result.Items[0].Available)
	assert.Equal(t, int64(0), result.Items[0].OnHold)
	assert.Equal(t, int64(100), result.Items[0].Scale)
	assert.Equal(t, "LIABILITY", result.Items[0].AccountType)
	assert.Equal(t, true, result.Items[0].AllowSending)
	assert.Equal(t, true, result.Items[0].AllowReceiving)

	// Test with options
	opts := &models.ListOptions{
		Limit:          5,
		Offset:         0,
		OrderBy:        "created_at",
		OrderDirection: "desc",
	}

	mockService.EXPECT().
		ListBalances(gomock.Any(), orgID, ledgerID, opts).
		Return(balancesList, nil)

	result, err = mockService.ListBalances(ctx, orgID, ledgerID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)

	// Test with empty organizationID
	mockService.EXPECT().
		ListBalances(gomock.Any(), "", ledgerID, gomock.Any()).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.ListBalances(ctx, "", ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		ListBalances(gomock.Any(), orgID, "", gomock.Any()).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.ListBalances(ctx, orgID, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")
}

// \1 performs an operation
func TestListAccountBalances(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	accountID := "acc-1"
	now := time.Now()

	// Create test balances list response
	balancesList := &models.ListResponse[models.Balance]{
		Items: []models.Balance{
			{
				ID:             "bal-123",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "user-account",
				AssetCode:      "USD",
				Available:      1000000,
				OnHold:         0,
				Scale:          100,
				Version:        1,
				AccountType:    "LIABILITY",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			{
				ID:             "bal-789",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "user-account",
				AssetCode:      "EUR",
				Available:      850000,
				OnHold:         0,
				Scale:          100,
				Version:        1,
				AccountType:    "LIABILITY",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		Pagination: models.Pagination{
			Total:  2,
			Limit:  10,
			Offset: 0,
		},
	}

	// Setup expectations for default options
	mockService.EXPECT().
		ListAccountBalances(gomock.Any(), orgID, ledgerID, accountID, gomock.Nil()).
		Return(balancesList, nil)

	// Test listing account balances with default options
	result, err := mockService.ListAccountBalances(ctx, orgID, ledgerID, accountID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "bal-123", result.Items[0].ID)
	assert.Equal(t, accountID, result.Items[0].AccountID)
	assert.Equal(t, "user-account", result.Items[0].Alias)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, "bal-789", result.Items[1].ID)
	assert.Equal(t, "EUR", result.Items[1].AssetCode)

	// Test with options
	opts := &models.ListOptions{
		Limit:          5,
		Offset:         0,
		OrderBy:        "created_at",
		OrderDirection: "desc",
	}

	mockService.EXPECT().
		ListAccountBalances(gomock.Any(), orgID, ledgerID, accountID, opts).
		Return(balancesList, nil)

	result, err = mockService.ListAccountBalances(ctx, orgID, ledgerID, accountID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)

	// Test with empty organizationID
	mockService.EXPECT().
		ListAccountBalances(gomock.Any(), "", ledgerID, accountID, gomock.Any()).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.ListAccountBalances(ctx, "", ledgerID, accountID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		ListAccountBalances(gomock.Any(), orgID, "", accountID, gomock.Any()).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.ListAccountBalances(ctx, orgID, "", accountID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty accountID
	mockService.EXPECT().
		ListAccountBalances(gomock.Any(), orgID, ledgerID, "", gomock.Any()).
		Return(nil, fmt.Errorf("account ID is required"))

	_, err = mockService.ListAccountBalances(ctx, orgID, ledgerID, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account ID is required")
}

// \1 performs an operation
func TestGetBalanceByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	balanceID := "bal-123"
	now := time.Now()

	// Create test balance
	balance := &models.Balance{
		ID:             balanceID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      "acc-1",
		Alias:          "user-account",
		AssetCode:      "USD",
		Available:      1000000,
		OnHold:         0,
		Scale:          100,
		Version:        1,
		AccountType:    "LIABILITY",
		AllowSending:   true,
		AllowReceiving: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, ledgerID, balanceID).
		Return(balance, nil)

	// Test getting a balance by ID
	result, err := mockService.GetBalance(ctx, orgID, ledgerID, balanceID)
	assert.NoError(t, err)
	assert.Equal(t, balanceID, result.ID)
	assert.Equal(t, "acc-1", result.AccountID)
	assert.Equal(t, "user-account", result.Alias)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, int64(1000000), result.Available)
	assert.Equal(t, int64(0), result.OnHold)
	assert.Equal(t, int64(100), result.Scale)
	assert.Equal(t, "LIABILITY", result.AccountType)
	assert.Equal(t, true, result.AllowSending)
	assert.Equal(t, true, result.AllowReceiving)

	// Test with empty organizationID
	mockService.EXPECT().
		GetBalance(gomock.Any(), "", ledgerID, balanceID).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.GetBalance(ctx, "", ledgerID, balanceID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, "", balanceID).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.GetBalance(ctx, orgID, "", balanceID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty balanceID
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("balance ID is required"))

	_, err = mockService.GetBalance(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "balance ID is required")

	// Test with not found
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Balance not found"))

	_, err = mockService.GetBalance(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestUpdateBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	balanceID := "bal-123"
	now := time.Now()

	// Create test input
	allowSending := true
	allowReceiving := false
	input := &models.UpdateBalanceInput{
		AllowSending:   &allowSending,
		AllowReceiving: &allowReceiving,
	}

	// Create expected output
	balance := &models.Balance{
		ID:             balanceID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      "acc-1",
		Alias:          "user-account",
		AssetCode:      "USD",
		Available:      1000000,
		OnHold:         0,
		Scale:          100,
		Version:        2,
		AccountType:    "LIABILITY",
		AllowSending:   true,
		AllowReceiving: false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		UpdateBalance(gomock.Any(), orgID, ledgerID, balanceID, input).
		Return(balance, nil)

	// Test updating a balance
	result, err := mockService.UpdateBalance(ctx, orgID, ledgerID, balanceID, input)
	assert.NoError(t, err)
	assert.Equal(t, balanceID, result.ID)
	assert.Equal(t, int64(2), result.Version)
	assert.Equal(t, true, result.AllowSending)
	assert.Equal(t, false, result.AllowReceiving)

	// Test with empty organizationID
	mockService.EXPECT().
		UpdateBalance(gomock.Any(), "", ledgerID, balanceID, input).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.UpdateBalance(ctx, "", ledgerID, balanceID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		UpdateBalance(gomock.Any(), orgID, "", balanceID, input).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.UpdateBalance(ctx, orgID, "", balanceID, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty balanceID
	mockService.EXPECT().
		UpdateBalance(gomock.Any(), orgID, ledgerID, "", input).
		Return(nil, fmt.Errorf("balance ID is required"))

	_, err = mockService.UpdateBalance(ctx, orgID, ledgerID, "", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "balance ID is required")

	// Test with nil input
	mockService.EXPECT().
		UpdateBalance(gomock.Any(), orgID, ledgerID, balanceID, nil).
		Return(nil, fmt.Errorf("balance input cannot be nil"))

	_, err = mockService.UpdateBalance(ctx, orgID, ledgerID, balanceID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "balance input cannot be nil")

	// Test with not found
	mockService.EXPECT().
		UpdateBalance(gomock.Any(), orgID, ledgerID, "not-found", input).
		Return(nil, fmt.Errorf("Balance not found"))

	_, err = mockService.UpdateBalance(ctx, orgID, ledgerID, "not-found", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestDeleteBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	balanceID := "bal-123"

	// Setup expectation for successful case
	mockService.EXPECT().
		DeleteBalance(gomock.Any(), orgID, ledgerID, balanceID).
		Return(nil)

	// Test deleting a balance
	err := mockService.DeleteBalance(ctx, orgID, ledgerID, balanceID)
	assert.NoError(t, err)

	// Test with empty organizationID
	mockService.EXPECT().
		DeleteBalance(gomock.Any(), "", ledgerID, balanceID).
		Return(fmt.Errorf("organization ID is required"))

	err = mockService.DeleteBalance(ctx, "", ledgerID, balanceID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		DeleteBalance(gomock.Any(), orgID, "", balanceID).
		Return(fmt.Errorf("ledger ID is required"))

	err = mockService.DeleteBalance(ctx, orgID, "", balanceID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")

	// Test with empty balanceID
	mockService.EXPECT().
		DeleteBalance(gomock.Any(), orgID, ledgerID, "").
		Return(fmt.Errorf("balance ID is required"))

	err = mockService.DeleteBalance(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "balance ID is required")

	// Test with not found
	mockService.EXPECT().
		DeleteBalance(gomock.Any(), orgID, ledgerID, "not-found").
		Return(fmt.Errorf("Balance not found"))

	err = mockService.DeleteBalance(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
