// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type enginePreparationReader struct {
	TransactionReader
	load   func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error)
	routes func(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error)
}

func (reader enginePreparationReader) GetBalances(ctx context.Context, orgID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	return reader.load(ctx, orgID, ledgerID, aliases)
}

func (reader enginePreparationReader) GetBalanceEngineBalances(ctx context.Context, orgID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []*mmodel.Balance, error) {
	pool, err := LoadBalanceEngineSnapshotPool(ctx, orgID, ledgerID, aliases, reader.load)
	return pool.ExplicitBalances, pool.Balances, err
}

func (reader enginePreparationReader) ValidateAccountingRules(ctx context.Context, orgID, ledgerID uuid.UUID, operations []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
	return reader.routes(ctx, orgID, ledgerID, operations, validate, action)
}

func enginePreparationFixture(t *testing.T) (balanceEnginePreparationInput, []*mmodel.Balance) {
	t.Helper()
	payload, _ := recoveryContractFixture(t)
	source := translationBalance(payload.OrganizationID, payload.LedgerID, "55555555-5555-4555-8555-555555555555", "@source", "default")
	target := translationBalance(payload.OrganizationID, payload.LedgerID, "66666666-6666-4666-8666-666666666666", "@target", "default")
	companion := translationBalance(payload.OrganizationID, payload.LedgerID, "77777777-7777-4777-8777-777777777777", "@source", "overdraft")
	companion.AccountID = source.AccountID
	companion.Direction = constant.DirectionDebit
	companion.Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}
	for _, balance := range []*mmodel.Balance{source, target, companion} {
		balance.AssetCode = "USD"
		balance.AllowSending, balance.AllowReceiving = true, true
	}
	input := balanceEnginePreparationInput{
		organizationID: payload.OrganizationID, ledgerID: payload.LedgerID,
		translation: BalanceEngineTranslationInput{
			TransactionID: payload.TransactionID, Action: constant.ActionDirect, TransactionStatus: constant.CREATED,
			TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{
				Asset: "USD", Value: decimal.NewFromInt(3),
				Source: mtransaction.Source{From: []mtransaction.FromTo{
					{AccountAlias: "0#@source#default", IsFrom: true}, {AccountAlias: "1#@source#default", IsFrom: true},
				}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "0#@target#default"}}},
			}},
			Validate: &mtransaction.Responses{
				Asset: "USD", Aliases: []string{"@source#default", "@target#default"},
				From: map[string]mtransaction.Amount{
					"0#@source#default": {Value: decimal.NewFromInt(1), Operation: constant.DEBIT, Direction: constant.DirectionDebit},
					"1#@source#default": {Value: decimal.NewFromInt(2), Operation: constant.DEBIT, Direction: constant.DirectionDebit},
				},
				To: map[string]mtransaction.Amount{"0#@target#default": {Value: decimal.NewFromInt(3), Operation: constant.CREDIT, Direction: constant.DirectionCredit}},
			},
		},
	}
	return input, []*mmodel.Balance{source, target, companion}
}

func TestPrepareBalanceEngineTransactionSeparatesLegsFromPool(t *testing.T) {
	input, balances := enginePreparationFixture(t)
	reads, routeCalls := 0, 0
	uc := &UseCase{TransactionReader: enginePreparationReader{
		load: func(ctx context.Context, orgID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			assert.True(t, readrouting.IsPrimaryRead(ctx))
			assert.Equal(t, input.organizationID, orgID)
			assert.Equal(t, input.ledgerID, ledgerID)
			reads++
			if reads == 1 {
				assert.ElementsMatch(t, []string{"@source#default", "@target#default"}, aliases)
				return balances[:2], nil
			}
			return balances[2:], nil
		},
		routes: func(ctx context.Context, _, _ uuid.UUID, ops []mmodel.BalanceOperation, validate *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
			routeCalls++
			assert.True(t, readrouting.IsPrimaryRead(ctx))
			assert.Equal(t, constant.ActionDirect, action)
			assert.Same(t, input.translation.Validate, validate)
			require.Len(t, ops, 3)
			assert.Equal(t, []string{"0#@source#default", "1#@source#default", "0#@target#default"}, []string{ops[0].Alias, ops[1].Alias, ops[2].Alias})
			for _, op := range ops {
				assert.Equal(t, "default", op.Balance.Key)
			}
			return nil, nil
		},
	}}
	prepared, err := uc.prepareBalanceEngineTransaction(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 2, reads)
	assert.Equal(t, 1, routeCalls)
	assert.Len(t, prepared.pool.ExplicitBalances, 2)
	assert.Len(t, prepared.pool.Snapshots, 3)
	require.Len(t, prepared.transaction.Postings, 3)
	assert.Equal(t, accounting.DrawAllowed, prepared.transaction.Postings[0].DrawPolicy)
	assert.Len(t, prepared.projection, 5)
	assert.Nil(t, input.translation.Balances)
}

