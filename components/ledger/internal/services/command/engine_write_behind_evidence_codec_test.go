// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexedEngineEvidenceFixture encodes the index, envelope, and receipt the
// engine stores for one applied transaction whose frozen plan is payload.
func indexedEngineEvidenceFixture(t *testing.T, payload TransactionCompletionPlan) (index, envelope, receipt []byte) {
	t.Helper()

	_, result := recoveryContractFixture(t)
	record := recoveryContractEnvelope(t, payload, result)

	envelope, err := EncodeTransactionWriteBehindEnvelope(TransactionWriteBehindEnvelope{
		FormatVersion: TransactionWriteBehindFormatVersion, ApplicationState: TransactionApplicationConfirmed,
		ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityPending,
		Record: record, Dependencies: []TransactionEvidenceReference{},
	})
	require.NoError(t, err)

	recoveryField := payload.TransactionID.String() + ":" + payload.ExecutionID.String()
	index, err = EncodeTransactionEvidenceIndex(TransactionEvidenceIndex{
		FormatVersion: TransactionEvidenceIndexFormatVersion, TenantID: payload.TenantID,
		OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID, TransactionID: payload.TransactionID,
		ExecutionID: payload.ExecutionID, Action: payload.Action, ApplicationState: TransactionApplicationConfirmed,
		ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityPending,
		RecoveryField: recoveryField, ReceiptField: payload.ExecutionID.String(),
		Dependencies: []TransactionEvidenceReference{},
	})
	require.NoError(t, err)

	receipt, err = json.Marshal(map[string]any{
		"formatVersion": 1, "tenantId": payload.TenantID, "organizationId": payload.OrganizationID, "ledgerId": payload.LedgerID,
		"executionId": payload.ExecutionID, "intentFingerprint": payload.IntentFingerprint,
		"response": `{"protocolVersion":1,"movements":[{}],"final":[{}]}`,
		"protection": map[string]any{
			"formatVersion": 2, "transactions": []uuid.UUID{payload.TransactionID},
			"recoveryFields": []string{recoveryField}, "indexFields": []uuid.UUID{payload.TransactionID},
			"acknowledged": map[string]bool{}, "terminalCompletedAtMs": map[string]int64{},
		},
	})
	require.NoError(t, err)

	return index, envelope, receipt
}

func TestEngineWriteBehindEvidenceCodec_DecodesTheExecutionMembers(t *testing.T) {
	codec := EngineWriteBehindEvidenceCodec{}

	t.Run("grouped evidence returns every member of the execution in order", func(t *testing.T) {
		payload := groupedCompletionPlanFixture(t)
		ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
		index, envelope, receipt := indexedEngineEvidenceFixture(t, payload)

		members, found, err := codec.DecodeEngineTransactionExecutionMembers(
			ctx, index, envelope, receipt, payload.OrganizationID, payload.LedgerID, payload.TransactionID,
		)
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, []EngineExecutionMember{
			{TransactionID: payload.TransactionID, OrganizationID: payload.OrganizationID, LedgerID: payload.LedgerID},
			{TransactionID: completionMembersForeignTxID, OrganizationID: completionMembersForeignOrg, LedgerID: completionMembersForeignLdg},
		}, members)
	})

	t.Run("evidence without a manifest reports none", func(t *testing.T) {
		for name, payload := range map[string]TransactionCompletionPlan{
			"ungrouped": func() TransactionCompletionPlan { plan, _ := recoveryContractFixture(t); return plan }(),
			"grouped without manifest": func() TransactionCompletionPlan {
				plan := groupedCompletionPlanFixture(t)
				plan.ExecutionMembers = nil

				return plan
			}(),
		} {
			t.Run(name, func(t *testing.T) {
				ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
				index, envelope, receipt := indexedEngineEvidenceFixture(t, payload)

				members, found, err := codec.DecodeEngineTransactionExecutionMembers(
					ctx, index, envelope, receipt, payload.OrganizationID, payload.LedgerID, payload.TransactionID,
				)
				require.NoError(t, err)
				assert.False(t, found)
				assert.Nil(t, members)
			})
		}
	})

	t.Run("evidence of another scope fails closed", func(t *testing.T) {
		payload := groupedCompletionPlanFixture(t)
		ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
		index, envelope, receipt := indexedEngineEvidenceFixture(t, payload)

		_, _, err := codec.DecodeEngineTransactionExecutionMembers(
			ctx, index, envelope, receipt, payload.OrganizationID, completionMembersForeignLdg, payload.TransactionID,
		)
		require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)

		_, _, err = codec.DecodeEngineTransactionExecutionMembers(
			tmcore.ContextWithTenantID(context.Background(), "another-tenant"),
			index, envelope, receipt, payload.OrganizationID, payload.LedgerID, payload.TransactionID,
		)
		require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	})

	t.Run("a malformed manifest fails closed", func(t *testing.T) {
		payload := groupedCompletionPlanFixture(t)
		ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
		index, envelope, receipt := indexedEngineEvidenceFixture(t, payload)

		duplicated := strings.Replace(string(envelope), completionMembersForeignTxID.String(), payload.TransactionID.String(), 1)
		_, _, err := codec.DecodeEngineTransactionExecutionMembers(
			ctx, index, []byte(duplicated), receipt, payload.OrganizationID, payload.LedgerID, payload.TransactionID,
		)
		require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	})

	t.Run("a receipt that does not protect the index fails closed", func(t *testing.T) {
		payload := groupedCompletionPlanFixture(t)
		ctx := tmcore.ContextWithTenantID(context.Background(), payload.TenantID)
		index, envelope, receipt := indexedEngineEvidenceFixture(t, payload)

		unprotected := strings.ReplaceAll(string(receipt), payload.TransactionID.String(), completionMembersForeignTxID.String())
		_, _, err := codec.DecodeEngineTransactionExecutionMembers(
			ctx, index, envelope, []byte(unprotected), payload.OrganizationID, payload.LedgerID, payload.TransactionID,
		)
		require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	})
}
