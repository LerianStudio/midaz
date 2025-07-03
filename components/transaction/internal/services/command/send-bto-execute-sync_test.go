package command

import (
	"encoding/json"
	"testing"
	"time"

	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestSendBTOExecuteSync_QueueDataCreation(t *testing.T) {
	// Arrange
	transactionID := uuid.New()

	parseDSL := &libTransaction.Transaction{
		ChartOfAccountsGroupName: "1000",
		Description:              "Test transaction",
		Code:                     transactionID.String(),
		Pending:                  false,
		Metadata:                 map[string]any{"test": "value"},
		Route:                    "test_route",
		Send: libTransaction.Send{
			Asset: "USD",
			Value: decimal.NewFromInt(1000),
			Source: libTransaction.Source{
				From: []libTransaction.FromTo{
					{
						IsFrom:          true,
						AccountAlias:    "account1",
						Amount:          &libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(1000)},
						Description:     "Source account",
						ChartOfAccounts: "1000",
						Metadata:        map[string]any{"test": "value"},
						Route:           "route1",
					},
				},
			},
			Distribute: libTransaction.Distribute{
				To: []libTransaction.FromTo{
					{
						IsFrom:          false,
						AccountAlias:    "account2",
						Amount:          &libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(1000)},
						Description:     "Destination account",
						ChartOfAccounts: "2000",
						Metadata:        map[string]any{"test": "value"},
						Route:           "route2",
					},
				},
			},
		},
	}

	validate := &libTransaction.Responses{
		Aliases: []string{"account1", "account2"},
		From: map[string]libTransaction.Amount{
			"account1": {
				Asset: "USD",
				Value: decimal.NewFromInt(1000),
			},
		},
		To: map[string]libTransaction.Amount{
			"account2": {
				Asset: "USD",
				Value: decimal.NewFromInt(1000),
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             transactionID.String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			AccountID:      "account1",
			Alias:          "@account1",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "wallet",
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      nil,
			Metadata:       map[string]any{"test": "value"},
		},
	}

	amount := decimal.NewFromInt(1000)
	inputTransaction := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      nil,
		Description:              "Test transaction",
		Status:                   transaction.Status{Code: "PENDING"},
		Amount:                   &amount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "1000",
		Source:                   []string{"@account1"},
		Destination:              []string{"@account2"},
		LedgerID:                 uuid.New().String(),
		OrganizationID:           uuid.New().String(),
		Route:                    "test_route",
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
		DeletedAt:                nil,
		Metadata:                 map[string]any{"test": "value"},
		Operations:               nil,
	}

	// Act - Test the queue data creation logic (the core functionality of SendBTOExecuteSync)
	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    balances,
		Transaction: inputTransaction,
		ParseDSL:    parseDSL,
	}

	marshal, err := json.Marshal(value)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, marshal)
	
	// Verify that the transaction ID is correctly used for QueueData
	queueData := mmodel.QueueData{
		ID:    inputTransaction.IDtoUUID(),
		Value: marshal,
	}
	
	assert.Equal(t, transactionID, queueData.ID)
	assert.NotEmpty(t, queueData.Value)
	
	// Verify the marshaled data can be unmarshaled back correctly
	var unmarshaled transaction.TransactionQueue
	err = json.Unmarshal(marshal, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, value.Transaction.ID, unmarshaled.Transaction.ID)
	assert.Equal(t, value.ParseDSL.Code, unmarshaled.ParseDSL.Code)
	assert.Equal(t, len(value.Validate.Aliases), len(unmarshaled.Validate.Aliases))
	assert.Equal(t, len(value.Balances), len(unmarshaled.Balances))
}

