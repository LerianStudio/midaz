// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// MaxRecoveryInventoryPageSize bounds one externally enumerated inventory page.
const MaxRecoveryInventoryPageSize = 1000

// RecoveryArtifactKind identifies one persisted transaction-recovery family.
type RecoveryArtifactKind string

const (
	// RecoveryArtifactPendingTransaction identifies a pending legacy backup payload.
	RecoveryArtifactPendingTransaction    RecoveryArtifactKind = "pending_transaction"
	RecoveryArtifactBackupQueue           RecoveryArtifactKind = "backup_queue"
	RecoveryArtifactEngineRecover         RecoveryArtifactKind = "engine_recover"
	RecoveryArtifactReceipt               RecoveryArtifactKind = "receipt"
	RecoveryArtifactGuard                 RecoveryArtifactKind = "guard"
	RecoveryArtifactProtectionCoordinator RecoveryArtifactKind = "protection_coordinator"
	RecoveryArtifactCleanupSchedule       RecoveryArtifactKind = "cleanup_schedule"
	RecoveryArtifactQuarantine            RecoveryArtifactKind = "quarantine"
)

// RecoveryPersistedFormat identifies the validated wire format of an artifact.
type RecoveryPersistedFormat string

const (
	// RecoveryFormatLegacy identifies an unversioned TransactionRedisQueue payload.
	RecoveryFormatLegacy               RecoveryPersistedFormat = "legacy"
	RecoveryFormatEngineEnvelopeV2     RecoveryPersistedFormat = "engine_envelope_v2"
	RecoveryFormatReceiptV1Unprotected RecoveryPersistedFormat = "receipt_v1_unprotected"
	RecoveryFormatReceiptV1Protected   RecoveryPersistedFormat = "receipt_v1_protected"
	RecoveryFormatGuardToken           RecoveryPersistedFormat = "guard_token"
	RecoveryFormatCoordinatorV1        RecoveryPersistedFormat = "coordinator_v1"
	RecoveryFormatCleanupMemberV1      RecoveryPersistedFormat = "cleanup_member_v1"
	RecoveryFormatInvalid              RecoveryPersistedFormat = "invalid"
)

const (
	// RecoveryActionCreate groups direct and hold creation actions.
	RecoveryActionCreate  = "create"
	RecoveryActionRevert  = "revert"
	RecoveryActionCommit  = "commit"
	RecoveryActionCancel  = "cancel"
	RecoveryActionUnknown = "unknown"
)

// RecoveryInventoryEntry is one record returned by a bounded external read.
// Raw is inspected but never returned by the report.
type RecoveryInventoryEntry struct {
	Kind RecoveryArtifactKind
	Key  string
	Raw  []byte
}

// RecoveryInventoryBucket is one deterministic count by family, format, and action.
type RecoveryInventoryBucket struct {
	Kind   RecoveryArtifactKind
	Format RecoveryPersistedFormat
	Action string
	Count  int
}

// RecoveryInventoryIssue reports malformed input without exposing its physical key.
type RecoveryInventoryIssue struct {
	Kind      RecoveryArtifactKind
	KeyDigest string
	Error     string
}

// RecoveryInventoryPage reports one bounded page. Complete is caller-supplied
// enumeration state, not proof that any artifact family is drained.
type RecoveryInventoryPage struct {
	TenantID   string
	Complete   bool
	NextCursor string
	Scanned    int
	Buckets    []RecoveryInventoryBucket
	Issues     []RecoveryInventoryIssue
}

type recoveryArtifactClassification struct {
	format RecoveryPersistedFormat
	action string
}

