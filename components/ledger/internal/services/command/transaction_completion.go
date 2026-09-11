// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const (
	// TransactionCompletionFormatVersion identifies the completion plan and record schema.
	TransactionCompletionFormatVersion = 2
	// EngineOperationIDNamespaceV1 is immutable: changing it changes replayed row IDs.
	EngineOperationIDNamespaceV1 = "c102438e-88ba-5d08-b785-a699df083ecd"

	OperationSpecSideFrom = "from"
	OperationSpecSideTo   = "to"

	OperationRecordStandard               = "standard"
	OperationRecordValidatedHoldDebit     = "validated_hold_debit"
	OperationRecordValidatedHoldReserve   = "validated_hold_reserve"
	OperationRecordValidatedCancelRelease = "validated_cancel_release"
	OperationRecordValidatedCancelCredit  = "validated_cancel_credit"
)

// ErrInvalidTransactionCompletionRecord identifies an invalid internal completion record.
// It is not a public balance refusal and must not trigger another accounting execution.
var ErrInvalidTransactionCompletionRecord = errors.New("invalid transaction completion record")

// OperationBalanceContext preserves persisted snapshot fields without the public
// balance response's derived position fields or custom response marshaling.
type OperationBalanceContext mmodel.Balance

// OperationRecordSpec records a row's immutable attribution and operation rule.
// Balance supplies identity and the original row snapshot, never an authoritative
// overdraft split. The executed Movement supplies truthful monetary state.
// Contexts are ordered; (TransactionID, PostingRef, Role, Ordinal) is unique.
type OperationRecordSpec struct {
	TransactionID     uuid.UUID               `json:"transactionId"`
	PostingRef        string                  `json:"postingRef"`
	OriginRef         string                  `json:"originRef"`
	BalanceRef        string                  `json:"balanceRef"`
	Role              string                  `json:"role"`
	Ordinal           uint32                  `json:"ordinal"`
	Side              string                  `json:"side"`
	RowType           string                  `json:"rowType"`
	Direction         string                  `json:"direction"`
	Description       string                  `json:"description"`
	RouteID           *string                 `json:"routeId"`
	RouteCode         string                  `json:"routeCode"`
	RouteDescription  string                  `json:"routeDescription"`
	ChartOfAccounts   string                  `json:"chartOfAccounts"`
	Metadata          map[string]any          `json:"metadata"`
	Balance           OperationBalanceContext `json:"balance"`
	RequestedAmount   decimal.Decimal         `json:"requestedAmount"`
	CompatibilityPath string                  `json:"compatibilityPath"`
}

// TransactionCompletionPlan freezes Go processing decisions before execution.
// TransactionInput includes resolved fees. Validate is retained for compatibility,
// but neither its split amounts nor Balance snapshots determine engine arithmetic.
// TransactionDate fixes the action/operation creation date. The other timestamps
// preserve the original transaction creation and separately captured updates.
type TransactionCompletionPlan struct {
	FormatVersion        int                      `json:"formatVersion"`
	TenantID             string                   `json:"tenantId"`
	HeaderID             string                   `json:"header_id"`
	TransactionID        uuid.UUID                `json:"transaction_id"`
	ParentTransactionID  *uuid.UUID               `json:"parentTransactionId"`
	FeesSkipped          bool                     `json:"feesSkipped"`
	TracerSkipped        bool                     `json:"tracerSkipped"`
	OrganizationID       uuid.UUID                `json:"organization_id"`
	LedgerID             uuid.UUID                `json:"ledger_id"`
	ExecutionID          uuid.UUID                `json:"executionId"`
	IntentFingerprint    string                   `json:"intentFingerprint"`
	TransactionInput     mtransaction.Transaction `json:"parserDSL"`
	TTL                  time.Time                `json:"ttl"`
	Validate             *mtransaction.Responses  `json:"validate"`
	TransactionStatus    string                   `json:"transaction_status"`
	Action               string                   `json:"action"`
	TransactionDate      time.Time                `json:"transaction_date"`
	TransactionCreatedAt time.Time                `json:"transactionCreatedAt"`
	TransactionUpdatedAt time.Time                `json:"transactionUpdatedAt"`
	OperationUpdatedAt   time.Time                `json:"operationUpdatedAt"`
	OperationSpecs       []OperationRecordSpec    `json:"projection"`
}

