package command

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// TestMsgpackRoundTripPreservesOperations verifies that msgpack serialization/deserialization
// preserves the Operations field in TransactionQueue.
func TestMsgpackRoundTripPreservesOperations(t *testing.T) {
	// Setup: Create test data
	organizationID := uuid.New().String()
	ledgerID := uuid.New().String()
	transactionID := uuid.New().String()

	// Create operations with different types: CREDIT, DEBIT, ON_HOLD
	amount1 := decimal.NewFromInt(100)
	amount2 := decimal.NewFromInt(200)
	amount3 := decimal.NewFromInt(50)

	available1 := decimal.NewFromInt(1000)
	onHold1 := decimal.NewFromInt(0)
	availableAfter1 := decimal.NewFromInt(900)
	version1 := int64(1)
	versionAfter1 := int64(2)

	available2 := decimal.NewFromInt(500)
	onHold2 := decimal.NewFromInt(0)
	availableAfter2 := decimal.NewFromInt(700)
	version2 := int64(1)
	versionAfter2 := int64(2)

	available3 := decimal.NewFromInt(300)
	onHold3 := decimal.NewFromInt(0)
	availableAfter3 := decimal.NewFromInt(300)
	onHoldAfter3 := decimal.NewFromInt(50)
	version3 := int64(1)
	versionAfter3 := int64(2)

	operation1 := &mmodel.Operation{
		ID:              uuid.New().String(),
		TransactionID:   transactionID,
		Description:     "Debit operation for testing",
		Type:            "DEBIT",
		AssetCode:       "USD",
		ChartOfAccounts: "1000",
		Amount: mmodel.OperationAmount{
			Value: &amount1,
		},
		Balance: mmodel.OperationBalance{
			Available: &available1,
			OnHold:    &onHold1,
			Version:   &version1,
		},
		BalanceAfter: mmodel.OperationBalance{
			Available: &availableAfter1,
			OnHold:    &onHold1,
			Version:   &versionAfter1,
		},
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: nil,
		},
		AccountID:       uuid.New().String(),
		AccountAlias:    "@source_account",
		BalanceKey:      "",
		BalanceID:       uuid.New().String(),
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Route:           "",
		BalanceAffected: true,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		DeletedAt:       nil,
		Metadata:        map[string]any{"key1": "value1", "operation_type": "debit"},
	}

	operation2 := &mmodel.Operation{
		ID:              uuid.New().String(),
		TransactionID:   transactionID,
		Description:     "Credit operation for testing",
		Type:            "CREDIT",
		AssetCode:       "USD",
		ChartOfAccounts: "2000",
		Amount: mmodel.OperationAmount{
			Value: &amount2,
		},
		Balance: mmodel.OperationBalance{
			Available: &available2,
			OnHold:    &onHold2,
			Version:   &version2,
		},
		BalanceAfter: mmodel.OperationBalance{
			Available: &availableAfter2,
			OnHold:    &onHold2,
			Version:   &versionAfter2,
		},
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: nil,
		},
		AccountID:       uuid.New().String(),
		AccountAlias:    "@destination_account",
		BalanceKey:      "",
		BalanceID:       uuid.New().String(),
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Route:           "",
		BalanceAffected: true,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		DeletedAt:       nil,
		Metadata:        map[string]any{"key2": "value2", "operation_type": "credit"},
	}

	operation3 := &mmodel.Operation{
		ID:              uuid.New().String(),
		TransactionID:   transactionID,
		Description:     "On hold operation for testing",
		Type:            "ON_HOLD",
		AssetCode:       "USD",
		ChartOfAccounts: "3000",
		Amount: mmodel.OperationAmount{
			Value: &amount3,
		},
		Balance: mmodel.OperationBalance{
			Available: &available3,
			OnHold:    &onHold3,
			Version:   &version3,
		},
		BalanceAfter: mmodel.OperationBalance{
			Available: &availableAfter3,
			OnHold:    &onHoldAfter3,
			Version:   &versionAfter3,
		},
		Status: mmodel.Status{
			Code:        "PENDING",
			Description: nil,
		},
		AccountID:       uuid.New().String(),
		AccountAlias:    "@hold_account",
		BalanceKey:      "hold-key",
		BalanceID:       uuid.New().String(),
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		Route:           "hold-route",
		BalanceAffected: true,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		DeletedAt:       nil,
		Metadata:        map[string]any{"key3": "value3", "operation_type": "on_hold"},
	}

	// Create the Transaction with Operations
	transactionAmount := decimal.NewFromInt(300)
	tran := &mmodel.Transaction{
		ID:                       transactionID,
		Description:              "Test transaction with operations",
		Status:                   mmodel.Status{Code: "ACTIVE"},
		Amount:                   &transactionAmount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "TEST_GROUP",
		Source:                   []string{"@source_account"},
		Destination:              []string{"@destination_account", "@hold_account"},
		LedgerID:                 ledgerID,
		OrganizationID:           organizationID,
		CreatedAt:                time.Now().UTC(),
		UpdatedAt:                time.Now().UTC(),
		Operations:               []*mmodel.Operation{operation1, operation2, operation3},
		Metadata:                 map[string]any{"transaction_key": "transaction_value"},
	}

	// Create the TransactionQueue
	transactionQueue := mmodel.TransactionQueue{
		Transaction: tran,
		Validate:    nil, // Not needed for this test
		Balances:    nil, // Not needed for this test
		ParseDSL:    nil, // Not needed for this test
	}

	// Marshal the TransactionQueue with msgpack
	marshaledBytes, err := msgpack.Marshal(transactionQueue)
	require.NoError(t, err, "msgpack.Marshal should not fail")
	require.NotEmpty(t, marshaledBytes, "marshaled bytes should not be empty")

	t.Logf("Marshaled TransactionQueue size: %d bytes", len(marshaledBytes))

	// Unmarshal back into a new TransactionQueue
	var unmarshaledQueue mmodel.TransactionQueue
	err = msgpack.Unmarshal(marshaledBytes, &unmarshaledQueue)
	require.NoError(t, err, "msgpack.Unmarshal should not fail")

	// Assert: Transaction is preserved
	require.NotNil(t, unmarshaledQueue.Transaction, "Transaction should not be nil after unmarshal")

	// Assert: Operations count is preserved
	assert.Equal(t, len(tran.Operations), len(unmarshaledQueue.Transaction.Operations),
		"Operations count should be preserved after msgpack round-trip")

	t.Logf("Original operations count: %d", len(tran.Operations))
	t.Logf("Unmarshaled operations count: %d", len(unmarshaledQueue.Transaction.Operations))

	// If operations are empty, the test has identified the issue
	if len(unmarshaledQueue.Transaction.Operations) == 0 {
		t.Error("CRITICAL: Operations field is EMPTY after msgpack round-trip - data loss detected!")
		return
	}

	// Assert: Each operation's data is preserved
	for i, originalOp := range tran.Operations {
		unmarshaledOp := unmarshaledQueue.Transaction.Operations[i]

		assert.Equal(t, originalOp.ID, unmarshaledOp.ID,
			"Operation[%d].ID should be preserved", i)
		assert.Equal(t, originalOp.TransactionID, unmarshaledOp.TransactionID,
			"Operation[%d].TransactionID should be preserved", i)
		assert.Equal(t, originalOp.Type, unmarshaledOp.Type,
			"Operation[%d].Type should be preserved", i)
		assert.Equal(t, originalOp.AssetCode, unmarshaledOp.AssetCode,
			"Operation[%d].AssetCode should be preserved", i)
		assert.Equal(t, originalOp.Description, unmarshaledOp.Description,
			"Operation[%d].Description should be preserved", i)
		assert.Equal(t, originalOp.AccountAlias, unmarshaledOp.AccountAlias,
			"Operation[%d].AccountAlias should be preserved", i)

		// Check Amount
		require.NotNil(t, unmarshaledOp.Amount.Value,
			"Operation[%d].Amount.Value should not be nil", i)
		assert.True(t, originalOp.Amount.Value.Equal(*unmarshaledOp.Amount.Value),
			"Operation[%d].Amount.Value should be preserved (original: %s, unmarshaled: %s)",
			i, originalOp.Amount.Value.String(), unmarshaledOp.Amount.Value.String())

		// Check Balance
		require.NotNil(t, unmarshaledOp.Balance.Available,
			"Operation[%d].Balance.Available should not be nil", i)
		require.NotNil(t, unmarshaledOp.Balance.Version,
			"Operation[%d].Balance.Version should not be nil", i)

		// Check BalanceAfter
		require.NotNil(t, unmarshaledOp.BalanceAfter.Available,
			"Operation[%d].BalanceAfter.Available should not be nil", i)
		require.NotNil(t, unmarshaledOp.BalanceAfter.Version,
			"Operation[%d].BalanceAfter.Version should not be nil", i)

		// Check Metadata
		assert.Equal(t, originalOp.Metadata["operation_type"], unmarshaledOp.Metadata["operation_type"],
			"Operation[%d].Metadata should be preserved", i)

		t.Logf("Operation[%d] verified: ID=%s, Type=%s, Amount=%s",
			i, unmarshaledOp.ID, unmarshaledOp.Type, unmarshaledOp.Amount.Value.String())
	}

	// Additional assertion: Verify transaction-level fields
	assert.Equal(t, tran.ID, unmarshaledQueue.Transaction.ID,
		"Transaction.ID should be preserved")
	assert.Equal(t, tran.Description, unmarshaledQueue.Transaction.Description,
		"Transaction.Description should be preserved")
	assert.Equal(t, tran.AssetCode, unmarshaledQueue.Transaction.AssetCode,
		"Transaction.AssetCode should be preserved")
	assert.Equal(t, tran.OrganizationID, unmarshaledQueue.Transaction.OrganizationID,
		"Transaction.OrganizationID should be preserved")
	assert.Equal(t, tran.LedgerID, unmarshaledQueue.Transaction.LedgerID,
		"Transaction.LedgerID should be preserved")

	t.Log("SUCCESS: Operations field survives msgpack round-trip serialization!")
}