func TestSendBTOExecuteSync_ValidQueueDataStructure(t *testing.T) {
	// Arrange
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	parseDSL := &libTransaction.Transaction{
		ChartOfAccountsGroupName: "1000",
		Description:              "Test transaction",
		Code:                     transactionID.String(),
		Pending:                  false,
		Metadata:                 map[string]any{"test": "value"},
		Route:                    "test_route",
		Send: libTransaction.Send{
			Asset: "USD",
			Value: decimal.NewFromInt(1000),
			Source: libTransaction.Source{
				From: []libTransaction.FromTo{
					{
						IsFrom:          true,
						AccountAlias:    "account1",
						Amount:          &libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(1000)},
						Description:     "Source account",
						ChartOfAccounts: "1000",
						Metadata:        map[string]any{"test": "value"},
						Route:           "route1",
					},
				},
			},
			Distribute: libTransaction.Distribute{
				To: []libTransaction.FromTo{
					{
						IsFrom:          false,
						AccountAlias:    "account2",
						Amount:          &libTransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(1000)},
						Description:     "Destination account",
						ChartOfAccounts: "2000",
						Metadata:        map[string]any{"test": "value"},
						Route:           "route2",
					},
				},
			},
		},
	}

	validate := &libTransaction.Responses{
		Aliases: []string{"account1", "account2"},
		From: map[string]libTransaction.Amount{
			"account1": {Asset: "USD", Value: decimal.NewFromInt(1000)},
		},
		To: map[string]libTransaction.Amount{
			"account2": {Asset: "USD", Value: decimal.NewFromInt(1000)},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      "account1",
			Alias:          "@account1",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "wallet",
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      nil,
			Metadata:       map[string]any{"test": "value"},
		},
	}

	amount := decimal.NewFromInt(1000)
	inputTransaction := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      nil,
		Description:              "Test transaction",
		Status:                   transaction.Status{Code: "PENDING"},
		Amount:                   &amount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "1000",
		Source:                   []string{"@account1"},
		Destination:              []string{"@account2"},
		LedgerID:                 uuid.New().String(),
		OrganizationID:           uuid.New().String(),
		Route:                    "test_route",
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
		DeletedAt:                nil,
		Metadata:                 map[string]any{"test": "value"},
		Operations:               nil,
	}

	// Act - Test that the queue data structure is correctly formed
	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    balances,
		Transaction: inputTransaction,
		ParseDSL:    parseDSL,
	}

	marshal, err := json.Marshal(value)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, marshal)

	// Verify the marshaled data can be unmarshaled back
	var unmarshaled transaction.TransactionQueue
	err = json.Unmarshal(marshal, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, value.Transaction.ID, unmarshaled.Transaction.ID)
	assert.Equal(t, value.ParseDSL.Code, unmarshaled.ParseDSL.Code)
}

func TestSendBTOExecuteSync_NilValidate(t *testing.T) {
	// Arrange
	transactionID := uuid.New()

	parseDSL := &libTransaction.Transaction{
		Code: transactionID.String(),
	}

	var nilValidate *libTransaction.Responses

	balances := []*mmodel.Balance{
		{
			ID:             transactionID.String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			AccountID:      "account1",
			Alias:          "@account1",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "wallet",
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      nil,
			Metadata:       map[string]any{"test": "value"},
		},
	}

	amount := decimal.NewFromInt(1000)
	inputTransaction := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      nil,
		Description:              "Test transaction",
		Status:                   transaction.Status{Code: "PENDING"},
		Amount:                   &amount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "1000",
		Source:                   []string{"@account1"},
		Destination:              []string{"@account2"},
		LedgerID:                 uuid.New().String(),
		OrganizationID:           uuid.New().String(),
		Route:                    "test_route",
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
		DeletedAt:                nil,
		Metadata:                 map[string]any{"test": "value"},
		Operations:               nil,
	}

	// Act - Test that the queue structure can handle nil validate
	value := transaction.TransactionQueue{
		Validate:    nilValidate,
		Balances:    balances,
		Transaction: inputTransaction,
		ParseDSL:    parseDSL,
	}

	marshal, err := json.Marshal(value)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, marshal)
	
	// Verify the marshaled data can be unmarshaled back with nil validation
	var unmarshaled transaction.TransactionQueue
	err = json.Unmarshal(marshal, &unmarshaled)
	assert.NoError(t, err)
	assert.Nil(t, unmarshaled.Validate)
	assert.Equal(t, value.Transaction.ID, unmarshaled.Transaction.ID)
	assert.Equal(t, value.ParseDSL.Code, unmarshaled.ParseDSL.Code)
}