// TransactionCompletionRecord stores one transaction's actual executed result.
// Payload is opaque to storage/accounting adapters; command and recovery decode it.
type TransactionCompletionRecord struct {
	FormatVersion     int                        `json:"formatVersion"`
	TenantID          string                     `json:"tenantId"`
	OrganizationID    uuid.UUID                  `json:"organizationId"`
	LedgerID          uuid.UUID                  `json:"ledgerId"`
	ExecutionID       uuid.UUID                  `json:"executionId"`
	IntentFingerprint string                     `json:"intentFingerprint"`
	TransactionID     uuid.UUID                  `json:"transactionId"`
	Payload           string                     `json:"payload"`
	Result            accounting.ExecutionResult `json:"result"`
}

// EngineTransactionIntent contains only immutable intent, not calculated
// postings, validation output, balance seeds, guards, or overdraft splits.
type EngineTransactionIntent struct {
	TransactionID        uuid.UUID                       `json:"transactionId"`
	ParentTransactionID  *uuid.UUID                      `json:"parentTransactionId"`
	FeesSkipped          bool                            `json:"feesSkipped"`
	TracerSkipped        bool                            `json:"tracerSkipped"`
	Action               string                          `json:"action"`
	TransactionStatus    string                          `json:"transactionStatus"`
	TransactionDate      time.Time                       `json:"transactionDate"`
	TransactionCreatedAt time.Time                       `json:"transactionCreatedAt"`
	TransactionUpdatedAt time.Time                       `json:"transactionUpdatedAt"`
	OperationUpdatedAt   time.Time                       `json:"operationUpdatedAt"`
	Input                mtransaction.Transaction        `json:"input"`
	PostingRefs          []string                        `json:"postingRefs"`
	BalanceRequirements  []accounting.BalanceRequirement `json:"balanceRequirements"`
	OperationSpecs       []OperationRecordIntent         `json:"projection"`
}

// OperationRecordIntent fingerprints immutable row decisions without carrying
// a monetary snapshot or an engine-calculated split.
type OperationRecordIntent struct {
	PostingRef        string          `json:"postingRef"`
	OriginRef         string          `json:"originRef"`
	BalanceRef        string          `json:"balanceRef"`
	Role              string          `json:"role"`
	Ordinal           uint32          `json:"ordinal"`
	Side              string          `json:"side"`
	RowType           string          `json:"rowType"`
	Direction         string          `json:"direction"`
	Description       string          `json:"description"`
	RouteID           *string         `json:"routeId"`
	RouteCode         string          `json:"routeCode"`
	RouteDescription  string          `json:"routeDescription"`
	ChartOfAccounts   string          `json:"chartOfAccounts"`
	Metadata          map[string]any  `json:"metadata"`
	RequestedAmount   decimal.Decimal `json:"requestedAmount"`
	CompatibilityPath string          `json:"compatibilityPath"`
}

// Intent returns the snapshot-independent fields used by the execution fingerprint.
func (spec OperationRecordSpec) Intent() OperationRecordIntent {
	return OperationRecordIntent{
		PostingRef: spec.PostingRef, OriginRef: spec.OriginRef, BalanceRef: spec.BalanceRef, Role: spec.Role, Ordinal: spec.Ordinal,
		Side: spec.Side, RowType: spec.RowType, Direction: spec.Direction, Description: spec.Description,
		RouteID: spec.RouteID, RouteCode: spec.RouteCode, RouteDescription: spec.RouteDescription,
		ChartOfAccounts: spec.ChartOfAccounts, Metadata: spec.Metadata, RequestedAmount: spec.RequestedAmount,
		CompatibilityPath: spec.CompatibilityPath,
	}
}

// EngineIntent fixes the execution scope and ordered logical intentions.
type EngineIntent struct {
	TenantID       string                    `json:"tenantId"`
	OrganizationID uuid.UUID                 `json:"organizationId"`
	LedgerID       uuid.UUID                 `json:"ledgerId"`
	ExecutionID    uuid.UUID                 `json:"executionId"`
	Transactions   []EngineTransactionIntent `json:"transactions"`
}

