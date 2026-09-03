// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strconv"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestBuildDoubleEntryPendingOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		balance            *mmodel.Balance
		fromTo             mtransaction.FromTo
		amount             mtransaction.Amount
		balanceAfter       mtransaction.Balance
		tran               transaction.Transaction
		transactionInput   mtransaction.Transaction
		isAnnotation       bool
		expectedOpCount    int
		expectedOp1Type    string
		expectedOp2Type    string
		checkVersionChain  bool
		checkBalanceFields bool
	}{
		{
			name: "generates exactly 2 operations with correct types",
			balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(0),
				Version:        5,
			},
			fromTo: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "test operation",
			},
			amount: mtransaction.Amount{
				Value:                  decimal.NewFromInt(300),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			balanceAfter: mtransaction.Balance{
				Available: decimal.NewFromInt(700),
				OnHold:    decimal.NewFromInt(300),
				Version:   7,
			},
			tran: transaction.Transaction{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
			},
			transactionInput: mtransaction.Transaction{
				Pending: true,
				Send:    mtransaction.Send{Asset: "BRL"},
			},
			isAnnotation:       false,
			expectedOpCount:    2,
			expectedOp1Type:    cn.DEBIT,
			expectedOp2Type:    libConstants.ONHOLD,
			checkVersionChain:  true,
			checkBalanceFields: true,
		},
		{
			name: "annotation mode zeroes all balance fields",
			balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(0),
				Version:        5,
			},
			fromTo: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amount: mtransaction.Amount{
				Value:                  decimal.NewFromInt(200),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			balanceAfter: mtransaction.Balance{
				Available: decimal.NewFromInt(800),
				OnHold:    decimal.NewFromInt(200),
				Version:   7,
			},
			tran: transaction.Transaction{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
			},
			transactionInput: mtransaction.Transaction{
				Pending:     true,
				Description: "annotation test",
				Send:        mtransaction.Send{Asset: "BRL"},
			},
			isAnnotation:    true,
			expectedOpCount: 2,
			expectedOp1Type: cn.DEBIT,
			expectedOp2Type: libConstants.ONHOLD,
		},
		{
			name: "uses transaction description when fromTo description is empty",
			balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(500),
				OnHold:         decimal.NewFromInt(0),
				Version:        1,
			},
			fromTo: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "",
			},
			amount: mtransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			balanceAfter: mtransaction.Balance{
				Available: decimal.NewFromInt(400),
				OnHold:    decimal.NewFromInt(100),
				Version:   3,
			},
			tran: transaction.Transaction{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
			},
			transactionInput: mtransaction.Transaction{
				Pending:     true,
				Description: "fallback description",
				Send:        mtransaction.Send{Asset: "USD"},
			},
			isAnnotation:    false,
			expectedOpCount: 2,
			expectedOp1Type: cn.DEBIT,
			expectedOp2Type: libConstants.ONHOLD,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			uc := &UseCase{}
			transactionDate := time.Now()

			ops, err := uc.buildDoubleEntryPendingOps(
				ctx,
				tt.balance,
				tt.fromTo,
				tt.amount,
				tt.balanceAfter,
				tt.tran,
				tt.transactionInput,
				transactionDate,
				tt.isAnnotation,
			)
			require.NoError(t, err)

			require.Len(t, ops, tt.expectedOpCount, "should generate exactly %d operations", tt.expectedOpCount)

			op1 := ops[0]
			op2 := ops[1]

			// Verify operation types
			assert.Equal(t, tt.expectedOp1Type, op1.Type, "op1 should be DEBIT")
			assert.Equal(t, tt.expectedOp2Type, op2.Type, "op2 should be ON_HOLD")

			// Both ops share the same transaction and balance IDs
			assert.Equal(t, tt.tran.ID, op1.TransactionID)
			assert.Equal(t, tt.tran.ID, op2.TransactionID)
			assert.Equal(t, tt.balance.ID, op1.BalanceID)
			assert.Equal(t, tt.balance.ID, op2.BalanceID)

			// Both ops have same amount value
			assert.True(t, tt.amount.Value.Equal(*op1.Amount.Value), "op1 amount should match input")
			assert.True(t, tt.amount.Value.Equal(*op2.Amount.Value), "op2 amount should match input")

			// Op IDs are different (each is a distinct UUIDv7)
			assert.NotEqual(t, op1.ID, op2.ID, "op1 and op2 should have distinct IDs")

			// BalanceAffected flag
			assert.Equal(t, !tt.isAnnotation, op1.BalanceAffected, "op1 BalanceAffected")
			assert.Equal(t, !tt.isAnnotation, op2.BalanceAffected, "op2 BalanceAffected")

			if tt.checkVersionChain && !tt.isAnnotation {
				// Version chaining: op1 starts at original, ends at original+1
				// op2 starts at original+1, ends at original+2
				originalVersion := tt.balance.Version

				assert.Equal(t, originalVersion, *op1.Balance.Version,
					"op1 balance before should have original version")
				assert.Equal(t, originalVersion+1, *op1.BalanceAfter.Version,
					"op1 balance after should be original+1")
				assert.Equal(t, originalVersion+1, *op2.Balance.Version,
					"op2 balance before should chain from op1 (original+1)")
				assert.Equal(t, originalVersion+2, *op2.BalanceAfter.Version,
					"op2 balance after should be original+2")
			}

			if tt.checkBalanceFields && !tt.isAnnotation {
				// Op1 (DEBIT): only Available changes, OnHold unchanged
				expectedDebitAvailable := tt.balance.Available.Sub(tt.amount.Value)
				assert.True(t, expectedDebitAvailable.Equal(*op1.BalanceAfter.Available),
					"op1 should decrease Available by amount: want %s got %s",
					expectedDebitAvailable.String(), op1.BalanceAfter.Available.String())
				assert.True(t, tt.balance.OnHold.Equal(*op1.BalanceAfter.OnHold),
					"op1 should not change OnHold: want %s got %s",
					tt.balance.OnHold.String(), op1.BalanceAfter.OnHold.String())

				// Op2 (ONHOLD): OnHold increases, Available stays at op1's result
				expectedOnHoldValue := tt.balance.OnHold.Add(tt.amount.Value)
				assert.True(t, expectedDebitAvailable.Equal(*op2.Balance.Available),
					"op2 balance before Available should match op1 after Available")
				assert.True(t, expectedOnHoldValue.Equal(*op2.BalanceAfter.OnHold),
					"op2 should increase OnHold by amount: want %s got %s",
					expectedOnHoldValue.String(), op2.BalanceAfter.OnHold.String())
			}

			if tt.isAnnotation {
				// All balance fields should be zeroed
				zero := decimal.NewFromInt(0)
				zeroVersion := int64(0)

				assert.True(t, zero.Equal(*op1.Balance.Available), "annotation op1 balance Available should be zero")
				assert.True(t, zero.Equal(*op1.Balance.OnHold), "annotation op1 balance OnHold should be zero")
				assert.Equal(t, zeroVersion, *op1.Balance.Version, "annotation op1 balance Version should be zero")
				assert.True(t, zero.Equal(*op1.BalanceAfter.Available), "annotation op1 balanceAfter Available should be zero")
				assert.True(t, zero.Equal(*op1.BalanceAfter.OnHold), "annotation op1 balanceAfter OnHold should be zero")
				assert.Equal(t, zeroVersion, *op1.BalanceAfter.Version, "annotation op1 balanceAfter Version should be zero")

				assert.True(t, zero.Equal(*op2.Balance.Available), "annotation op2 balance Available should be zero")
				assert.True(t, zero.Equal(*op2.Balance.OnHold), "annotation op2 balance OnHold should be zero")
				assert.Equal(t, zeroVersion, *op2.Balance.Version, "annotation op2 balance Version should be zero")
				assert.True(t, zero.Equal(*op2.BalanceAfter.Available), "annotation op2 balanceAfter Available should be zero")
				assert.True(t, zero.Equal(*op2.BalanceAfter.OnHold), "annotation op2 balanceAfter OnHold should be zero")
				assert.Equal(t, zeroVersion, *op2.BalanceAfter.Version, "annotation op2 balanceAfter Version should be zero")
			}

			// Description fallback
			if tt.fromTo.Description != "" {
				assert.Equal(t, tt.fromTo.Description, op1.Description, "should use fromTo description")
				assert.Equal(t, tt.fromTo.Description, op2.Description, "should use fromTo description")
			} else {
				assert.Equal(t, tt.transactionInput.Description, op1.Description, "should fall back to transaction description")
				assert.Equal(t, tt.transactionInput.Description, op2.Description, "should fall back to transaction description")
			}
		})
	}
}

func TestBuildDoubleEntryCanceledOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		balance            *mmodel.Balance
		fromTo             mtransaction.FromTo
		amount             mtransaction.Amount
		balanceAfter       mtransaction.Balance
		tran               transaction.Transaction
		transactionInput   mtransaction.Transaction
		isAnnotation       bool
		expectedOpCount    int
		expectedOp1Type    string
		expectedOp2Type    string
		checkVersionChain  bool
		checkBalanceFields bool
	}{
		{
			name: "generates exactly 2 operations RELEASE+CREDIT with correct types",
			balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(500),
				OnHold:         decimal.NewFromInt(300),
				Version:        7,
			},
			fromTo: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "canceled operation",
			},
			amount: mtransaction.Amount{
				Value:                  decimal.NewFromInt(300),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: mtransaction.Balance{
				Available: decimal.NewFromInt(800),
				OnHold:    decimal.NewFromInt(0),
				Version:   10,
			},
			tran: transaction.Transaction{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
			},
			transactionInput: mtransaction.Transaction{
				Pending: false,
				Send:    mtransaction.Send{Asset: "BRL"},
			},
			isAnnotation:       false,
			expectedOpCount:    2,
			expectedOp1Type:    cn.RELEASE,
			expectedOp2Type:    cn.CREDIT,
			checkVersionChain:  true,
			checkBalanceFields: true,
		},
		{
			name: "annotation mode zeroes all balance fields",
			balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(500),
				OnHold:         decimal.NewFromInt(300),
				Version:        7,
			},
			fromTo: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amount: mtransaction.Amount{
				Value:                  decimal.NewFromInt(300),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: mtransaction.Balance{
				Available: decimal.NewFromInt(800),
				OnHold:    decimal.NewFromInt(0),
				Version:   10,
			},
			tran: transaction.Transaction{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
			},
			transactionInput: mtransaction.Transaction{
				Pending:     false,
				Description: "annotation canceled",
				Send:        mtransaction.Send{Asset: "BRL"},
			},
			isAnnotation:    true,
			expectedOpCount: 2,
			expectedOp1Type: cn.RELEASE,
			expectedOp2Type: cn.CREDIT,
		},
		{
			name: "uses transaction description when fromTo description is empty",
			balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(200),
				Version:        1,
			},
			fromTo: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "",
			},
			amount: mtransaction.Amount{
				Value:                  decimal.NewFromInt(200),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: mtransaction.Balance{
				Available: decimal.NewFromInt(1200),
				OnHold:    decimal.NewFromInt(0),
				Version:   4,
			},
			tran: transaction.Transaction{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
			},
			transactionInput: mtransaction.Transaction{
				Pending:     false,
				Description: "fallback canceled description",
				Send:        mtransaction.Send{Asset: "USD"},
			},
			isAnnotation:    false,
			expectedOpCount: 2,
			expectedOp1Type: cn.RELEASE,
			expectedOp2Type: cn.CREDIT,
		},
		{
			name: "zero amount produces 2 operations with unchanged balances",
			balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(500),
				Version:        5,
			},
			fromTo: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "zero amount test",
			},
			amount: mtransaction.Amount{
				Value:                  decimal.NewFromInt(0),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: mtransaction.Balance{
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.NewFromInt(500),
				Version:   7,
			},
			tran: transaction.Transaction{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
			},
			transactionInput: mtransaction.Transaction{
				Pending: false,
				Send:    mtransaction.Send{Asset: "BRL"},
			},
			isAnnotation:       false,
			expectedOpCount:    2,
			expectedOp1Type:    cn.RELEASE,
			expectedOp2Type:    cn.CREDIT,
			checkVersionChain:  true,
			checkBalanceFields: true,
		},
		{
			name: "version starting at 0 chains correctly",
			balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@source1",
				Key:            "default",
				Available:      decimal.NewFromInt(100),
				OnHold:         decimal.NewFromInt(100),
				Version:        0,
			},
			fromTo: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
				Description:  "version zero test",
			},
			amount: mtransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              libConstants.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			balanceAfter: mtransaction.Balance{
				Available: decimal.NewFromInt(200),
				OnHold:    decimal.NewFromInt(0),
				Version:   2,
			},
			tran: transaction.Transaction{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
				LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
			},
			transactionInput: mtransaction.Transaction{
				Pending: false,
				Send:    mtransaction.Send{Asset: "BRL"},
			},
			isAnnotation:       false,
			expectedOpCount:    2,
			expectedOp1Type:    cn.RELEASE,
			expectedOp2Type:    cn.CREDIT,
			checkVersionChain:  true,
			checkBalanceFields: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			uc := &UseCase{}
			transactionDate := time.Now()

			ops, err := uc.buildDoubleEntryCanceledOps(
				ctx,
				tt.balance,
				tt.fromTo,
				tt.amount,
				tt.balanceAfter,
				tt.tran,
				tt.transactionInput,
				transactionDate,
				tt.isAnnotation,
			)
			require.NoError(t, err)

			require.Len(t, ops, tt.expectedOpCount, "should generate exactly %d operations", tt.expectedOpCount)

			op1 := ops[0]
			op2 := ops[1]

			// Verify operation types
			assert.Equal(t, tt.expectedOp1Type, op1.Type, "op1 should be RELEASE")
			assert.Equal(t, tt.expectedOp2Type, op2.Type, "op2 should be CREDIT")

			// Both ops share the same transaction and balance IDs
			assert.Equal(t, tt.tran.ID, op1.TransactionID)
			assert.Equal(t, tt.tran.ID, op2.TransactionID)
			assert.Equal(t, tt.balance.ID, op1.BalanceID)
			assert.Equal(t, tt.balance.ID, op2.BalanceID)

			// Both ops have same amount value
			assert.True(t, tt.amount.Value.Equal(*op1.Amount.Value), "op1 amount should match input")
			assert.True(t, tt.amount.Value.Equal(*op2.Amount.Value), "op2 amount should match input")

			// Op IDs are different (each is a distinct UUIDv7)
			assert.NotEqual(t, op1.ID, op2.ID, "op1 and op2 should have distinct IDs")

			// BalanceAffected flag
			assert.Equal(t, !tt.isAnnotation, op1.BalanceAffected, "op1 BalanceAffected")
			assert.Equal(t, !tt.isAnnotation, op2.BalanceAffected, "op2 BalanceAffected")

			if tt.checkVersionChain && !tt.isAnnotation {
				// Version chaining: op1 starts at original, ends at original+1
				// op2 starts at original+1 (release version), ends at original+2
				originalVersion := tt.balance.Version

				assert.Equal(t, originalVersion, *op1.Balance.Version,
					"op1 balance before should have original version")
				assert.Equal(t, originalVersion+1, *op1.BalanceAfter.Version,
					"op1 balance after should be original+1")
				assert.Equal(t, originalVersion+1, *op2.Balance.Version,
					"op2 balance before should chain from op1 (original+1)")
				assert.Equal(t, originalVersion+2, *op2.BalanceAfter.Version,
					"op2 balance after should be original+2")
			}

			if tt.checkBalanceFields && !tt.isAnnotation {
				// Op1 (RELEASE): only OnHold changes, Available unchanged
				expectedReleaseOnHold := tt.balance.OnHold.Sub(tt.amount.Value)
				assert.True(t, tt.balance.Available.Equal(*op1.BalanceAfter.Available),
					"op1 should NOT change Available: want %s got %s",
					tt.balance.Available.String(), op1.BalanceAfter.Available.String())
				assert.True(t, expectedReleaseOnHold.Equal(*op1.BalanceAfter.OnHold),
					"op1 should decrease OnHold by amount: want %s got %s",
					expectedReleaseOnHold.String(), op1.BalanceAfter.OnHold.String())

				// Op2 (CREDIT): Available increases, OnHold stays at op1's result
				expectedCreditAvailable := tt.balance.Available.Add(tt.amount.Value)
				assert.True(t, expectedReleaseOnHold.Equal(*op2.BalanceAfter.OnHold),
					"op2 OnHold should remain at op1 result: want %s got %s",
					expectedReleaseOnHold.String(), op2.BalanceAfter.OnHold.String())
				assert.True(t, expectedCreditAvailable.Equal(*op2.BalanceAfter.Available),
					"op2 should increase Available by amount: want %s got %s",
					expectedCreditAvailable.String(), op2.BalanceAfter.Available.String())
			}

			if tt.isAnnotation {
				// All balance fields should be zeroed
				zero := decimal.NewFromInt(0)
				zeroVersion := int64(0)

				assert.True(t, zero.Equal(*op1.Balance.Available), "annotation op1 balance Available should be zero")
				assert.True(t, zero.Equal(*op1.Balance.OnHold), "annotation op1 balance OnHold should be zero")
				assert.Equal(t, zeroVersion, *op1.Balance.Version, "annotation op1 balance Version should be zero")
				assert.True(t, zero.Equal(*op1.BalanceAfter.Available), "annotation op1 balanceAfter Available should be zero")
				assert.True(t, zero.Equal(*op1.BalanceAfter.OnHold), "annotation op1 balanceAfter OnHold should be zero")
				assert.Equal(t, zeroVersion, *op1.BalanceAfter.Version, "annotation op1 balanceAfter Version should be zero")

				assert.True(t, zero.Equal(*op2.Balance.Available), "annotation op2 balance Available should be zero")
				assert.True(t, zero.Equal(*op2.Balance.OnHold), "annotation op2 balance OnHold should be zero")
				assert.Equal(t, zeroVersion, *op2.Balance.Version, "annotation op2 balance Version should be zero")
				assert.True(t, zero.Equal(*op2.BalanceAfter.Available), "annotation op2 balanceAfter Available should be zero")
				assert.True(t, zero.Equal(*op2.BalanceAfter.OnHold), "annotation op2 balanceAfter OnHold should be zero")
				assert.Equal(t, zeroVersion, *op2.BalanceAfter.Version, "annotation op2 balanceAfter Version should be zero")
			}

			// Description fallback
			if tt.fromTo.Description != "" {
				assert.Equal(t, tt.fromTo.Description, op1.Description, "should use fromTo description")
				assert.Equal(t, tt.fromTo.Description, op2.Description, "should use fromTo description")
			} else {
				assert.Equal(t, tt.transactionInput.Description, op1.Description, "should fall back to transaction description")
				assert.Equal(t, tt.transactionInput.Description, op2.Description, "should fall back to transaction description")
			}
		})
	}
}

