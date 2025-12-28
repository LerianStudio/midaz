package query

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestValidateAccountRules_RouteLookup_Postcondition(t *testing.T) {
	// This test verifies the assertion is hit when route lookup succeeds
	// but the cache rule is unexpectedly nil (shouldn't happen in practice)
	ctx := context.Background()

	// Create cache with nil account rule (edge case)
	cache := mmodel.TransactionRouteCache{
		Source: map[string]mmodel.OperationRouteCache{
			"route-1": {Account: nil}, // Account is nil but route exists
		},
		Destination: map[string]mmodel.OperationRouteCache{},
	}

	validate := &pkgTransaction.Responses{
		From:                map[string]pkgTransaction.Amount{"@alias1": {Value: decimal.NewFromInt(100)}},
		OperationRoutesFrom: map[string]string{"@alias1": "route-1"},
	}

	operations := []mmodel.BalanceOperation{
		{
			Alias: "@alias1",
			Balance: &mmodel.Balance{
				AccountType: "deposit",
			},
		},
	}

	// This should NOT panic - nil Account is valid (means no rules to check)
	require.NotPanics(t, func() {
		_ = validateAccountRules(ctx, cache, validate, operations)
	})
}