// ComputeEngineIntentFingerprint hashes deterministic JSON of explicit
// immutable intent. Derived companion contexts are excluded; their attribution
// is inherited from primaries. encoding/json sorts map keys; money stays strings.
func ComputeEngineIntentFingerprint(intent EngineIntent) (string, error) {
	if intent.OrganizationID == uuid.Nil || intent.LedgerID == uuid.Nil || intent.ExecutionID == uuid.Nil || len(intent.Transactions) == 0 {
		return "", invalidTransactionCompletionRecord("missing intent identity")
	}

	intent.Transactions = append([]EngineTransactionIntent(nil), intent.Transactions...)
	seen := make(map[uuid.UUID]bool, len(intent.Transactions))

	for index, transaction := range intent.Transactions {
		if transaction.TransactionID == uuid.Nil || seen[transaction.TransactionID] || transaction.Action == "" {
			return "", invalidTransactionCompletionRecord("invalid transaction intention")
		}

		if !validFrozenTimestamps(transaction.TransactionDate, transaction.TransactionCreatedAt, transaction.TransactionUpdatedAt, transaction.OperationUpdatedAt) {
			return "", invalidTransactionCompletionRecord("missing frozen intention timestamps")
		}

		if !validCompletionParent(transaction.TransactionID, transaction.ParentTransactionID) {
			return "", invalidTransactionCompletionRecord("invalid parent transaction identity")
		}

		seen[transaction.TransactionID] = true

		refs := make(map[string]bool, len(transaction.PostingRefs))
		for _, ref := range transaction.PostingRefs {
			if ref == "" || refs[ref] {
				return "", invalidTransactionCompletionRecord("invalid intent posting reference")
			}

			refs[ref] = true
		}

		for _, requirement := range transaction.BalanceRequirements {
			if requirement.BalanceRef == "" || requirement.AssetCode == "" ||
				(requirement.Permission != accounting.BalancePermissionSend && requirement.Permission != accounting.BalancePermissionReceive) {
				return "", invalidTransactionCompletionRecord("invalid intent balance requirement")
			}
		}

		primaryProjection := make([]OperationRecordIntent, 0, len(transaction.OperationSpecs))
		for _, spec := range transaction.OperationSpecs {
			if !validOperationRecordRole(spec.Role) {
				return "", invalidTransactionCompletionRecord("invalid intent spec role")
			}

			if spec.Role == accounting.RolePrimary {
				primaryProjection = append(primaryProjection, spec)
			}
		}

		intent.Transactions[index].OperationSpecs = primaryProjection
	}

	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("encode engine intention: %w", err)
	}

	hash := sha256.Sum256(append([]byte("midaz.balance-accounting.intent.v1\x00"), encoded...))

	return hex.EncodeToString(hash[:]), nil
}

// DeterministicOperationID derives a UUIDv5 from an immutable namespace and
// length-prefixed fields. Ordinals distinguish rows without relying on map order.
func DeterministicOperationID(executionID, transactionID uuid.UUID, postingRef, role string, ordinal uint32) (uuid.UUID, error) {
	if executionID == uuid.Nil || transactionID == uuid.Nil || postingRef == "" || !validOperationRecordRole(role) {
		return uuid.Nil, invalidTransactionCompletionRecord("invalid operation identity")
	}

	var input []byte
	for _, field := range [][]byte{executionID[:], transactionID[:], []byte(postingRef), []byte(role)} {
		input = binary.BigEndian.AppendUint64(input, uint64(len(field)))
		input = append(input, field...)
	}

	input = binary.BigEndian.AppendUint32(input, ordinal)

	namespace, err := uuid.Parse(EngineOperationIDNamespaceV1)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse operation namespace: %w", err)
	}

	return uuid.NewSHA1(namespace, input), nil
}

// EncodeTransactionCompletionPlan validates and freezes a typed payload as JSON.
func EncodeTransactionCompletionPlan(payload TransactionCompletionPlan) (json.RawMessage, error) {
	if err := validateTransactionCompletionPlan(payload); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode transaction completion plan: %w", err)
	}

	return encoded, nil
}

