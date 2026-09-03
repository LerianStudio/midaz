// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accountTypeBalance builds the balance an operation is generated from, with the
// observed account's type on it.
//
// The alias never carries constant.DefaultExternalAccountAliasPrefix, including
// for the external case: mmodel.Account.Alias is validated with
// prohibitedexternalaccountprefix, so a client-created external account has
// type "external" and an alias without the prefix. Only the per-asset account
// Midaz creates for itself has both.
func accountTypeBalance(accountType string) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             "balance-1",
		OrganizationID: "org-1",
		LedgerID:       "ledger-1",
		AccountID:      "account-1",
		Alias:          "@treasury_settlement",
		Key:            "default",
		AccountType:    accountType,
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.Zero,
		Version:        1,
	}
}

func accountTypeOperationLeg(opType, direction string) (mtransaction.FromTo, mtransaction.Amount) {
	return mtransaction.FromTo{
			AccountAlias: "@treasury_settlement",
			IsFrom:       direction == constant.DirectionDebit,
		}, mtransaction.Amount{
			Asset:     "BRL",
			Value:     decimal.NewFromInt(100),
			Operation: opType,
			Direction: direction,
		}
}

func accountTypeAfter() mtransaction.Balance {
	return mtransaction.Balance{
		Available: decimal.NewFromInt(900),
		OnHold:    decimal.Zero,
		Version:   2,
	}
}

// accountTypeDate is the fixed transaction date every fixture below is built
// with — nothing here depends on the wall clock.
var accountTypeDate = time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

func accountTypeInput() mtransaction.Transaction {
	return mtransaction.Transaction{
		Description: "account type fixture",
		Send:        mtransaction.Send{Asset: "BRL"},
	}
}

// TestBuildStandardOp_CarriesAccountTypeFromBalance locks the standard
// single-operation builder to the account type on the balance it builds from.
// The balance is already in hand here, so the type costs no extra read.
func TestBuildStandardOp_CarriesAccountTypeFromBalance(t *testing.T) {
	uc := &UseCase{}

	tests := []string{"deposit", constant.ExternalAccountType, "creditCard"}

	for _, accountType := range tests {
		t.Run(accountType, func(t *testing.T) {
			ft, amt := accountTypeOperationLeg(constant.DEBIT, constant.DirectionDebit)

			op, err := uc.buildStandardOp(
				accountTypeBalance(accountType), ft, amt, accountTypeAfter(),
				transaction.Transaction{ID: "txn-1"}, accountTypeInput(), accountTypeDate, false,
			)

			require.NoError(t, err)
			require.NotNil(t, op)
			assert.Equal(t, accountType, op.AccountType,
				"operation must carry the observed account's type")
		})
	}
}

// TestBuildStandardOp_AccountTypeIsNotTheOperationType pins the two apart.
// Operation.Type is the DEBIT/CREDIT movement; AccountType is the account's
// own type. Wiring either onto the other satisfies neither.
func TestBuildStandardOp_AccountTypeIsNotTheOperationType(t *testing.T) {
	uc := &UseCase{}

	ft, amt := accountTypeOperationLeg(constant.DEBIT, constant.DirectionDebit)

	op, err := uc.buildStandardOp(
		accountTypeBalance(constant.ExternalAccountType), ft, amt, accountTypeAfter(),
		transaction.Transaction{ID: "txn-1"}, accountTypeInput(), accountTypeDate, false,
	)

	require.NoError(t, err)
	require.NotNil(t, op)
	assert.Equal(t, constant.DEBIT, op.Type, "Type stays the ledger movement")
	assert.Equal(t, constant.ExternalAccountType, op.AccountType, "AccountType is the account's type")
}

// TestBuildDoubleEntryOps_CarryAccountTypeFromBalance covers the two
// double-entry builders, which construct their operations from their own
// struct literals. Every leg they emit — including the ON_HOLD and RELEASE
// companions — must carry the account type, or a consumer counting accounts
// sees a leg it cannot classify.
func TestBuildDoubleEntryOps_CarryAccountTypeFromBalance(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	tests := []struct {
		name  string
		build func(*mmodel.Balance, mtransaction.FromTo, mtransaction.Amount) ([]*operation.Operation, error)
	}{
		{
			name: "pending",
			build: func(blc *mmodel.Balance, ft mtransaction.FromTo, amt mtransaction.Amount) ([]*operation.Operation, error) {
				return uc.buildDoubleEntryPendingOps(
					ctx, blc, ft, amt, accountTypeAfter(),
					transaction.Transaction{ID: "txn-1"}, accountTypeInput(), accountTypeDate, false,
				)
			},
		},
		{
			name: "canceled",
			build: func(blc *mmodel.Balance, ft mtransaction.FromTo, amt mtransaction.Amount) ([]*operation.Operation, error) {
				return uc.buildDoubleEntryCanceledOps(
					ctx, blc, ft, amt, accountTypeAfter(),
					transaction.Transaction{ID: "txn-1"}, accountTypeInput(), accountTypeDate, false,
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft, amt := accountTypeOperationLeg(constant.DEBIT, constant.DirectionDebit)

			ops, err := tt.build(accountTypeBalance(constant.ExternalAccountType), ft, amt)

			require.NoError(t, err)
			require.Len(t, ops, 2, "double-entry builders emit both legs")

			for i, op := range ops {
				assert.Equalf(t, constant.ExternalAccountType, op.AccountType,
					"leg %d (%s) must carry the account type", i, op.Type)
			}
		})
	}
}
