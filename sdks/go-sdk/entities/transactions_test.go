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

// stringPtr is a helper function to convert a string to a pointer
func stringPtr(s string) *string {
	return &s
}

// \1 performs an operation
func TestListTransactions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock transactions service
	mockService := mocks.NewMockTransactionsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test transaction list response
	transactionsList := &models.ListResponse[models.Transaction]{
		Items: []models.Transaction{
			{
				ID:             "tx-123",
				AssetCode:      "USD",
				Amount:         1000,
				Scale:          2,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Status: models.Status{
					Code: "COMPLETED",
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:             "tx-456",
				AssetCode:      "EUR",
				Amount:         2000,
				Scale:          2,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Status: models.Status{
					Code: "COMPLETED",
				},
				CreatedAt: now,
				UpdatedAt: now,
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
		ListTransactions(gomock.Any(), orgID, ledgerID, gomock.Nil()).
		Return(transactionsList, nil)

	// Test listing transactions with default options
	result, err := mockService.ListTransactions(ctx, orgID, ledgerID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "tx-123", result.Items[0].ID)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, int64(1000), result.Items[0].Amount)
	assert.Equal(t, int64(2), result.Items[0].Scale)
	assert.Equal(t, "COMPLETED", result.Items[0].Status.Code)
	assert.Equal(t, orgID, result.Items[0].OrganizationID)
	assert.Equal(t, ledgerID, result.Items[0].LedgerID)

	// Test with options
	opts := &models.ListOptions{
		Limit:          5,
		Offset:         0,
		OrderBy:        "created_at",
		OrderDirection: "desc",
	}

	mockService.EXPECT().
		ListTransactions(gomock.Any(), orgID, ledgerID, opts).
		Return(transactionsList, nil)

	result, err = mockService.ListTransactions(ctx, orgID, ledgerID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)

	// Test with empty organizationID
	mockService.EXPECT().
		ListTransactions(gomock.Any(), "", ledgerID, gomock.Any()).
		Return(nil, fmt.Errorf("organization ID is required"))

	_, err = mockService.ListTransactions(ctx, "", ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "organization ID is required")

	// Test with empty ledgerID
	mockService.EXPECT().
		ListTransactions(gomock.Any(), orgID, "", gomock.Any()).
		Return(nil, fmt.Errorf("ledger ID is required"))

	_, err = mockService.ListTransactions(ctx, orgID, "", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ledger ID is required")
}

// \1 performs an operation
func TestGetTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock transactions service
	mockService := mocks.NewMockTransactionsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	transactionID := "tx-123"
	now := time.Now()

	// Create test transaction
	transaction := &models.Transaction{
		ID:             transactionID,
		AssetCode:      "USD",
		Amount:         1000,
		Scale:          2,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "COMPLETED",
		},
		Operations: []models.Operation{
			{
				ID:           "op-1",
				Type:         "DEBIT",
				AccountID:    "acc-1",
				Amount:       1000,
				AssetCode:    "USD",
				Scale:        2,
				AccountAlias: stringPtr("user-account"),
			},
			{
				ID:           "op-2",
				Type:         "CREDIT",
				AccountID:    "acc-2",
				Amount:       1000,
				AssetCode:    "USD",
				Scale:        2,
				AccountAlias: stringPtr("platform-account"),
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		GetTransaction(gomock.Any(), orgID, ledgerID, transactionID).
		Return(transaction, nil)

	// Test getting a transaction by ID
	result, err := mockService.GetTransaction(ctx, orgID, ledgerID, transactionID)
	assert.NoError(t, err)
	assert.Equal(t, transactionID, result.ID)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, int64(1000), result.Amount)
	assert.Equal(t, int64(2), result.Scale)
	assert.Equal(t, "COMPLETED", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)

	// Verify operations
	assert.Len(t, result.Operations, 2)
	assert.Equal(t, "DEBIT", result.Operations[0].Type)
	assert.Equal(t, "CREDIT", result.Operations[1].Type)
	assert.NotNil(t, result.Operations[0].AccountAlias)
	assert.NotNil(t, result.Operations[1].AccountAlias)
	assert.Equal(t, "user-account", *result.Operations[0].AccountAlias)
	assert.Equal(t, "platform-account", *result.Operations[1].AccountAlias)

	// Test with empty transactionID
	mockService.EXPECT().
		GetTransaction(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("transaction ID is required"))

	_, err = mockService.GetTransaction(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction ID is required")

	// Test with not found
	mockService.EXPECT().
		GetTransaction(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Transaction not found"))

	_, err = mockService.GetTransaction(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCreateTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock transactions service
	mockService := mocks.NewMockTransactionsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test input
	input := &models.CreateTransactionInput{
		AssetCode: "USD",
		Amount:    5000,
		Scale:     2,
		Operations: []models.CreateOperationInput{
			{
				Type:         "DEBIT",
				AccountID:    "acc-1",
				Amount:       5000,
				AssetCode:    "USD",
				Scale:        2,
				AccountAlias: stringPtr("source-account"),
			},
			{
				Type:         "CREDIT",
				AccountID:    "acc-2",
				Amount:       5000,
				AssetCode:    "USD",
				Scale:        2,
				AccountAlias: stringPtr("destination-account"),
			},
		},
		Metadata: map[string]any{
			"reference": "payment-123",
		},
	}

	// Create expected output
	transaction := &models.Transaction{
		ID:             "tx-new",
		AssetCode:      "USD",
		Amount:         5000,
		Scale:          2,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "PENDING",
		},
		Metadata:  map[string]any{"reference": "payment-123"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreateTransaction(gomock.Any(), orgID, ledgerID, input).
		Return(transaction, nil)

	// Test creating a new transaction
	result, err := mockService.CreateTransaction(ctx, orgID, ledgerID, input)
	assert.NoError(t, err)
	assert.Equal(t, "tx-new", result.ID)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, int64(5000), result.Amount)
	assert.Equal(t, int64(2), result.Scale)
	assert.Equal(t, "PENDING", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "payment-123", result.Metadata["reference"])

	// Test with nil input
	mockService.EXPECT().
		CreateTransaction(gomock.Any(), orgID, ledgerID, nil).
		Return(nil, fmt.Errorf("transaction input cannot be nil"))

	_, err = mockService.CreateTransaction(ctx, orgID, ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction input cannot be nil")
}

// \1 performs an operation
func TestCreateTransactionWithDSL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock transactions service
	mockService := mocks.NewMockTransactionsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test input
	input := &models.TransactionDSLInput{
		Description: "Test DSL transaction",
		Send: &models.DSLSend{
			Asset: "USD",
			Value: 1000,
			Scale: 2,
			Source: &models.DSLSource{
				From: []models.DSLFromTo{
					{
						Account: "source-account",
						Amount: &models.DSLAmount{
							Value: 1000,
							Scale: 2,
							Asset: "USD",
						},
					},
				},
			},
			Distribute: &models.DSLDistribute{
				To: []models.DSLFromTo{
					{
						Account: "destination-account",
						Amount: &models.DSLAmount{
							Value: 1000,
							Scale: 2,
							Asset: "USD",
						},
					},
				},
			},
		},
		Metadata: map[string]any{
			"reference": "dsl-payment-123",
		},
	}

	// Create expected output
	transaction := &models.Transaction{
		ID:             "tx-dsl",
		AssetCode:      "USD",
		Amount:         1000,
		Scale:          2,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "PENDING",
		},
		Metadata:  map[string]any{"reference": "dsl-payment-123"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CreateTransactionWithDSL(gomock.Any(), orgID, ledgerID, input).
		Return(transaction, nil)

	// Test creating a transaction with DSL
	result, err := mockService.CreateTransactionWithDSL(ctx, orgID, ledgerID, input)
	assert.NoError(t, err)
	assert.Equal(t, "tx-dsl", result.ID)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, int64(1000), result.Amount)
	assert.Equal(t, int64(2), result.Scale)
	assert.Equal(t, "PENDING", result.Status.Code)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "dsl-payment-123", result.Metadata["reference"])

	// Test with nil input
	mockService.EXPECT().
		CreateTransactionWithDSL(gomock.Any(), orgID, ledgerID, nil).
		Return(nil, fmt.Errorf("transaction DSL input cannot be nil"))

	_, err = mockService.CreateTransactionWithDSL(ctx, orgID, ledgerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction DSL input cannot be nil")
}

// \1 performs an operation
func TestUpdateTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock transactions service
	mockService := mocks.NewMockTransactionsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	transactionID := "tx-123"
	now := time.Now()

	// Create test input
	input := map[string]any{
		"metadata": map[string]any{
			"reference": "updated-payment-123",
			"status":    "processed",
		},
	}

	// Create expected output
	transaction := &models.Transaction{
		ID:             transactionID,
		AssetCode:      "USD",
		Amount:         1000,
		Scale:          2,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "COMPLETED",
		},
		Metadata:  map[string]any{"reference": "updated-payment-123", "status": "processed"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		UpdateTransaction(gomock.Any(), orgID, ledgerID, transactionID, input).
		Return(transaction, nil)

	// Test updating a transaction
	result, err := mockService.UpdateTransaction(ctx, orgID, ledgerID, transactionID, input)
	assert.NoError(t, err)
	assert.Equal(t, transactionID, result.ID)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, int64(1000), result.Amount)
	assert.Equal(t, int64(2), result.Scale)
	assert.Equal(t, "COMPLETED", result.Status.Code)

	// Test with nil input
	mockService.EXPECT().
		UpdateTransaction(gomock.Any(), orgID, ledgerID, transactionID, nil).
		Return(nil, fmt.Errorf("update input cannot be nil"))

	_, err = mockService.UpdateTransaction(ctx, orgID, ledgerID, transactionID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update input cannot be nil")

	// Test with not found
	mockService.EXPECT().
		UpdateTransaction(gomock.Any(), orgID, ledgerID, "not-found", input).
		Return(nil, fmt.Errorf("Transaction not found"))

	_, err = mockService.UpdateTransaction(ctx, orgID, ledgerID, "not-found", input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCommitTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock transactions service
	mockService := mocks.NewMockTransactionsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	transactionID := "tx-123"
	now := time.Now()

	// Create expected output
	transaction := &models.Transaction{
		ID:             transactionID,
		AssetCode:      "USD",
		Amount:         1000,
		Scale:          2,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "COMPLETED",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CommitTransaction(gomock.Any(), orgID, ledgerID, transactionID).
		Return(transaction, nil)

	// Test committing a transaction
	result, err := mockService.CommitTransaction(ctx, orgID, ledgerID, transactionID)
	assert.NoError(t, err)
	assert.Equal(t, transactionID, result.ID)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, int64(1000), result.Amount)
	assert.Equal(t, int64(2), result.Scale)
	assert.Equal(t, "COMPLETED", result.Status.Code)

	// Test with empty transactionID
	mockService.EXPECT().
		CommitTransaction(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("transaction ID is required"))

	_, err = mockService.CommitTransaction(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "transaction ID is required")

	// Test with not found
	mockService.EXPECT().
		CommitTransaction(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Transaction not found"))

	_, err = mockService.CommitTransaction(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// \1 performs an operation
func TestCommitTransactionWithExternalID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock transactions service
	mockService := mocks.NewMockTransactionsService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	externalID := "ext-123"
	now := time.Now()

	// Create expected output
	transaction := &models.Transaction{
		ID:             "tx-123",
		AssetCode:      "USD",
		Amount:         1000,
		Scale:          2,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Status: models.Status{
			Code: "COMPLETED",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Setup expectation for successful case
	mockService.EXPECT().
		CommitTransactionWithExternalID(gomock.Any(), orgID, ledgerID, externalID).
		Return(transaction, nil)

	// Test committing a transaction with external ID
	result, err := mockService.CommitTransactionWithExternalID(ctx, orgID, ledgerID, externalID)
	assert.NoError(t, err)
	assert.Equal(t, "tx-123", result.ID)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, int64(1000), result.Amount)
	assert.Equal(t, int64(2), result.Scale)
	assert.Equal(t, "COMPLETED", result.Status.Code)

	// Test with empty externalID
	mockService.EXPECT().
		CommitTransactionWithExternalID(gomock.Any(), orgID, ledgerID, "").
		Return(nil, fmt.Errorf("external ID is required"))

	_, err = mockService.CommitTransactionWithExternalID(ctx, orgID, ledgerID, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "external ID is required")

	// Test with not found
	mockService.EXPECT().
		CommitTransactionWithExternalID(gomock.Any(), orgID, ledgerID, "not-found").
		Return(nil, fmt.Errorf("Transaction not found"))

	_, err = mockService.CommitTransactionWithExternalID(ctx, orgID, ledgerID, "not-found")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