// DecodeTransactionCompletionPlan accepts only the supported typed schema.
func DecodeTransactionCompletionPlan(data []byte) (*TransactionCompletionPlan, error) {
	var payload TransactionCompletionPlan
	if err := decodeTransactionCompletionJSON(data, &payload); err != nil {
		return nil, err
	}

	if err := validateTransactionCompletionPlan(payload); err != nil {
		return nil, err
	}

	return &payload, nil
}

// ValidateTransactionCompletion checks request, recovery, and guard correlation.
// The adapter must separately compare the payload tenant with authenticated context.
func ValidateTransactionCompletion(input EngineExecution) error {
	request := input.Execution
	if err := validateCompletionExecutionIdentity(input); err != nil {
		return err
	}

	transactions := make(map[uuid.UUID]accounting.Transaction, len(request.Transactions))
	for _, transaction := range request.Transactions {
		if transaction.ID == uuid.Nil {
			return invalidTransactionCompletionRecord("missing transaction identity")
		}

		if _, duplicate := transactions[transaction.ID]; duplicate {
			return invalidTransactionCompletionRecord("duplicate transaction identity")
		}

		transactions[transaction.ID] = transaction
	}

	seen := make(map[uuid.UUID]bool, len(input.CompletionPlans))
	intents := make(map[uuid.UUID]EngineTransactionIntent, len(input.CompletionPlans))

	var tenant string

	tenantSet := false

	for _, recovery := range input.CompletionPlans {
		transaction, exists := transactions[recovery.TransactionID]
		if !exists || seen[recovery.TransactionID] {
			return invalidTransactionCompletionRecord("unrelated or duplicate recovery transaction")
		}

		seen[recovery.TransactionID] = true

		payload, err := DecodeTransactionCompletionPlan(recovery.Payload)
		if err != nil {
			return err
		}

		if err := validateCompletionExecutionScope(input, recovery.TransactionID, payload); err != nil {
			return err
		}

		if tenantSet && tenant != payload.TenantID {
			return invalidTransactionCompletionRecord("mixed recovery tenants")
		}

		tenant = payload.TenantID
		tenantSet = true

		if err := validateCompletionPostings(transaction, payload.OperationSpecs); err != nil {
			return err
		}

		if err := validateCompletionSnapshotIdentities(request.Balances, payload.OperationSpecs); err != nil {
			return err
		}

		if err := validateCompletionBalanceRequirements(request.Balances, transaction.BalanceRequirements); err != nil {
			return err
		}

		intents[transaction.ID] = transactionCompletionIntent(transaction, *payload)
	}

	if err := validateCompletionGuards(input.Guards, transactions); err != nil {
		return err
	}

	return validateCompletionExecutionFingerprint(request, tenant, intents, input.IntentFingerprint)
}

func validateCompletionBalanceRequirements(snapshots []accounting.BalanceSnapshot, requirements []accounting.BalanceRequirement) error {
	known := make(map[string]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		known[snapshot.BalanceRef] = struct{}{}
	}

	for _, requirement := range requirements {
		if _, exists := known[requirement.BalanceRef]; !exists || requirement.AssetCode == "" ||
			(requirement.Permission != accounting.BalancePermissionSend && requirement.Permission != accounting.BalancePermissionReceive) {
			return invalidTransactionCompletionRecord("invalid execution balance requirement")
		}
	}

	return nil
}

func validateCompletionGuards(guards []ExecutionGuard, transactions map[uuid.UUID]accounting.Transaction) error {
	seen := make(map[uuid.UUID]bool, len(guards))
	for _, guard := range guards {
		if _, exists := transactions[guard.TransactionID]; !exists || seen[guard.TransactionID] || guard.NextToken == "" || guard.ExpectedToken == guard.NextToken {
			return invalidTransactionCompletionRecord("invalid execution guard correlation")
		}

		seen[guard.TransactionID] = true
	}

	return nil
}

func validateCompletionExecutionFingerprint(request accounting.Execution, tenant string, intents map[uuid.UUID]EngineTransactionIntent, expected string) error {
	intent := EngineIntent{
		TenantID: tenant, OrganizationID: request.OrganizationID, LedgerID: request.LedgerID, ExecutionID: request.ExecutionID,
		Transactions: make([]EngineTransactionIntent, 0, len(request.Transactions)),
	}
	for _, transaction := range request.Transactions {
		intent.Transactions = append(intent.Transactions, intents[transaction.ID])
	}

	fingerprint, err := ComputeEngineIntentFingerprint(intent)
	if err != nil {
		return err
	}

	if fingerprint != expected {
		return invalidTransactionCompletionRecord("fingerprint does not match frozen execution intent")
	}

	return nil
}

