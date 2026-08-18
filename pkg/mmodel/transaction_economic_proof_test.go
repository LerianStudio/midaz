// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestValidateRedisTransactionEconomicEffect_UsesImmutableMovementNotCandidate(t *testing.T) {
	t.Parallel()

	envelope, operation := exactEconomicProofFixture(t)
	require.NoError(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{operation}))

	tests := []struct {
		name   string
		mutate func(*OperationRedis)
	}{
		{name: "amount", mutate: func(op *OperationRedis) { op.AmountValue = op.AmountValue.Add(decimal.NewFromInt(1)) }},
		{name: "direction", mutate: func(op *OperationRedis) { op.Direction = constant.DirectionCredit }},
		{name: "type", mutate: func(op *OperationRedis) { op.Type = constant.CREDIT }},
		{name: "balance identity", mutate: func(op *OperationRedis) { op.BalanceID = uuid.NewString() }},
		{name: "balance before", mutate: func(op *OperationRedis) { op.BalanceAvailable = op.BalanceAvailable.Add(decimal.NewFromInt(1)) }},
		{name: "balance after", mutate: func(op *OperationRedis) { op.BalanceAfterVersion++ }},
		{name: "asset", mutate: func(op *OperationRedis) { op.AssetCode = "EUR" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := operation
			test.mutate(&candidate)
			assert.Error(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{candidate}))
		})
	}
}

func TestValidateRedisTransactionEconomicEffect_PreservesExactLargeDecimals(t *testing.T) {
	t.Parallel()

	envelope, operation := exactEconomicProofFixture(t)
	before := decimal.RequireFromString("900719925474099300000000000000000000.000")
	amount := decimal.RequireFromString("0.000000000000000000000000000000000001")
	after := before.Sub(amount)
	envelope.Balances[0].Available = before
	envelope.BalancesAfter[0].Available = after
	operation.BalanceAvailable = before
	operation.BalanceAfterAvailable = after
	operation.AmountValue = amount
	envelope.TransactionInput.Send.Value = amount
	envelope.Validate.From["@source#default"] = mtransaction.Amount{
		Asset: "USD", Value: amount, Operation: constant.DEBIT, Direction: constant.DirectionDebit,
	}

	require.NoError(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{operation}))
	operation.AmountValue = decimal.RequireFromString("0.000000000000000000000000000000000002")
	assert.Error(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{operation}))
}

func TestValidateRedisTransactionEconomicEffect_BindsSemanticsToImmutableBalanceInput(t *testing.T) {
	t.Parallel()

	envelope, debit := exactEconomicProofFixture(t)
	creditBefore := envelope.Balances[0]
	creditBefore.ID = uuid.NewString()
	creditBefore.AccountID = uuid.NewString()
	creditBefore.Alias = "@destination"
	creditBefore.Available = decimal.NewFromInt(100)
	creditAfter := creditBefore
	creditAfter.Available = decimal.NewFromInt(200)
	creditAfter.Version++
	credit := debit
	credit.ID = uuid.NewString()
	credit.BalanceID = creditBefore.ID
	credit.AccountID = creditBefore.AccountID
	credit.Type = constant.CREDIT
	credit.Direction = constant.DirectionCredit
	credit.BalanceAvailable = creditBefore.Available
	credit.BalanceOnHold = creditBefore.OnHold
	credit.BalanceVersion = creditBefore.Version
	credit.BalanceAfterAvailable = creditAfter.Available
	credit.BalanceAfterOnHold = creditAfter.OnHold
	credit.BalanceAfterVersion = creditAfter.Version
	envelope.Balances = append(envelope.Balances, creditBefore)
	envelope.BalancesAfter = append(envelope.BalancesAfter, creditAfter)
	envelope.Validate.To = map[string]mtransaction.Amount{
		"@destination#default": {
			Asset: "USD", Value: decimal.NewFromInt(100),
			Operation: constant.CREDIT, Direction: constant.DirectionCredit,
		},
	}

	require.NoError(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{debit, credit}))
	truncated := *envelope
	truncated.Balances = append([]BalanceRedis(nil), envelope.Balances[:1]...)
	truncated.BalancesAfter = append([]BalanceRedis(nil), envelope.BalancesAfter[:1]...)
	assert.Error(t, ValidateRedisTransactionEconomicEffect(&truncated, []OperationRedis{debit}),
		"a candidate cannot hide one immutable leg by truncating both its operation and balance snapshots")
	debit.Type, credit.Type = credit.Type, debit.Type
	debit.Direction, credit.Direction = credit.Direction, debit.Direction
	assert.Error(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{debit, credit}),
		"globally valid debit/credit labels cannot be swapped across immutable source and destination balances")
}