func TestPrepareBalanceEngineTransactionRejectsBeforeExecution(t *testing.T) {
	for _, scenario := range []string{"canceled", "canceled after routes", "missing reader", "internal target", "missing target", "route failure"} {
		t.Run(scenario, func(t *testing.T) {
			input, balances := enginePreparationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reads, routes := 0, 0
			failure := errors.New("route unavailable")
			uc := &UseCase{TransactionReader: enginePreparationReader{
				load: func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
					reads++
					if reads == 1 {
						return balances[:2], nil
					}
					return balances[2:], nil
				},
				routes: func(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
					routes++
					if scenario == "canceled after routes" {
						cancel()
					}
					if scenario == "route failure" {
						return nil, failure
					}
					return nil, nil
				},
			}}
			switch scenario {
			case "canceled":
				cancel()
			case "missing reader":
				uc.TransactionReader = nil
			case "internal target":
				balances[0].Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}
			case "missing target":
				input.translation.TransactionInput.Send.Source.From[0].AccountAlias = "0#@missing#default"
				input.translation.Validate.From["0#@missing#default"] = input.translation.Validate.From["0#@source#default"]
				delete(input.translation.Validate.From, "0#@source#default")
			}
			_, err := uc.prepareBalanceEngineTransaction(ctx, input)
			require.Error(t, err)
			switch scenario {
			case "canceled":
				assert.ErrorIs(t, err, context.Canceled)
				assert.Zero(t, reads)
			case "canceled after routes":
				assert.ErrorIs(t, err, context.Canceled)
				assert.Equal(t, 1, routes)
			case "missing reader":
				assert.Zero(t, reads)
			case "route failure":
				assert.ErrorIs(t, err, failure)
				assert.Equal(t, 1, routes)
			case "missing target":
				assert.Equal(t, pkg.ValidateBusinessError(constant.ErrAccountIneligibility, "ValidateAccounts"), err)
			}
		})
	}
}

func TestPrepareBalanceEngineTransactionDefersEligibilityToAtomicExecution(t *testing.T) {
	input, balances := enginePreparationFixture(t)
	balances[0].AllowSending = false
	balances[1].AllowReceiving = false

	reads := 0
	uc := &UseCase{TransactionReader: enginePreparationReader{
		load: func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
			reads++
			if reads == 1 {
				return balances[:2], nil
			}
			return balances[2:], nil
		},
		routes: func(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
			return nil, nil
		},
	}}

	prepared, err := uc.prepareBalanceEngineTransaction(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, prepared.transaction.BalanceRequirements, 3)
	assert.Equal(t, accounting.BalancePermissionSend, prepared.transaction.BalanceRequirements[0].Permission)
	assert.Equal(t, accounting.BalancePermissionSend, prepared.transaction.BalanceRequirements[1].Permission)
	assert.Equal(t, accounting.BalancePermissionReceive, prepared.transaction.BalanceRequirements[2].Permission)
}

func TestOrderedBalanceEngineValidationOperationsPreservesStaticHoldEntries(t *testing.T) {
	input, balances := enginePreparationFixture(t)
	input.translation.Action = constant.ActionHold
	input.translation.TransactionStatus = constant.PENDING
	for alias, amount := range input.translation.Validate.From {
		amount.RouteValidationEnabled = true
		amount.TransactionType = constant.PENDING
		amount.Operation = constant.ONHOLD
		input.translation.Validate.From[alias] = amount
	}
	ops, err := orderedBalanceEngineValidationOperations(input.translation, balances[:2])
	require.NoError(t, err)
	require.Len(t, ops, 5)
	assert.Equal(t, []string{constant.DEBIT, constant.ONHOLD, constant.DEBIT, constant.ONHOLD, constant.CREDIT}, []string{
		ops[0].Amount.Operation, ops[1].Amount.Operation, ops[2].Amount.Operation, ops[3].Amount.Operation, ops[4].Amount.Operation,
	})
	assert.Equal(t, "1", ops[0].Amount.Value.String())
	assert.Equal(t, "1", ops[1].Amount.Value.String())
	assert.Equal(t, "2", ops[2].Amount.Value.String())
	assert.Equal(t, "2", ops[3].Amount.Value.String())
	legBalances, err := deduplicateBalances(ops)
	require.NoError(t, err)
	require.Len(t, legBalances, 3)
	require.NoError(t, mtransaction.ValidateBalancesRules(context.Background(), input.translation.TransactionInput, *input.translation.Validate, legBalances, nil))
}

func TestPrepareBalanceEngineCancellationDoesNotRequireDestinationBalance(t *testing.T) {
	input, balances := enginePreparationFixture(t)
	input.translation.Action = constant.ActionCancel
	input.translation.TransactionStatus = constant.CANCELED
	for alias, amount := range input.translation.Validate.From {
		amount.TransactionType = constant.CANCELED
		amount.Operation = constant.RELEASE
		amount.Direction = constant.DirectionCredit
		input.translation.Validate.From[alias] = amount
	}
	reads := 0
	uc := &UseCase{TransactionReader: enginePreparationReader{
		load: func(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
			reads++
			if reads == 1 {
				assert.Equal(t, []string{"@source#default"}, aliases)
				return balances[:1], nil
			}
			assert.Equal(t, []string{"@source#overdraft"}, aliases)
			return balances[2:], nil
		},
		routes: func(_ context.Context, _, _ uuid.UUID, operations []mmodel.BalanceOperation, _ *mtransaction.Responses, action string) (*mmodel.TransactionRouteCache, error) {
			assert.Equal(t, constant.ActionCancel, action)
			require.Len(t, operations, 2)
			assert.Equal(t, "0#@source#default", operations[0].Alias)
			assert.Equal(t, "1#@source#default", operations[1].Alias)
			return nil, nil
		},
	}}
	prepared, err := uc.prepareBalanceEngineTransaction(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 2, reads)
	assert.Len(t, prepared.pool.ExplicitBalances, 1)
	assert.Len(t, prepared.pool.Snapshots, 2)
	require.Len(t, prepared.transaction.Postings, 2)
	for _, posting := range prepared.transaction.Postings {
		assert.Equal(t, accounting.PostingRelease, posting.Type)
		assert.Equal(t, "@source#default", posting.BalanceRef)
	}
	assert.Len(t, input.translation.TransactionInput.Send.Distribute.To, 1)
	assert.Len(t, input.translation.Validate.To, 1)
}
