// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func recoveryInventoryEnvelope(t *testing.T, action string) []byte {
	t.Helper()
	payload, result := recoveryContractFixture(t)
	payload.Action = action
	var err error
	payload.IntentFingerprint, err = ComputeBalanceEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	envelope := recoveryContractEnvelope(t, payload, result)
	raw, err := EncodeTransactionCompletionRecord(envelope)
	require.NoError(t, err)
	return raw
}

func recoveryInventoryLegacy(t *testing.T, action string) []byte {
	t.Helper()
	record := mmodel.TransactionRedisQueue{
		TransactionID:  uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		OrganizationID: uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		LedgerID:       uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		Action:         action,
		TransactionInput: mtransaction.Transaction{Send: mtransaction.Send{Source: mtransaction.Source{From: []mtransaction.FromTo{{Amount: &mtransaction.Amount{
			Value: decimal.NewFromInt(12), OverdraftAmount: decimal.NewFromInt(7),
		}}}}}},
	}
	raw, err := json.Marshal(record)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"overdraftAmount":"7"`)
	return raw
}

func recoveryInventoryReceiptBytes(t *testing.T, tenantID, organizationID, ledgerID, executionID string, protected bool) []byte {
	t.Helper()

	receipt := recoveryInventoryReceipt{
		FormatVersion: 1, TenantID: tenantID, OrganizationID: organizationID, LedgerID: ledgerID,
		ExecutionID: executionID, IntentFingerprint: "fingerprint", Response: "{}",
	}
	if protected {
		transactionID := "99999999-9999-4999-8999-999999999999"
		receipt.Protection = &recoveryReceiptProtection{
			FormatVersion: 1, RetentionSeconds: 300,
			Transactions: []string{transactionID}, RecoveryFields: []string{transactionID + ":" + executionID},
			Acknowledged: map[string]bool{}, TerminalCompletedAtMS: map[string]int64{},
		}
	}

	raw, err := json.Marshal(receipt)
	require.NoError(t, err)

	return raw
}

func TestBuildRecoveryInventoryPageClassifiesFormatsAndActions(t *testing.T) {
	tenantID := "tenant-a"
	organizationID := "33333333-3333-4333-8333-333333333333"
	ledgerID := "44444444-4444-4444-8444-444444444444"
	executionID := "88888888-8888-4888-8888-888888888888"
	unprotectedReceipt := recoveryInventoryReceiptBytes(t, tenantID, organizationID, ledgerID, executionID, false)
	protectedReceipt := recoveryInventoryReceiptBytes(t, tenantID, organizationID, ledgerID, "77777777-7777-4777-8777-777777777777", true)
	coordinator := []byte(`{"formatVersion":1,"executions":{"` + executionID + `":0}}`)
	cleanup := []byte(organizationID + ":" + ledgerID + ":" + executionID)
	legacy := recoveryInventoryLegacy(t, constant.ActionCancel)
	legacyBefore := append([]byte(nil), legacy...)

	entries := []RecoveryInventoryEntry{
		{Kind: RecoveryArtifactReceipt, Key: "receipt-secret-id", Raw: unprotectedReceipt},
		{Kind: RecoveryArtifactReceipt, Key: "protected-receipt-secret-id", Raw: protectedReceipt},
		{Kind: RecoveryArtifactBackupQueue, Key: "backup-revert", Raw: recoveryInventoryEnvelope(t, constant.ActionRevert)},
		{Kind: RecoveryArtifactCleanupSchedule, Key: "cleanup-member", Raw: cleanup},
		{Kind: RecoveryArtifactProtectionCoordinator, Key: "coordinator-secret-id", Raw: coordinator},
		{Kind: RecoveryArtifactGuard, Key: "guard-secret-id", Raw: []byte("APPROVED")},
		{Kind: RecoveryArtifactQuarantine, Key: "quarantine-secret-id", Raw: legacy},
		{Kind: RecoveryArtifactPendingTransaction, Key: "pending-create", Raw: recoveryInventoryLegacy(t, constant.ActionDirect)},
		{Kind: RecoveryArtifactBackupQueue, Key: "backup-commit", Raw: recoveryInventoryEnvelope(t, constant.ActionCommit)},
		{Kind: RecoveryArtifactBackupQueue, Key: "backup-cancel", Raw: recoveryInventoryEnvelope(t, constant.ActionCancel)},
	}

	page, err := BuildRecoveryInventoryPage(tenantID, "cursor-2", false, entries)
	require.NoError(t, err)
	require.Equal(t, tenantID, page.TenantID)
	require.False(t, page.Complete)
	require.Equal(t, "cursor-2", page.NextCursor)
	require.Equal(t, len(entries), page.Scanned)
	require.Empty(t, page.Issues)
	require.Equal(t, []RecoveryInventoryBucket{
		{Kind: RecoveryArtifactBackupQueue, Format: RecoveryFormatEngineEnvelopeV2, Action: RecoveryActionCancel, Count: 1},
		{Kind: RecoveryArtifactBackupQueue, Format: RecoveryFormatEngineEnvelopeV2, Action: RecoveryActionCommit, Count: 1},
		{Kind: RecoveryArtifactBackupQueue, Format: RecoveryFormatEngineEnvelopeV2, Action: RecoveryActionRevert, Count: 1},
		{Kind: RecoveryArtifactCleanupSchedule, Format: RecoveryFormatCleanupMemberV1, Action: RecoveryActionUnknown, Count: 1},
		{Kind: RecoveryArtifactGuard, Format: RecoveryFormatGuardToken, Action: RecoveryActionUnknown, Count: 1},
		{Kind: RecoveryArtifactPendingTransaction, Format: RecoveryFormatLegacy, Action: RecoveryActionCreate, Count: 1},
		{Kind: RecoveryArtifactProtectionCoordinator, Format: RecoveryFormatCoordinatorV1, Action: RecoveryActionUnknown, Count: 1},
		{Kind: RecoveryArtifactQuarantine, Format: RecoveryFormatLegacy, Action: RecoveryActionCancel, Count: 1},
		{Kind: RecoveryArtifactReceipt, Format: RecoveryFormatReceiptV1Protected, Action: RecoveryActionUnknown, Count: 1},
		{Kind: RecoveryArtifactReceipt, Format: RecoveryFormatReceiptV1Unprotected, Action: RecoveryActionUnknown, Count: 1},
	}, page.Buckets)
	require.Equal(t, legacyBefore, legacy, "classification must preserve the raw overdraft-bearing payload")
}

func TestBuildRecoveryInventoryPageReportsOnlyHashedIssueKeys(t *testing.T) {
	page, err := BuildRecoveryInventoryPage("tenant-a", "", true, []RecoveryInventoryEntry{
		{Kind: RecoveryArtifactBackupQueue, Key: "transaction:private-id", Raw: []byte(`{"formatVersion":3}`)},
		{Kind: RecoveryArtifactGuard, Key: "guard:private-id", Raw: nil},
		{Kind: RecoveryArtifactReceipt, Key: "receipt:private-id", Raw: []byte(`{"formatVersion":1,"tenantId":"tenant-b"}`)},
	})
	require.NoError(t, err)
	require.True(t, page.Complete)
	require.Len(t, page.Issues, 3)
	for _, issue := range page.Issues {
		require.Len(t, issue.KeyDigest, 16)
		require.NotContains(t, issue.KeyDigest, "private-id")
	}
	require.Equal(t, []RecoveryInventoryBucket{
		{Kind: RecoveryArtifactBackupQueue, Format: RecoveryFormatInvalid, Action: RecoveryActionUnknown, Count: 1},
		{Kind: RecoveryArtifactGuard, Format: RecoveryFormatInvalid, Action: RecoveryActionUnknown, Count: 1},
		{Kind: RecoveryArtifactReceipt, Format: RecoveryFormatInvalid, Action: RecoveryActionUnknown, Count: 1},
	}, page.Buckets)
}

func TestBuildRecoveryInventoryPageRejectsUnsafePageMetadata(t *testing.T) {
	_, err := BuildRecoveryInventoryPage("", "", false, nil)
	require.ErrorContains(t, err, "tenant scope")

	_, err = BuildRecoveryInventoryPage("tenant-a", "next", true, nil)
	require.ErrorContains(t, err, "cannot have a next cursor")

	_, err = BuildRecoveryInventoryPage("tenant-a", "", false, make([]RecoveryInventoryEntry, MaxRecoveryInventoryPageSize+1))
	require.ErrorContains(t, err, "exceeds")

	_, err = BuildRecoveryInventoryPage("tenant-a", "", false, []RecoveryInventoryEntry{{Kind: RecoveryArtifactGuard}})
	require.ErrorContains(t, err, "empty key")

	duplicate := RecoveryInventoryEntry{Kind: RecoveryArtifactGuard, Key: "same", Raw: []byte("PENDING")}
	_, err = BuildRecoveryInventoryPage("tenant-a", "", false, []RecoveryInventoryEntry{duplicate, duplicate})
	require.ErrorContains(t, err, "duplicate kind and key")
}

func TestRecoveryInventoryRejectsAmbiguousAndUnsupportedArtifacts(t *testing.T) {
	tests := []RecoveryInventoryEntry{
		{Kind: RecoveryArtifactBackupQueue, Key: "duplicate", Raw: []byte(`{"transaction_id":"11111111-1111-4111-8111-111111111111","organization_id":"22222222-2222-4222-8222-222222222222","ledger_id":"33333333-3333-4333-8333-333333333333","action":"direct","action":"cancel"}`)},
		{Kind: RecoveryArtifactBackupQueue, Key: "case", Raw: []byte(`{"FormatVersion":2}`)},
		{Kind: RecoveryArtifactCleanupSchedule, Key: "cleanup", Raw: []byte("not:a:uuid")},
		{Kind: RecoveryArtifactKind("unknown"), Key: "kind", Raw: []byte("value")},
	}

	page, err := BuildRecoveryInventoryPage("tenant-a", "", false, tests)
	require.NoError(t, err)
	require.Len(t, page.Issues, len(tests))
	for _, issue := range page.Issues {
		require.NotEmpty(t, strings.TrimSpace(issue.Error))
	}
}

func TestRecoveryInventoryRejectsMalformedProtectionProofs(t *testing.T) {
	tenantID := "tenant-a"
	organizationID := "33333333-3333-4333-8333-333333333333"
	ledgerID := "44444444-4444-4444-8444-444444444444"
	executionID := "88888888-8888-4888-8888-888888888888"
	transactionID := "99999999-9999-4999-8999-999999999999"

	tests := []RecoveryInventoryEntry{
		{
			Kind: RecoveryArtifactReceipt, Key: "receipt-retention",
			Raw: []byte(`{"formatVersion":1,"tenantId":"` + tenantID + `","organizationId":"` + organizationID + `","ledgerId":"` + ledgerID + `","executionId":"` + executionID + `","intentFingerprint":"fingerprint","response":"{}","protection":{"formatVersion":1,"retentionSeconds":0,"transactions":["` + transactionID + `"],"recoveryFields":["` + transactionID + `:` + executionID + `"],"acknowledged":{},"terminalCompletedAtMs":{}}}`),
		},
		{
			Kind: RecoveryArtifactReceipt, Key: "receipt-field",
			Raw: []byte(`{"formatVersion":1,"tenantId":"` + tenantID + `","organizationId":"` + organizationID + `","ledgerId":"` + ledgerID + `","executionId":"` + executionID + `","intentFingerprint":"fingerprint","response":"{}","protection":{"formatVersion":1,"retentionSeconds":300,"transactions":["` + transactionID + `"],"recoveryFields":["wrong"],"acknowledged":{},"terminalCompletedAtMs":{}}}`),
		},
		{
			Kind: RecoveryArtifactReceipt, Key: "receipt-deadline",
			Raw: []byte(`{"formatVersion":1,"tenantId":"` + tenantID + `","organizationId":"` + organizationID + `","ledgerId":"` + ledgerID + `","executionId":"` + executionID + `","intentFingerprint":"fingerprint","response":"{}","protection":{"formatVersion":1,"retentionSeconds":300,"transactions":["` + transactionID + `"],"recoveryFields":["` + transactionID + `:` + executionID + `"],"acknowledged":{"` + transactionID + `":true},"terminalCompletedAtMs":{"` + transactionID + `":1000},"cleanupAfterMs":1001}}`),
		},
		{
			Kind: RecoveryArtifactProtectionCoordinator, Key: "coordinator-id",
			Raw: []byte(`{"formatVersion":1,"executions":{"not-a-uuid":0}}`),
		},
		{
			Kind: RecoveryArtifactProtectionCoordinator, Key: "coordinator-deadline",
			Raw: []byte(`{"formatVersion":1,"executions":{"` + executionID + `":300000},"cleanupAfterMs":299999}`),
		},
	}

	page, err := BuildRecoveryInventoryPage(tenantID, "next", false, tests)
	require.NoError(t, err)
	require.Len(t, page.Issues, len(tests))
	require.Equal(t, []RecoveryInventoryBucket{
		{Kind: RecoveryArtifactProtectionCoordinator, Format: RecoveryFormatInvalid, Action: RecoveryActionUnknown, Count: 2},
		{Kind: RecoveryArtifactReceipt, Format: RecoveryFormatInvalid, Action: RecoveryActionUnknown, Count: 3},
	}, page.Buckets)
}

func TestRecoveryInventoryAcceptsCompletedProtectionProof(t *testing.T) {
	tenantID := "tenant-a"
	organizationID := "33333333-3333-4333-8333-333333333333"
	ledgerID := "44444444-4444-4444-8444-444444444444"
	executionID := "88888888-8888-4888-8888-888888888888"
	transactionID := "99999999-9999-4999-8999-999999999999"
	completedAt := int64(1_000)
	retention := int64(300)

	receipt := recoveryInventoryReceipt{
		FormatVersion: 1, TenantID: tenantID, OrganizationID: organizationID, LedgerID: ledgerID,
		ExecutionID: executionID, IntentFingerprint: "fingerprint", Response: "{}",
		Protection: &recoveryReceiptProtection{
			FormatVersion: 1, RetentionSeconds: retention,
			Transactions: []string{transactionID}, RecoveryFields: []string{transactionID + ":" + executionID},
			Acknowledged: map[string]bool{transactionID: true}, TerminalCompletedAtMS: map[string]int64{transactionID: completedAt},
			CleanupAfterMS: completedAt + retention*1000,
		},
	}
	raw, err := json.Marshal(receipt)
	require.NoError(t, err)

	classification, err := classifyRecoveryReceipt(tenantID, raw)
	require.NoError(t, err)
	require.Equal(t, RecoveryFormatReceiptV1Protected, classification.format)

	coordinator := []byte(`{"formatVersion":1,"executions":{"` + executionID + `":301000},"cleanupAfterMs":301000}`)
	classification, err = classifyRecoveryCoordinator(coordinator)
	require.NoError(t, err)
	require.Equal(t, RecoveryFormatCoordinatorV1, classification.format)
}