func TestValidateRedisTransactionEconomicEffect_PendingOutcomeCoversOnlyHeldSource(t *testing.T) {
	t.Parallel()

	envelope, hold := exactEconomicProofFixture(t)
	envelope.TransactionStatus = constant.PENDING
	envelope.BalancesAfter[0].OnHold = decimal.NewFromInt(100)
	hold.Type = constant.ONHOLD
	hold.BalanceAfterOnHold = decimal.NewFromInt(100)
	envelope.Validate.From = map[string]mtransaction.Amount{
		"@source#default": {
			Asset: "USD", Value: decimal.NewFromInt(100), Operation: constant.ONHOLD,
			TransactionType: constant.PENDING, Direction: constant.DirectionDebit,
		},
	}
	envelope.Validate.To = map[string]mtransaction.Amount{
		"@destination#default": {
			Asset: "USD", Value: decimal.NewFromInt(100), Operation: constant.CREDIT,
			TransactionType: constant.PENDING, Direction: constant.DirectionCredit,
		},
	}

	require.NoError(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{hold}),
		"a hold outcome must prove its source movement without inventing the destination operation reserved for commit")
}

func TestValidateRedisTransactionEconomicEffect_CancelOutcomeCoversOnlyReleasedSource(t *testing.T) {
	t.Parallel()

	envelope, release := exactEconomicProofFixture(t)
	envelope.TransactionStatus = constant.CANCELED
	envelope.Balances[0].Available = decimal.NewFromInt(900)
	envelope.Balances[0].OnHold = decimal.NewFromInt(100)
	envelope.BalancesAfter[0].Available = decimal.NewFromInt(1000)
	envelope.BalancesAfter[0].OnHold = decimal.Zero
	release.Type = constant.RELEASE
	release.Direction = constant.DirectionCredit
	release.BalanceAvailable = decimal.NewFromInt(900)
	release.BalanceOnHold = decimal.NewFromInt(100)
	release.BalanceAfterAvailable = decimal.NewFromInt(1000)
	release.BalanceAfterOnHold = decimal.Zero
	envelope.Validate.From = map[string]mtransaction.Amount{
		"@source#default": {
			Asset: "USD", Value: decimal.NewFromInt(100), Operation: constant.RELEASE,
			TransactionType: constant.CANCELED, Direction: constant.DirectionCredit,
		},
	}
	envelope.Validate.To = map[string]mtransaction.Amount{
		"@destination#default": {
			Asset: "USD", Value: decimal.NewFromInt(100), Operation: constant.CREDIT,
			TransactionType: constant.CANCELED, Direction: constant.DirectionCredit,
		},
	}

	require.NoError(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{release}),
		"cancel must prove the held source release without inventing a destination movement that never occurred")
}

func TestValidateRedisTransactionEconomicEffect_UsesDurableOperationTypeOverride(t *testing.T) {
	t.Parallel()

	envelope, operation := exactEconomicProofFixture(t)
	envelope.EffectModeVersion = TransactionEffectModeVersion
	envelope.EffectMode = TransactionEffectBalanceMutation
	envelope.OperationTypeOverride = constant.BLOCK
	operation.Type = constant.BLOCK
	require.NoError(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{operation}))

	envelope.OperationTypeOverride = ""
	assert.Error(t, ValidateRedisTransactionEconomicEffect(envelope, []OperationRedis{operation}),
		"the typed operation must be backed by the queue-only durable discriminator")
}

