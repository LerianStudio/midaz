//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestIntegration_EngineWriteBehindPublishesIndexedEvidence(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	input, limits := richAdapterExecution(t)
	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
	require.NoError(t, err)

	result, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result)

	keys, err := resolveAdapterKeys(ctx, input.Execution)
	require.NoError(t, err)
	transactionID := input.Execution.Transactions[0].ID
	executionID := input.Execution.ExecutionID

	rawIndex, err := inspector.HGet(ctx, keys.TransactionIndex, transactionID.String()).Bytes()
	require.NoError(t, err)
	index, err := command.DecodeTransactionEvidenceIndex(rawIndex)
	require.NoError(t, err)
	require.Equal(t, transactionID, index.TransactionID)
	require.Equal(t, executionID, index.ExecutionID)
	require.Equal(t, command.TransactionApplicationConfirmed, index.ApplicationState)
	require.Equal(t, command.TransactionReplayReconstructible, index.ReplayState)
	require.Equal(t, command.TransactionDurabilityPending, index.DurabilityState)
	require.Empty(t, index.Dependencies)

	rawEvidence, err := inspector.HGet(ctx, keys.Recovery, index.RecoveryField).Bytes()
	require.NoError(t, err)
	evidence, err := command.DecodeTransactionWriteBehindEnvelope(rawEvidence)
	require.NoError(t, err)
	require.Equal(t, index.ExecutionID, evidence.Record.ExecutionID)
	require.Equal(t, index.TransactionID, evidence.Record.TransactionID)
	require.Empty(t, evidence.Dependencies)

	var receipt struct {
		Protection struct {
			FormatVersion int      `json:"formatVersion"`
			IndexFields   []string `json:"indexFields"`
		} `json:"protection"`
	}
	rawReceipt, err := inspector.HGet(ctx, keys.Receipts, index.ReceiptField).Bytes()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(rawReceipt, &receipt))
	require.Equal(t, 2, receipt.Protection.FormatVersion)
	require.Equal(t, []string{transactionID.String()}, receipt.Protection.IndexFields)

	replayed, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	require.Equal(t, result, replayed)
	require.Equal(t, int64(1), inspector.HLen(ctx, keys.TransactionIndex).Val())
}

func TestIntegration_EngineWriteBehindAdvancesOnlyFromIndexedPredecessor(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	first, limits := richAdapterExecution(t)
	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
	require.NoError(t, err)
	_, err = adapter.Execute(ctx, first)
	require.NoError(t, err)

	second, _ := richAdapterExecution(t)
	second.Execution.ExecutionID = uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	second.Guards[0].ExpectedToken = first.Guards[0].NextToken
	second.Guards[0].NextToken = "executed-twice"
	plan, err := command.DecodeTransactionCompletionPlan(second.CompletionPlans[0].Payload)
	require.NoError(t, err)
	plan.ExecutionID = second.Execution.ExecutionID
	second.CompletionPlans[0].Payload = encodeAdapterRecovery(t, &second, *plan)
	second.CompletionPlans[0].Dependencies = []command.TransactionEvidenceReference{{
		Kind: command.TransactionDependencyPredecessor, TenantID: plan.TenantID,
		OrganizationID: second.Execution.OrganizationID, LedgerID: second.Execution.LedgerID,
		TransactionID: second.Execution.Transactions[0].ID, ExecutionID: first.Execution.ExecutionID,
	}}

	_, err = adapter.Execute(ctx, second)
	require.NoError(t, err)
	keys, err := resolveAdapterKeys(ctx, second.Execution)
	require.NoError(t, err)
	rawIndex, err := inspector.HGet(ctx, keys.TransactionIndex, second.Execution.Transactions[0].ID.String()).Bytes()
	require.NoError(t, err)
	index, err := command.DecodeTransactionEvidenceIndex(rawIndex)
	require.NoError(t, err)
	require.Equal(t, second.Execution.ExecutionID, index.ExecutionID)
	require.Equal(t, second.CompletionPlans[0].Dependencies, index.Dependencies)
	require.True(t, inspector.HExists(ctx, keys.Recovery, first.Execution.Transactions[0].ID.String()+":"+first.Execution.ExecutionID.String()).Val())
	require.True(t, inspector.HExists(ctx, keys.Receipts, first.Execution.ExecutionID.String()).Val())
}