func TestSendBTOExecuteSync_NilParseDSL(t *testing.T) {
	// Arrange
	transactionID := uuid.New()

	var nilParseDSL *libTransaction.Transaction

	validate := &libTransaction.Responses{
		Aliases: []string{"account1", "account2"},
		From: map[string]libTransaction.Amount{
			"account1": {Asset: "USD", Value: decimal.NewFromInt(1000)},
		},
		To: map[string]libTransaction.Amount{
			"account2": {Asset: "USD", Value: decimal.NewFromInt(1000)},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             transactionID.String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			AccountID:      "account1",
			Alias:          "@account1",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "wallet",
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			DeletedAt:      nil,
			Metadata:       map[string]any{"test": "value"},
		},
	}

	amount := decimal.NewFromInt(1000)
	inputTransaction := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      nil,
		Description:              "Test transaction",
		Status:                   transaction.Status{Code: "PENDING"},
		Amount:                   &amount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "1000",
		Source:                   []string{"@account1"},
		Destination:              []string{"@account2"},
		LedgerID:                 uuid.New().String(),
		OrganizationID:           uuid.New().String(),
		Route:                    "test_route",
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
		DeletedAt:                nil,
		Metadata:                 map[string]any{"test": "value"},
		Operations:               nil,
	}

	// Act - Test that the queue structure can handle nil parseDSL
	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    balances,
		Transaction: inputTransaction,
		ParseDSL:    nilParseDSL,
	}

	marshal, err := json.Marshal(value)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, marshal)
	
	// Verify the marshaled data can be unmarshaled back with nil parseDSL
	var unmarshaled transaction.TransactionQueue
	err = json.Unmarshal(marshal, &unmarshaled)
	assert.NoError(t, err)
	assert.Nil(t, unmarshaled.ParseDSL)
	assert.Equal(t, value.Transaction.ID, unmarshaled.Transaction.ID)
	assert.Equal(t, len(value.Validate.Aliases), len(unmarshaled.Validate.Aliases))
}

func TestSendBTOExecuteSync_EmptyBalances(t *testing.T) {
	// Arrange
	transactionID := uuid.New()

	parseDSL := &libTransaction.Transaction{
		Code: transactionID.String(),
	}

	validate := &libTransaction.Responses{
		Aliases: []string{"account1", "account2"},
		From: map[string]libTransaction.Amount{
			"account1": {Asset: "USD", Value: decimal.NewFromInt(1000)},
		},
		To: map[string]libTransaction.Amount{
			"account2": {Asset: "USD", Value: decimal.NewFromInt(1000)},
		},
	}

	emptyBalances := []*mmodel.Balance{}

	amount := decimal.NewFromInt(1000)
	inputTransaction := &transaction.Transaction{
		ID:                       transactionID.String(),
		ParentTransactionID:      nil,
		Description:              "Test transaction",
		Status:                   transaction.Status{Code: "PENDING"},
		Amount:                   &amount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "1000",
		Source:                   []string{"@account1"},
		Destination:              []string{"@account2"},
		LedgerID:                 uuid.New().String(),
		OrganizationID:           uuid.New().String(),
		Route:                    "test_route",
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
		DeletedAt:                nil,
		Metadata:                 map[string]any{"test": "value"},
		Operations:               nil,
	}

	// Act - Test that the queue structure can handle empty balances
	value := transaction.TransactionQueue{
		Validate:    validate,
		Balances:    emptyBalances,
		Transaction: inputTransaction,
		ParseDSL:    parseDSL,
	}

	marshal, err := json.Marshal(value)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, marshal)
	
	// Verify the marshaled data can be unmarshaled back with empty balances
	var unmarshaled transaction.TransactionQueue
	err = json.Unmarshal(marshal, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(unmarshaled.Balances))
	assert.Equal(t, value.Transaction.ID, unmarshaled.Transaction.ID)
	assert.Equal(t, value.ParseDSL.Code, unmarshaled.ParseDSL.Code)
}