// BuildRecoveryInventoryPage classifies one tenant-scoped page without
// changing any record. The external reader owns cursor and completeness truth.
func BuildRecoveryInventoryPage(tenantID, nextCursor string, complete bool, entries []RecoveryInventoryEntry) (RecoveryInventoryPage, error) {
	if tenantID == "" {
		return RecoveryInventoryPage{}, errors.New("recovery inventory requires a tenant scope")
	}

	if len(entries) > MaxRecoveryInventoryPageSize {
		return RecoveryInventoryPage{}, fmt.Errorf("recovery inventory page exceeds %d records", MaxRecoveryInventoryPageSize)
	}

	if complete && nextCursor != "" {
		return RecoveryInventoryPage{}, errors.New("complete recovery inventory page cannot have a next cursor")
	}

	ordered := append([]RecoveryInventoryEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}

		return ordered[i].Key < ordered[j].Key
	})

	page := RecoveryInventoryPage{TenantID: tenantID, Complete: complete, NextCursor: nextCursor, Scanned: len(ordered)}
	buckets := make(map[string]*RecoveryInventoryBucket)

	for index, entry := range ordered {
		if entry.Key == "" {
			return RecoveryInventoryPage{}, errors.New("recovery inventory entry has an empty key")
		}

		if index > 0 && entry.Kind == ordered[index-1].Kind && entry.Key == ordered[index-1].Key {
			return RecoveryInventoryPage{}, errors.New("recovery inventory contains a duplicate kind and key")
		}

		classification, err := classifyRecoveryArtifact(tenantID, entry)
		if err != nil {
			classification = recoveryArtifactClassification{format: RecoveryFormatInvalid, action: RecoveryActionUnknown}

			page.Issues = append(page.Issues, RecoveryInventoryIssue{
				Kind: entry.Kind, KeyDigest: recoveryInventoryKeyDigest(entry.Key), Error: err.Error(),
			})
		}

		bucketKey := string(entry.Kind) + "\x00" + string(classification.format) + "\x00" + classification.action

		bucket := buckets[bucketKey]
		if bucket == nil {
			bucket = &RecoveryInventoryBucket{Kind: entry.Kind, Format: classification.format, Action: classification.action}
			buckets[bucketKey] = bucket
		}

		bucket.Count++
	}

	page.Buckets = make([]RecoveryInventoryBucket, 0, len(buckets))
	for _, bucket := range buckets {
		page.Buckets = append(page.Buckets, *bucket)
	}

	sort.Slice(page.Buckets, func(i, j int) bool {
		if page.Buckets[i].Kind != page.Buckets[j].Kind {
			return page.Buckets[i].Kind < page.Buckets[j].Kind
		}

		if page.Buckets[i].Format != page.Buckets[j].Format {
			return page.Buckets[i].Format < page.Buckets[j].Format
		}

		return page.Buckets[i].Action < page.Buckets[j].Action
	})

	return page, nil
}

func classifyRecoveryArtifact(tenantID string, entry RecoveryInventoryEntry) (recoveryArtifactClassification, error) {
	switch entry.Kind {
	case RecoveryArtifactPendingTransaction, RecoveryArtifactBackupQueue, RecoveryArtifactQuarantine:
		return classifyRecoveryRecord(tenantID, entry.Raw, true)
	case RecoveryArtifactEngineRecover:
		return classifyRecoveryRecord(tenantID, entry.Raw, false)
	case RecoveryArtifactReceipt:
		return classifyRecoveryReceipt(tenantID, entry.Raw)
	case RecoveryArtifactGuard:
		if len(entry.Raw) == 0 {
			return recoveryArtifactClassification{}, errors.New("empty recovery guard")
		}

		return recoveryArtifactClassification{format: RecoveryFormatGuardToken, action: RecoveryActionUnknown}, nil
	case RecoveryArtifactProtectionCoordinator:
		return classifyRecoveryCoordinator(entry.Raw)
	case RecoveryArtifactCleanupSchedule:
		return classifyRecoveryCleanupMember(entry.Raw)
	default:
		return recoveryArtifactClassification{}, errors.New("unsupported recovery artifact kind")
	}
}

func classifyRecoveryRecord(tenantID string, raw []byte, allowLegacy bool) (recoveryArtifactClassification, error) {
	fields, err := recoveryInventoryObject(raw)
	if err != nil {
		return recoveryArtifactClassification{}, err
	}

	version, versioned := fields["formatVersion"]
	for name := range fields {
		if strings.EqualFold(name, "formatVersion") && name != "formatVersion" {
			return recoveryArtifactClassification{}, errors.New("ambiguous recovery format discriminator")
		}
	}

	if versioned {
		if string(version) != "2" {
			return recoveryArtifactClassification{}, errors.New("unsupported recovery format version")
		}

		envelope, err := DecodeTransactionCompletionRecord(raw)
		if err != nil {
			return recoveryArtifactClassification{}, err
		}

		if envelope.TenantID != tenantID {
			return recoveryArtifactClassification{}, errors.New("recovery envelope tenant differs from inventory scope")
		}

		payload, err := DecodeTransactionCompletionPlan([]byte(envelope.Payload))
		if err != nil {
			return recoveryArtifactClassification{}, err
		}

		return recoveryArtifactClassification{format: RecoveryFormatEngineEnvelopeV2, action: recoveryInventoryAction(payload.Action)}, nil
	}

	if !allowLegacy {
		return recoveryArtifactClassification{}, errors.New("engine recover record requires format version 2")
	}

	var legacy mmodel.TransactionRedisQueue
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return recoveryArtifactClassification{}, fmt.Errorf("decode legacy recovery record: %w", err)
	}

	if legacy.TransactionID == uuid.Nil || legacy.OrganizationID == uuid.Nil || legacy.LedgerID == uuid.Nil {
		return recoveryArtifactClassification{}, errors.New("legacy recovery record has incomplete identity")
	}

	return recoveryArtifactClassification{format: RecoveryFormatLegacy, action: recoveryInventoryAction(legacy.Action)}, nil
}