func validateCompletionSnapshotIdentities(snapshots []accounting.BalanceSnapshot, projections []OperationRecordSpec) error {
	byRef := make(map[string]accounting.BalanceSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byRef[snapshot.BalanceRef] = snapshot
	}

	for _, spec := range projections {
		snapshot, exists := byRef[spec.BalanceRef]
		if !exists {
			continue
		}

		balance := spec.Balance
		if balance.ID != snapshot.ID.String() || balance.AccountID != snapshot.AccountID.String() || balance.Key != snapshot.Key || mtransaction.SplitAlias(balance.Alias) != snapshot.Alias || balance.AssetCode != snapshot.AssetCode || balance.AccountType != snapshot.AccountType {
			return invalidTransactionCompletionRecord("spec identity does not match execution snapshot")
		}
	}

	return nil
}

func transactionCompletionIntent(transaction accounting.Transaction, payload TransactionCompletionPlan) EngineTransactionIntent {
	intent := EngineTransactionIntent{
		TransactionID: payload.TransactionID, ParentTransactionID: payload.ParentTransactionID,
		FeesSkipped: payload.FeesSkipped, TracerSkipped: payload.TracerSkipped, Action: payload.Action,
		TransactionStatus: payload.TransactionStatus, TransactionDate: payload.TransactionDate, Input: payload.TransactionInput,
		TransactionCreatedAt: payload.TransactionCreatedAt, TransactionUpdatedAt: payload.TransactionUpdatedAt, OperationUpdatedAt: payload.OperationUpdatedAt,
		PostingRefs:         make([]string, 0, len(transaction.Postings)),
		BalanceRequirements: append([]accounting.BalanceRequirement(nil), transaction.BalanceRequirements...),
		OperationSpecs:      make([]OperationRecordIntent, 0, len(payload.OperationSpecs)),
	}
	for _, posting := range transaction.Postings {
		intent.PostingRefs = append(intent.PostingRefs, posting.Ref)
	}

	for _, spec := range payload.OperationSpecs {
		intent.OperationSpecs = append(intent.OperationSpecs, spec.Intent())
	}

	return intent
}

func validateCompletionExecutionIdentity(input EngineExecution) error {
	request := input.Execution
	if request.OrganizationID == uuid.Nil || request.LedgerID == uuid.Nil || request.ExecutionID == uuid.Nil || !validIntentFingerprint(input.IntentFingerprint) || len(request.Transactions) == 0 {
		return invalidTransactionCompletionRecord("invalid execution identity")
	}

	if len(input.CompletionPlans) != len(request.Transactions) || len(input.Guards) != len(request.Transactions) {
		return invalidTransactionCompletionRecord("transactions, recovery and guards must correlate one to one")
	}

	return nil
}

func validateCompletionExecutionScope(input EngineExecution, transactionID uuid.UUID, payload *TransactionCompletionPlan) error {
	request := input.Execution
	if payload.TransactionID != transactionID || payload.ExecutionID != request.ExecutionID || payload.OrganizationID != request.OrganizationID || payload.LedgerID != request.LedgerID || payload.IntentFingerprint != input.IntentFingerprint {
		return invalidTransactionCompletionRecord("recovery scope does not match execution")
	}

	return nil
}

func validateCompletionPostings(transaction accounting.Transaction, projections []OperationRecordSpec) error {
	postings := make(map[string]accounting.Posting, len(transaction.Postings))
	for _, posting := range transaction.Postings {
		if posting.Ref == "" {
			return invalidTransactionCompletionRecord("missing posting reference")
		}

		if _, duplicate := postings[posting.Ref]; duplicate {
			return invalidTransactionCompletionRecord("duplicate posting reference")
		}

		postings[posting.Ref] = posting
	}

	primaries := make(map[string]bool, len(postings))
	for _, spec := range projections {
		posting, exists := postings[spec.PostingRef]
		if !exists {
			return invalidTransactionCompletionRecord("spec references an unrelated posting")
		}

		if spec.Role == accounting.RolePrimary {
			if spec.BalanceRef != posting.BalanceRef {
				return invalidTransactionCompletionRecord("spec balance does not match posting")
			}

			primaries[spec.PostingRef] = true
		}
	}

	if len(primaries) != len(postings) {
		return invalidTransactionCompletionRecord("posting has no primary spec context")
	}

	return nil
}