func TestValidateRedisTransactionAnnotationEffect_ProvesNoBalanceMutationFromImmutableInput(t *testing.T) {
	t.Parallel()

	envelope, operation := exactEconomicProofFixture(t)
	envelope.Balances = nil
	envelope.BalancesAfter = nil
	envelope.TransactionStatus = constant.NOTED
	envelope.EffectModeVersion = TransactionEffectModeVersion
	envelope.EffectMode = TransactionEffectAnnotationOnly
	operation.BalanceAffected = false
	operation.AmountValue = decimal.Zero
	operation.BalanceAvailable = decimal.Zero
	operation.BalanceOnHold = decimal.Zero
	operation.BalanceVersion = 0
	operation.BalanceAfterAvailable = decimal.Zero
	operation.BalanceAfterOnHold = decimal.Zero
	operation.BalanceAfterVersion = 0

	require.NoError(t, ValidateRedisTransactionAnnotationEffect(envelope, []OperationRedis{operation}))

	tests := []struct {
		name   string
		mutate func(*TransactionRedisQueue, *OperationRedis)
	}{
		{name: "amount", mutate: func(_ *TransactionRedisQueue, op *OperationRedis) {
			op.AmountValue = decimal.NewFromInt(1)
		}},
		{name: "direction", mutate: func(_ *TransactionRedisQueue, op *OperationRedis) { op.Direction = constant.DirectionCredit }},
		{name: "type", mutate: func(_ *TransactionRedisQueue, op *OperationRedis) { op.Type = constant.CREDIT }},
		{name: "balance affected", mutate: func(_ *TransactionRedisQueue, op *OperationRedis) { op.BalanceAffected = true }},
		{name: "balance delta", mutate: func(_ *TransactionRedisQueue, op *OperationRedis) { op.BalanceAfterAvailable = decimal.NewFromInt(1) }},
		{name: "balance version", mutate: func(_ *TransactionRedisQueue, op *OperationRedis) { op.BalanceAfterVersion = 1 }},
		{name: "outcome owner", mutate: func(queue *TransactionRedisQueue, _ *OperationRedis) { queue.AttemptOwner = "malicious-owner" }},
		{name: "outcome", mutate: func(queue *TransactionRedisQueue, _ *OperationRedis) {
			queue.ExpectedOutcome = TransactionOutcomeCommitted
		}},
		{name: "balance evidence", mutate: func(queue *TransactionRedisQueue, _ *OperationRedis) {
			queue.Balances = []BalanceRedis{{ID: uuid.NewString()}}
		}},
		{name: "wrong mode", mutate: func(queue *TransactionRedisQueue, _ *OperationRedis) {
			queue.EffectMode = TransactionEffectBalanceMutation
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateQueue := *envelope
			candidate := operation
			test.mutate(&candidateQueue, &candidate)
			assert.Error(t, ValidateRedisTransactionAnnotationEffect(&candidateQueue, []OperationRedis{candidate}))
		})
	}
}