func recoveryInventoryObject(raw []byte) (map[string]json.RawMessage, error) {
	if !json.Valid(raw) {
		return nil, errors.New("invalid recovery JSON")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := validateCompletionJSONValue(decoder); err != nil {
		return nil, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("recovery record must be an object")
	}

	return fields, nil
}

type recoveryInventoryReceipt struct {
	FormatVersion     int                        `json:"formatVersion"`
	TenantID          string                     `json:"tenantId"`
	OrganizationID    string                     `json:"organizationId"`
	LedgerID          string                     `json:"ledgerId"`
	ExecutionID       string                     `json:"executionId"`
	IntentFingerprint string                     `json:"intentFingerprint"`
	Response          string                     `json:"response"`
	Protection        *recoveryReceiptProtection `json:"protection,omitempty"`
}

type recoveryReceiptProtection struct {
	FormatVersion         int              `json:"formatVersion"`
	RetentionSeconds      int64            `json:"retentionSeconds"`
	Transactions          []string         `json:"transactions"`
	RecoveryFields        []string         `json:"recoveryFields"`
	Acknowledged          map[string]bool  `json:"acknowledged"`
	TerminalCompletedAtMS map[string]int64 `json:"terminalCompletedAtMs"`
	CleanupAfterMS        int64            `json:"cleanupAfterMs,omitempty"`
}

func classifyRecoveryReceipt(tenantID string, raw []byte) (recoveryArtifactClassification, error) {
	var receipt recoveryInventoryReceipt
	if err := decodeTransactionCompletionJSON(raw, &receipt); err != nil {
		return recoveryArtifactClassification{}, err
	}

	if receipt.FormatVersion != 1 || receipt.TenantID != tenantID {
		return recoveryArtifactClassification{}, errors.New("invalid recovery receipt version or tenant")
	}

	for _, rawID := range []string{receipt.OrganizationID, receipt.LedgerID, receipt.ExecutionID} {
		if !isCanonicalRecoveryInventoryUUID(rawID) {
			return recoveryArtifactClassification{}, errors.New("invalid recovery receipt identity")
		}
	}

	if receipt.IntentFingerprint == "" || receipt.Response == "" {
		return recoveryArtifactClassification{}, errors.New("incomplete recovery receipt")
	}

	if receipt.Protection == nil {
		return recoveryArtifactClassification{format: RecoveryFormatReceiptV1Unprotected, action: RecoveryActionUnknown}, nil
	}

	if err := validateCompletionReceiptProtection(receipt.ExecutionID, receipt.Protection); err != nil {
		return recoveryArtifactClassification{}, err
	}

	return recoveryArtifactClassification{format: RecoveryFormatReceiptV1Protected, action: RecoveryActionUnknown}, nil
}

func validateCompletionReceiptProtection(executionID string, protection *recoveryReceiptProtection) error {
	if protection.FormatVersion != 1 || protection.RetentionSeconds < 1 || protection.RetentionSeconds > 604800 ||
		len(protection.Transactions) == 0 || len(protection.Transactions) != len(protection.RecoveryFields) ||
		protection.Acknowledged == nil || protection.TerminalCompletedAtMS == nil {
		return errors.New("invalid recovery receipt protection")
	}

	knownTransactions, err := validateCompletionProtectionMembers(executionID, protection)
	if err != nil {
		return err
	}

	allReady, latestTerminal, err := validateCompletionProtectionProofs(protection, knownTransactions)
	if err != nil {
		return err
	}

	if !allReady {
		if protection.CleanupAfterMS != 0 {
			return errors.New("premature recovery receipt cleanup deadline")
		}

		return nil
	}

	retentionMillis := protection.RetentionSeconds * 1000
	if latestTerminal > math.MaxInt64-retentionMillis || protection.CleanupAfterMS != latestTerminal+retentionMillis {
		return errors.New("invalid recovery receipt cleanup deadline")
	}

	return nil
}

func validateCompletionProtectionMembers(executionID string, protection *recoveryReceiptProtection) (map[string]struct{}, error) {
	knownTransactions := make(map[string]struct{}, len(protection.Transactions))

	for index, transactionID := range protection.Transactions {
		if !isCanonicalRecoveryInventoryUUID(transactionID) {
			return nil, errors.New("invalid recovery receipt protection transaction")
		}

		if _, exists := knownTransactions[transactionID]; exists {
			return nil, errors.New("duplicate recovery receipt protection transaction")
		}

		if protection.RecoveryFields[index] != transactionID+":"+executionID {
			return nil, errors.New("invalid recovery receipt protection field")
		}

		knownTransactions[transactionID] = struct{}{}
	}

	return knownTransactions, nil
}

func validateCompletionProtectionProofs(
	protection *recoveryReceiptProtection,
	knownTransactions map[string]struct{},
) (bool, int64, error) {
	for transactionID := range protection.Acknowledged {
		if _, exists := knownTransactions[transactionID]; !exists {
			return false, 0, errors.New("unknown recovery receipt acknowledgement")
		}
	}

	for transactionID, completedAt := range protection.TerminalCompletedAtMS {
		if _, exists := knownTransactions[transactionID]; !exists || completedAt < 1 || !protection.Acknowledged[transactionID] {
			return false, 0, errors.New("invalid recovery receipt terminal proof")
		}
	}

	allReady, latestTerminal := true, int64(0)

	for _, transactionID := range protection.Transactions {
		completedAt := protection.TerminalCompletedAtMS[transactionID]
		if !protection.Acknowledged[transactionID] || completedAt < 1 {
			allReady = false
		}

		if completedAt > latestTerminal {
			latestTerminal = completedAt
		}
	}

	return allReady, latestTerminal, nil
}

func classifyRecoveryCoordinator(raw []byte) (recoveryArtifactClassification, error) {
	var coordinator struct {
		FormatVersion  int              `json:"formatVersion"`
		Executions     map[string]int64 `json:"executions"`
		CleanupAfterMS int64            `json:"cleanupAfterMs,omitempty"`
	}
	if err := decodeTransactionCompletionJSON(raw, &coordinator); err != nil {
		return recoveryArtifactClassification{}, err
	}

	if coordinator.FormatVersion != 1 || len(coordinator.Executions) == 0 {
		return recoveryArtifactClassification{}, errors.New("invalid recovery coordinator")
	}

	allReady, latestDeadline := true, int64(0)

	for executionID, deadline := range coordinator.Executions {
		if !isCanonicalRecoveryInventoryUUID(executionID) || deadline < 0 {
			return recoveryArtifactClassification{}, errors.New("invalid recovery coordinator execution")
		}

		if deadline == 0 {
			allReady = false
		}

		if deadline > latestDeadline {
			latestDeadline = deadline
		}
	}

	if (allReady && coordinator.CleanupAfterMS != latestDeadline) || (!allReady && coordinator.CleanupAfterMS != 0) {
		return recoveryArtifactClassification{}, errors.New("invalid recovery coordinator cleanup deadline")
	}

	return recoveryArtifactClassification{format: RecoveryFormatCoordinatorV1, action: RecoveryActionUnknown}, nil
}

func classifyRecoveryCleanupMember(raw []byte) (recoveryArtifactClassification, error) {
	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 {
		return recoveryArtifactClassification{}, errors.New("invalid recovery cleanup member")
	}

	for _, part := range parts {
		if !isCanonicalRecoveryInventoryUUID(part) {
			return recoveryArtifactClassification{}, errors.New("invalid recovery cleanup member identity")
		}
	}

	return recoveryArtifactClassification{format: RecoveryFormatCleanupMemberV1, action: RecoveryActionUnknown}, nil
}

func isCanonicalRecoveryInventoryUUID(raw string) bool {
	id, err := uuid.Parse(raw)

	return err == nil && id != uuid.Nil && id.String() == raw
}

func recoveryInventoryAction(action string) string {
	switch action {
	case constant.ActionDirect, constant.ActionHold:
		return RecoveryActionCreate
	case constant.ActionRevert:
		return RecoveryActionRevert
	case constant.ActionCommit:
		return RecoveryActionCommit
	case constant.ActionCancel:
		return RecoveryActionCancel
	default:
		return RecoveryActionUnknown
	}
}

func recoveryInventoryKeyDigest(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:8])
}
