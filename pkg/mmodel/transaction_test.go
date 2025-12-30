package mmodel

import (
	"fmt"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestTransactionRevert_UnknownOperationType_Panics(t *testing.T) {
	// Arrange: Create transaction with unknown operation type
	amount := decimal.NewFromInt(100)
	transaction := Transaction{
		ID:        "123e4567-e89b-12d3-a456-426614174000",
		AssetCode: "USD",
		Amount:    &amount,
		Operations: []*Operation{
			{
				ID:           "op-1",
				Type:         "UNKNOWN_TYPE", // Invalid type
				AccountAlias: "@account1",
				AssetCode:    "USD",
				Amount:       OperationAmount{Value: &amount},
			},
		},
	}

	// Act & Assert: Should panic for unknown operation type
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for unknown operation type, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "unhandled operation type in TransactionRevert") {
			t.Errorf("Expected panic about unhandled operation type, got: %v", r)
		}
	}()

	transaction.TransactionRevert()
}