func TestIntegration_EngineWriteBehindAdvancesFromCompletedEvidence(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)
	first, limits := richAdapterExecution(t)
	adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
	require.NoError(t, err)
	_, err = adapter.Execute(ctx, first)
	require.NoError(t, err)

	keys, err := resolveAdapterKeys(ctx, first.Execution)
	require.NoError(t, err)
	field := first.Execution.Transactions[0].ID.String() + ":" + first.Execution.ExecutionID.String()
	rawEnvelope, err := inspector.HGet(ctx, keys.Recovery, field).Result()
	require.NoError(t, err)
	rawIndex, err := inspector.HGet(ctx, keys.TransactionIndex, first.Execution.Transactions[0].ID.String()).Result()
	require.NoError(t, err)
	completedEnvelope := strings.Replace(rawEnvelope, `"durabilityState":"pending"`, `"durabilityState":"complete"`, 1)
	completedIndex := strings.Replace(rawIndex, `"durabilityState":"pending"`, `"durabilityState":"complete"`, 1)
	require.NotEqual(t, rawEnvelope, completedEnvelope)
	require.NotEqual(t, rawIndex, completedIndex)
	require.NoError(t, inspector.HSet(ctx, keys.Evidence, field, completedEnvelope).Err())
	require.NoError(t, inspector.HSet(ctx, keys.TransactionIndex, first.Execution.Transactions[0].ID.String(), completedIndex).Err())
	require.NoError(t, inspector.HDel(ctx, keys.Recovery, field).Err())

	second, _ := richAdapterExecution(t)
	second.Execution.ExecutionID = uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	second.Guards[0].ExpectedToken = first.Guards[0].NextToken
	second.Guards[0].NextToken = "executed-twice"
	plan, err := command.DecodeTransactionCompletionPlan(second.CompletionPlans[0].Payload)
	require.NoError(t, err)
	plan.ExecutionID = second.Execution.ExecutionID
	second.CompletionPlans[0].Payload = encodeAdapterRecovery(t, &second, *plan)
	second.CompletionPlans[0].Dependencies = []command.TransactionEvidenceReference{{
		Kind: command.TransactionDependencyPredecessor, TenantID: plan.TenantID,
		OrganizationID: second.Execution.OrganizationID, LedgerID: second.Execution.LedgerID,
		TransactionID: second.Execution.Transactions[0].ID, ExecutionID: first.Execution.ExecutionID,
	}}

	_, err = adapter.Execute(ctx, second)
	require.NoError(t, err)
	require.True(t, inspector.HExists(ctx, keys.Evidence, field).Val())
	require.False(t, inspector.HExists(ctx, keys.Recovery, field).Val())
}

func TestIntegration_EngineWriteBehindDependencyPreflightRefusesWithoutWrites(t *testing.T) {
	ctx := context.Background()
	inspector, _, _ := newAdapterValkey(t)

	tests := []struct {
		name         string
		dependencies func(command.EngineExecution) []command.TransactionEvidenceReference
		code         string
	}{
		{
			name: "missing evidence",
			dependencies: func(input command.EngineExecution) []command.TransactionEvidenceReference {
				return []command.TransactionEvidenceReference{{Kind: command.TransactionDependencyPredecessor, OrganizationID: input.Execution.OrganizationID, LedgerID: input.Execution.LedgerID, TransactionID: input.Execution.Transactions[0].ID, ExecutionID: uuid.New()}}
			},
			code: "dependency_evidence_missing",
		},
		{
			name: "cross scope",
			dependencies: func(input command.EngineExecution) []command.TransactionEvidenceReference {
				return []command.TransactionEvidenceReference{{Kind: command.TransactionDependencyPredecessor, OrganizationID: uuid.New(), LedgerID: input.Execution.LedgerID, TransactionID: input.Execution.Transactions[0].ID, ExecutionID: uuid.New()}}
			},
			code: "invalid_protocol",
		},
		{
			name: "bounded dependency graph",
			dependencies: func(input command.EngineExecution) []command.TransactionEvidenceReference {
				dependencies := make([]command.TransactionEvidenceReference, 3)
				for i := range dependencies {
					dependencies[i] = command.TransactionEvidenceReference{Kind: command.TransactionDependencyPredecessor, OrganizationID: input.Execution.OrganizationID, LedgerID: input.Execution.LedgerID, TransactionID: input.Execution.Transactions[0].ID, ExecutionID: uuid.New()}
				}
				return dependencies
			},
			code: "invalid_protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, inspector.FlushDB(ctx).Err())
			input, limits := richAdapterExecution(t)
			input.CompletionPlans[0].Dependencies = tt.dependencies(input)
			adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
			require.NoError(t, err)
			keys, err := resolveAdapterKeys(ctx, input.Execution)
			require.NoError(t, err)
			before := captureAdapterState(t, inspector, keys)

			result, err := adapter.Execute(ctx, input)
			require.Nil(t, result)
			assertAdapterTechnical(t, err, tt.code, false)
			require.Equal(t, before, captureAdapterState(t, inspector, keys))
		})
	}
}