func TestTryBuildDoubleEntryOps(t *testing.T) {
	t.Parallel()

	baseBalance := &mmodel.Balance{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
		LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
		AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
		Alias:          "@source1",
		Key:            "default",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.NewFromInt(200),
		Version:        5,
	}

	baseTran := transaction.Transaction{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: uuid.Must(libCommons.GenerateUUIDv7()).String(),
		LedgerID:       uuid.Must(libCommons.GenerateUUIDv7()).String(),
	}

	baseBalanceAfter := mtransaction.Balance{
		Available: decimal.NewFromInt(800),
		OnHold:    decimal.NewFromInt(400),
		Version:   7,
	}

	tests := []struct {
		name                   string
		ft                     mtransaction.FromTo
		amt                    mtransaction.Amount
		transactionInput       mtransaction.Transaction
		routeValidationEnabled bool
		processedDoubleEntry   map[string]bool
		fromToIndex            int
		expectedOps            int
		expectedHandled        bool
	}{
		{
			name: "returns (nil, false) when routeValidationEnabled is false",
			ft: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: mtransaction.Amount{
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.ONHOLD,
				TransactionType: cn.PENDING,
			},
			transactionInput: mtransaction.Transaction{
				Pending: true,
				Send:    mtransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: false,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            0,
			expectedHandled:        false,
		},
		{
			name: "returns (nil, false) when IsFrom is false",
			ft: mtransaction.FromTo{
				AccountAlias: "@dest1",
				BalanceKey:   "default",
				IsFrom:       false,
			},
			amt: mtransaction.Amount{
				Value:           decimal.NewFromInt(100),
				Operation:       libConstants.ONHOLD,
				TransactionType: cn.PENDING,
			},
			transactionInput: mtransaction.Transaction{
				Pending: true,
				Send:    mtransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            0,
			expectedHandled:        false,
		},
		{
			name: "returns (nil, true) for already-processed alias (deduplication)",
			ft: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: mtransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			transactionInput: mtransaction.Transaction{
				Pending: true,
				Send:    mtransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   map[string]bool{"@source1#0": true},
			fromToIndex:            0,
			expectedOps:            0,
			expectedHandled:        true,
		},
		{
			name: "returns (nil, false) for non-double-entry operation (DEBIT+CREATED)",
			ft: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: mtransaction.Amount{
				Value:           decimal.NewFromInt(100),
				Operation:       cn.DEBIT,
				TransactionType: cn.CREATED,
			},
			transactionInput: mtransaction.Transaction{
				Send: mtransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            0,
			expectedHandled:        false,
		},
		{
			name: "dispatches to pending path for PENDING+ONHOLD",
			ft: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: mtransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			transactionInput: mtransaction.Transaction{
				Pending: true,
				Send:    mtransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            2,
			expectedHandled:        true,
		},
		{
			name: "dispatches to canceled path for CANCELED+RELEASE",
			ft: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: mtransaction.Amount{
				Value:                  decimal.NewFromInt(100),
				Operation:              cn.RELEASE,
				TransactionType:        cn.CANCELED,
				RouteValidationEnabled: true,
			},
			transactionInput: mtransaction.Transaction{
				Send: mtransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   make(map[string]bool),
			expectedOps:            2,
			expectedHandled:        true,
		},
		{
			name: "allows second entry for same alias with different fromToIndex (transfer+fee)",
			ft: mtransaction.FromTo{
				AccountAlias: "@source1",
				BalanceKey:   "default",
				IsFrom:       true,
			},
			amt: mtransaction.Amount{
				Value:                  decimal.NewFromInt(50),
				Operation:              libConstants.ONHOLD,
				TransactionType:        cn.PENDING,
				RouteValidationEnabled: true,
			},
			transactionInput: mtransaction.Transaction{
				Pending: true,
				Send:    mtransaction.Send{Asset: "USD"},
			},
			routeValidationEnabled: true,
			processedDoubleEntry:   map[string]bool{"@source1#0": true},
			fromToIndex:            1,
			expectedOps:            2,
			expectedHandled:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			uc := &UseCase{}
			transactionDate := time.Now()

			ops, handled, err := uc.tryBuildDoubleEntryOps(
				ctx,
				baseBalance,
				tt.ft,
				tt.amt,
				baseBalanceAfter,
				baseTran,
				tt.transactionInput,
				transactionDate,
				false, // isAnnotation
				tt.routeValidationEnabled,
				tt.processedDoubleEntry,
				tt.fromToIndex,
			)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedHandled, handled, "handled flag mismatch")

			if tt.expectedOps == 0 {
				assert.Nil(t, ops, "expected nil ops")
			} else {
				require.Len(t, ops, tt.expectedOps, "expected %d operations", tt.expectedOps)

				// Verify ops have distinct IDs
				assert.NotEqual(t, ops[0].ID, ops[1].ID, "operations should have distinct IDs")

				// Verify composite key (alias#index) was marked as processed
				dedupKey := baseBalance.Alias + "#" + strconv.Itoa(tt.fromToIndex)
				assert.True(t, tt.processedDoubleEntry[dedupKey],
					"composite key should be marked as processed in the deduplication map")
			}
		})
	}
}