// EncodeTransactionCompletionRecord validates a completed execution record.
func EncodeTransactionCompletionRecord(envelope TransactionCompletionRecord) (json.RawMessage, error) {
	if err := validateTransactionCompletionRecord(envelope); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode transaction completion record: %w", err)
	}

	return encoded, nil
}

// DecodeTransactionCompletionRecord rejects unrecognized versions and scope drift.
func DecodeTransactionCompletionRecord(data []byte) (*TransactionCompletionRecord, error) {
	var envelope TransactionCompletionRecord
	if err := decodeTransactionCompletionJSON(data, &envelope); err != nil {
		return nil, err
	}

	if err := validateTransactionCompletionRecord(envelope); err != nil {
		return nil, err
	}

	return &envelope, nil
}

func validateTransactionCompletionPlan(payload TransactionCompletionPlan) error {
	if payload.FormatVersion != TransactionCompletionFormatVersion || payload.TransactionID == uuid.Nil || payload.OrganizationID == uuid.Nil || payload.LedgerID == uuid.Nil || payload.ExecutionID == uuid.Nil || !validIntentFingerprint(payload.IntentFingerprint) {
		return invalidTransactionCompletionRecord("invalid payload version or identity")
	}

	if !validCompletionParent(payload.TransactionID, payload.ParentTransactionID) {
		return invalidTransactionCompletionRecord("invalid parent transaction identity")
	}

	if payload.TTL.IsZero() || payload.Action == "" || payload.TransactionStatus == "" || payload.OperationSpecs == nil {
		return invalidTransactionCompletionRecord("missing frozen transaction context")
	}

	if !validFrozenTimestamps(payload.TransactionDate, payload.TransactionCreatedAt, payload.TransactionUpdatedAt, payload.OperationUpdatedAt) {
		return invalidTransactionCompletionRecord("missing frozen transaction timestamps")
	}

	seen := make(map[uuid.UUID]bool, len(payload.OperationSpecs))
	for _, spec := range payload.OperationSpecs {
		if err := validateOperationRecordSpec(payload, spec); err != nil {
			return err
		}

		id, err := DeterministicOperationID(payload.ExecutionID, payload.TransactionID, spec.PostingRef, spec.Role, spec.Ordinal)
		if err != nil || seen[id] {
			return invalidTransactionCompletionRecord("duplicate or invalid spec reference")
		}

		seen[id] = true
	}

	contexts, _, err := indexOperationRecordSpecs(payload.OperationSpecs)
	if err != nil {
		return err
	}

	return validateOperationRecordAttribution(contexts)
}

func validFrozenTimestamps(action, created, updated, operationUpdated time.Time) bool {
	return !action.IsZero() && !created.IsZero() && !updated.IsZero() && !operationUpdated.IsZero()
}

func validateOperationRecordSpec(payload TransactionCompletionPlan, spec OperationRecordSpec) error {
	if spec.TransactionID != payload.TransactionID || spec.BalanceRef == "" || spec.RowType == "" || !spec.RequestedAmount.IsPositive() {
		return invalidTransactionCompletionRecord("invalid spec identity or amount")
	}

	if spec.Side != OperationSpecSideFrom && spec.Side != OperationSpecSideTo {
		return invalidTransactionCompletionRecord("invalid spec side")
	}

	if spec.Direction != "debit" && spec.Direction != "credit" {
		return invalidTransactionCompletionRecord("invalid row direction")
	}

	switch spec.CompatibilityPath {
	case OperationRecordStandard:
	case OperationRecordValidatedHoldDebit, OperationRecordValidatedHoldReserve, OperationRecordValidatedCancelRelease, OperationRecordValidatedCancelCredit:
		if spec.OriginRef == "" {
			return invalidTransactionCompletionRecord("compatibility spec requires an origin reference")
		}
	default:
		return invalidTransactionCompletionRecord("unknown spec compatibility path")
	}

	return validateOperationBalanceContext(payload, spec)
}

