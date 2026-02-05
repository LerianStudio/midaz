//go:build integration || chaos

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CreateBalanceOperation creates a BalanceOperation for testing purposes.
// Uses default available balance of 1000 and account type "deposit".
func CreateBalanceOperation(organizationID, ledgerID uuid.UUID, alias, assetCode, operation string, amount decimal.Decimal) mmodel.BalanceOperation {
	return CreateBalanceOperationWithAvailable(organizationID, ledgerID, alias, assetCode, operation, amount, decimal.NewFromInt(1000), "deposit")
}

// CreateBalanceOperationWithAvailable creates a BalanceOperation with custom available balance and account type.
func CreateBalanceOperationWithAvailable(organizationID, ledgerID uuid.UUID, alias, assetCode, operation string, amount, available decimal.Decimal, accountType string) mmodel.BalanceOperation {
	return CreateBalanceOperationWithOnHold(organizationID, ledgerID, alias, assetCode, operation, amount, available, decimal.Zero, accountType)
}

// CreateBalanceOperationWithOnHold creates a BalanceOperation with custom available, onHold balance and account type.
// Used for testing PENDING transaction flows where OnHold balance is significant.
func CreateBalanceOperationWithOnHold(organizationID, ledgerID uuid.UUID, alias, assetCode, operation string, amount, available, onHold decimal.Decimal, accountType string) mmodel.BalanceOperation {
	balanceID := libCommons.GenerateUUIDv7().String()
	accountID := libCommons.GenerateUUIDv7().String()
	balanceKey := "default"

	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balanceKey)

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             balanceID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID,
			Alias:          alias,
			Key:            balanceKey,
			AssetCode:      assetCode,
			Available:      available,
			OnHold:         onHold,
			Version:        1,
			AccountType:    accountType,
			AllowSending:   true,
			AllowReceiving: true,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		Alias: alias,
		Amount: pkgTransaction.Amount{
			Asset:     assetCode,
			Value:     amount,
			Operation: operation,
		},
		InternalKey: internalKey,
	}
}

// AssertInsufficientFundsError verifies the error is an UnprocessableOperationError with code "0018".
func AssertInsufficientFundsError(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err, "expected an error")

	var unprocessableErr pkg.UnprocessableOperationError
	if assert.ErrorAs(t, err, &unprocessableErr, "error should be UnprocessableOperationError") {
		assert.Equal(t, constant.ErrInsufficientFunds.Error(), unprocessableErr.Code, "error code should be 0018")
	}
}