func TestResolveTransactionEffectMode_ExplicitVersionedCompatibility(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		queue   TransactionRedisQueue
		want    TransactionEffectMode
		wantErr bool
	}{
		{name: "new balance mutation", queue: TransactionRedisQueue{EffectModeVersion: 1, EffectMode: TransactionEffectBalanceMutation}, want: TransactionEffectBalanceMutation},
		{name: "new annotation", queue: TransactionRedisQueue{EffectModeVersion: 1, EffectMode: TransactionEffectAnnotationOnly, TransactionStatus: constant.NOTED}, want: TransactionEffectAnnotationOnly},
		{name: "legacy noted", queue: TransactionRedisQueue{TransactionStatus: constant.NOTED}, want: TransactionEffectAnnotationOnly},
		{name: "legacy movement", queue: TransactionRedisQueue{TransactionStatus: constant.CREATED}, want: TransactionEffectBalanceMutation},
		{name: "partial mode", queue: TransactionRedisQueue{EffectMode: TransactionEffectAnnotationOnly, TransactionStatus: constant.NOTED}, wantErr: true},
		{name: "partial version", queue: TransactionRedisQueue{EffectModeVersion: 1, TransactionStatus: constant.NOTED}, wantErr: true},
		{name: "unknown version", queue: TransactionRedisQueue{EffectModeVersion: 2, EffectMode: TransactionEffectAnnotationOnly, TransactionStatus: constant.NOTED}, wantErr: true},
		{name: "unknown mode", queue: TransactionRedisQueue{EffectModeVersion: 1, EffectMode: "UNKNOWN", TransactionStatus: constant.NOTED}, wantErr: true},
		{name: "annotation mode on created", queue: TransactionRedisQueue{EffectModeVersion: 1, EffectMode: TransactionEffectAnnotationOnly, TransactionStatus: constant.CREATED}, wantErr: true},
		{name: "unknown balance mutation override", queue: TransactionRedisQueue{EffectModeVersion: 1, EffectMode: TransactionEffectBalanceMutation, TransactionStatus: constant.CREATED, OperationTypeOverride: "FOO"}, wantErr: true},
		{name: "annotation override", queue: TransactionRedisQueue{EffectModeVersion: 1, EffectMode: TransactionEffectAnnotationOnly, TransactionStatus: constant.NOTED, OperationTypeOverride: constant.BLOCK}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveTransactionEffectMode(&test.queue)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func exactEconomicProofFixture(t *testing.T) (*TransactionRedisQueue, OperationRedis) {
	t.Helper()

	organizationID := uuid.NewString()
	ledgerID := uuid.NewString()
	transactionID := uuid.NewString()
	balanceID := uuid.NewString()
	accountID := uuid.NewString()
	before := BalanceRedis{
		ID: balanceID, Alias: "@source", Key: constant.DefaultBalanceKey, AccountID: accountID,
		AssetCode: "USD", Available: decimal.NewFromInt(1000), OnHold: decimal.Zero, Version: 7,
		AccountType: "deposit", AllowSending: 1, AllowReceiving: 1, Direction: constant.DirectionCredit,
		OverdraftUsed: "0", OverdraftLimit: "0", BalanceScope: BalanceScopeTransactional,
	}
	after := before
	after.Available = decimal.NewFromInt(900)
	after.Version = 8
	amount := decimal.NewFromInt(100)
	envelope := &TransactionRedisQueue{
		TransactionID: uuid.MustParse(transactionID), OrganizationID: uuid.MustParse(organizationID),
		LedgerID: uuid.MustParse(ledgerID), Balances: []BalanceRedis{before}, BalancesAfter: []BalanceRedis{after},
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{Asset: "USD", Value: amount}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"@source#default": {Asset: "USD", Value: amount, Operation: constant.DEBIT, Direction: constant.DirectionDebit},
		}},
		TransactionStatus: constant.CREATED,
	}
	operation := OperationRedis{
		ID: uuid.NewString(), TransactionID: transactionID, Type: constant.DEBIT, Direction: constant.DirectionDebit,
		AssetCode: "USD", AmountValue: amount, BalanceAvailable: before.Available, BalanceOnHold: before.OnHold,
		BalanceVersion: before.Version, BalanceAfterAvailable: after.Available, BalanceAfterOnHold: after.OnHold,
		BalanceAfterVersion: after.Version, BalanceID: balanceID, BalanceKey: before.Key, AccountID: accountID,
		AccountAlias: before.Alias, OrganizationID: organizationID, LedgerID: ledgerID, BalanceAffected: true,
		Snapshot: OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
	}

	return envelope, operation
}