func validateOperationBalanceContext(payload TransactionCompletionPlan, spec OperationRecordSpec) error {
	if spec.Balance.OrganizationID != payload.OrganizationID.String() || spec.Balance.LedgerID != payload.LedgerID.String() {
		return invalidTransactionCompletionRecord("spec snapshot scope mismatch")
	}

	if spec.Balance.Alias == "" || spec.Balance.Key == "" || spec.Balance.AssetCode == "" || spec.BalanceRef != mtransaction.SplitAlias(spec.Balance.Alias)+"#"+spec.Balance.Key {
		return invalidTransactionCompletionRecord("spec logical balance identity mismatch")
	}

	for _, value := range spec.Metadata {
		switch value.(type) {
		case nil, string, bool, json.Number, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		default:
			return invalidTransactionCompletionRecord("spec metadata must be flat")
		}
	}

	for _, value := range []string{spec.Balance.ID, spec.Balance.AccountID} {
		parsed, err := uuid.Parse(value)
		if err != nil || parsed == uuid.Nil {
			return invalidTransactionCompletionRecord("invalid spec balance identity")
		}
	}

	if spec.Role == accounting.RoleOverdraftCompanion && (spec.Balance.Key != "overdraft" || len(spec.Metadata) != 0 || spec.ChartOfAccounts != "") {
		return invalidTransactionCompletionRecord("invalid companion spec context")
	}

	return nil
}

func validateTransactionCompletionRecord(envelope TransactionCompletionRecord) error {
	if envelope.FormatVersion != TransactionCompletionFormatVersion || envelope.Result.Movements == nil || envelope.Result.Final == nil {
		return invalidTransactionCompletionRecord("invalid envelope version or result shape")
	}

	payload, err := DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	if err != nil {
		return err
	}

	if envelope.TenantID != payload.TenantID || envelope.OrganizationID != payload.OrganizationID || envelope.LedgerID != payload.LedgerID || envelope.ExecutionID != payload.ExecutionID || envelope.TransactionID != payload.TransactionID || envelope.IntentFingerprint != payload.IntentFingerprint {
		return invalidTransactionCompletionRecord("envelope and payload scope mismatch")
	}

	_, err = validateOperationMovementResult(*payload, envelope.Result)

	return err
}

func validOperationRecordRole(role string) bool {
	return role == accounting.RolePrimary || role == accounting.RoleOverdraftCompanion
}

func validCompletionParent(transactionID uuid.UUID, parentID *uuid.UUID) bool {
	return parentID == nil || (*parentID != uuid.Nil && *parentID != transactionID)
}

func validIntentFingerprint(fingerprint string) bool {
	decoded, err := hex.DecodeString(fingerprint)
	return err == nil && len(decoded) == sha256.Size && fingerprint == strings.ToLower(fingerprint)
}

func invalidTransactionCompletionRecord(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidTransactionCompletionRecord, reason)
}

func decodeTransactionCompletionJSON(data []byte, target any) error {
	if !json.Valid(data) || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return invalidTransactionCompletionRecord("expected a JSON object")
	}

	tokens := json.NewDecoder(bytes.NewReader(data))
	tokens.UseNumber()

	if err := validateCompletionJSONValue(tokens); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: decode JSON: %w", ErrInvalidTransactionCompletionRecord, err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return invalidTransactionCompletionRecord("unexpected trailing JSON")
	}

	return nil
}

func validateCompletionJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%w: read JSON token: %w", ErrInvalidTransactionCompletionRecord, err)
	}

	delimiter, structured := token.(json.Delim)
	if !structured {
		return nil
	}

	keys := make(map[string]bool)

	for decoder.More() {
		if delimiter == '{' {
			keyToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("%w: read JSON key: %w", ErrInvalidTransactionCompletionRecord, err)
			}

			key, ok := keyToken.(string)
			if !ok || keys[key] {
				return invalidTransactionCompletionRecord("duplicate or invalid JSON object key")
			}

			keys[key] = true
		}

		if err := validateCompletionJSONValue(decoder); err != nil {
			return err
		}
	}

	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("%w: close JSON value: %w", ErrInvalidTransactionCompletionRecord, err)
	}

	return nil
}
