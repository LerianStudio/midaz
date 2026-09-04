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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const (
	// BalanceEngineRecoveryVersion identifies the frozen-payload and envelope schema.
	BalanceEngineRecoveryVersion = 2
	// BalanceEngineOperationIDNamespaceV1 is immutable: changing it changes replayed row IDs.
	BalanceEngineOperationIDNamespaceV1 = "c102438e-88ba-5d08-b785-a699df083ecd"

	ProjectionSideFrom = "from"
	ProjectionSideTo   = "to"

	ProjectionStandard               = "standard"
	ProjectionValidatedHoldDebit     = "validated_hold_debit"
	ProjectionValidatedHoldReserve   = "validated_hold_reserve"
	ProjectionValidatedCancelRelease = "validated_cancel_release"
	ProjectionValidatedCancelCredit  = "validated_cancel_credit"
)

// ErrInvalidBalanceEngineRecovery identifies an invalid internal recovery protocol.
// It is not a public balance refusal and must not enter a stale-version retry loop.
var ErrInvalidBalanceEngineRecovery = errors.New("invalid balance engine recovery")

// FrozenProjectionBalance preserves persisted snapshot fields without the public
// balance response's derived position fields or custom response marshaling.
type FrozenProjectionBalance mmodel.Balance

// FrozenProjectionContext records a row's immutable attribution and projection rule.
// Balance supplies identity and the original row snapshot, never an authoritative
// overdraft split. The executed Movement supplies truthful monetary state.
// Contexts are ordered; (TransactionID, PostingRef, Role, Ordinal) is unique.
type FrozenProjectionContext struct {
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
	Balance           FrozenProjectionBalance `json:"balance"`
	RequestedAmount   decimal.Decimal         `json:"requestedAmount"`
	CompatibilityPath string                  `json:"compatibilityPath"`
}

// BalanceEngineRecoveryPayload freezes Go processing decisions before execution.
// TransactionInput includes resolved fees. Validate is retained for compatibility,
// but neither its split amounts nor Balance snapshots determine engine arithmetic.
type BalanceEngineRecoveryPayload struct {
	FormatVersion       int                       `json:"formatVersion"`
	TenantID            string                    `json:"tenantId"`
	HeaderID            string                    `json:"header_id"`
	TransactionID       uuid.UUID                 `json:"transaction_id"`
	ParentTransactionID *uuid.UUID                `json:"parentTransactionId"`
	FeesSkipped         bool                      `json:"feesSkipped"`
	TracerSkipped       bool                      `json:"tracerSkipped"`
	OrganizationID      uuid.UUID                 `json:"organization_id"`
	LedgerID            uuid.UUID                 `json:"ledger_id"`
	ExecutionID         uuid.UUID                 `json:"executionId"`
	IntentFingerprint   string                    `json:"intentFingerprint"`
	TransactionInput    mtransaction.Transaction  `json:"parserDSL"`
	TTL                 time.Time                 `json:"ttl"`
	Validate            *mtransaction.Responses   `json:"validate"`
	TransactionStatus   string                    `json:"transaction_status"`
	Action              string                    `json:"action"`
	TransactionDate     time.Time                 `json:"transaction_date"`
	Projection          []FrozenProjectionContext `json:"projection"`
}

// BalanceEngineRecoveryEnvelope stores one transaction's actual executed result.
// Payload is opaque to storage/accounting adapters; command and recovery decode it.
type BalanceEngineRecoveryEnvelope struct {
	FormatVersion     int           `json:"formatVersion"`
	TenantID          string        `json:"tenantId"`
	OrganizationID    uuid.UUID     `json:"organizationId"`
	LedgerID          uuid.UUID     `json:"ledgerId"`
	ExecutionID       uuid.UUID     `json:"executionId"`
	IntentFingerprint string        `json:"intentFingerprint"`
	TransactionID     uuid.UUID     `json:"transactionId"`
	Payload           string        `json:"payload"`
	Result            engine.Result `json:"result"`
}

// BalanceEngineTransactionIntent contains only immutable intent, not calculated
// postings, validation output, balance seeds, guards, or overdraft splits.
type BalanceEngineTransactionIntent struct {
	TransactionID       uuid.UUID                `json:"transactionId"`
	ParentTransactionID *uuid.UUID               `json:"parentTransactionId"`
	FeesSkipped         bool                     `json:"feesSkipped"`
	TracerSkipped       bool                     `json:"tracerSkipped"`
	Action              string                   `json:"action"`
	TransactionStatus   string                   `json:"transactionStatus"`
	TransactionDate     time.Time                `json:"transactionDate"`
	Input               mtransaction.Transaction `json:"input"`
	PostingRefs         []string                 `json:"postingRefs"`
	Projection          []FrozenProjectionIntent `json:"projection"`
}

// FrozenProjectionIntent fingerprints immutable row decisions without carrying
// a monetary snapshot or an engine-calculated split.
type FrozenProjectionIntent struct {
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
func (projection FrozenProjectionContext) Intent() FrozenProjectionIntent {
	return FrozenProjectionIntent{
		PostingRef: projection.PostingRef, OriginRef: projection.OriginRef, BalanceRef: projection.BalanceRef, Role: projection.Role, Ordinal: projection.Ordinal,
		Side: projection.Side, RowType: projection.RowType, Direction: projection.Direction, Description: projection.Description,
		RouteID: projection.RouteID, RouteCode: projection.RouteCode, RouteDescription: projection.RouteDescription,
		ChartOfAccounts: projection.ChartOfAccounts, Metadata: projection.Metadata, RequestedAmount: projection.RequestedAmount,
		CompatibilityPath: projection.CompatibilityPath,
	}
}

// BalanceEngineIntent fixes the execution scope and ordered logical intentions.
type BalanceEngineIntent struct {
	TenantID       string                           `json:"tenantId"`
	OrganizationID uuid.UUID                        `json:"organizationId"`
	LedgerID       uuid.UUID                        `json:"ledgerId"`
	ExecutionID    uuid.UUID                        `json:"executionId"`
	Transactions   []BalanceEngineTransactionIntent `json:"transactions"`
}

// ComputeBalanceEngineIntentFingerprint hashes deterministic JSON of explicit
// immutable intent. Derived companion contexts are excluded; their attribution
// is inherited from primaries. encoding/json sorts map keys; money stays strings.
func ComputeBalanceEngineIntentFingerprint(intent BalanceEngineIntent) (string, error) {
	if intent.OrganizationID == uuid.Nil || intent.LedgerID == uuid.Nil || intent.ExecutionID == uuid.Nil || len(intent.Transactions) == 0 {
		return "", invalidRecovery("missing intent identity")
	}

	intent.Transactions = append([]BalanceEngineTransactionIntent(nil), intent.Transactions...)
	seen := make(map[uuid.UUID]bool, len(intent.Transactions))

	for index, transaction := range intent.Transactions {
		if transaction.TransactionID == uuid.Nil || seen[transaction.TransactionID] || transaction.Action == "" || transaction.TransactionDate.IsZero() {
			return "", invalidRecovery("invalid transaction intention")
		}

		if !validRecoveryParent(transaction.TransactionID, transaction.ParentTransactionID) {
			return "", invalidRecovery("invalid parent transaction identity")
		}

		seen[transaction.TransactionID] = true

		refs := make(map[string]bool, len(transaction.PostingRefs))
		for _, ref := range transaction.PostingRefs {
			if ref == "" || refs[ref] {
				return "", invalidRecovery("invalid intent posting reference")
			}

			refs[ref] = true
		}

		primaryProjection := make([]FrozenProjectionIntent, 0, len(transaction.Projection))
		for _, projection := range transaction.Projection {
			if !validProjectionRole(projection.Role) {
				return "", invalidRecovery("invalid intent projection role")
			}

			if projection.Role == engine.RolePrimary {
				primaryProjection = append(primaryProjection, projection)
			}
		}

		intent.Transactions[index].Projection = primaryProjection
	}

	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("encode balance engine intention: %w", err)
	}

	hash := sha256.Sum256(append([]byte("midaz.balance-engine.intent.v1\x00"), encoded...))

	return hex.EncodeToString(hash[:]), nil
}

// DeterministicOperationID derives a UUIDv5 from an immutable namespace and
// length-prefixed fields. Ordinals distinguish rows without relying on map order.
func DeterministicOperationID(executionID, transactionID uuid.UUID, postingRef, role string, ordinal uint32) (uuid.UUID, error) {
	if executionID == uuid.Nil || transactionID == uuid.Nil || postingRef == "" || !validProjectionRole(role) {
		return uuid.Nil, invalidRecovery("invalid operation identity")
	}

	var input []byte
	for _, field := range [][]byte{executionID[:], transactionID[:], []byte(postingRef), []byte(role)} {
		input = binary.BigEndian.AppendUint64(input, uint64(len(field)))
		input = append(input, field...)
	}

	input = binary.BigEndian.AppendUint32(input, ordinal)

	namespace, err := uuid.Parse(BalanceEngineOperationIDNamespaceV1)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse operation namespace: %w", err)
	}

	return uuid.NewSHA1(namespace, input), nil
}

// EncodeBalanceEngineRecoveryPayload validates and freezes a typed payload as JSON.
func EncodeBalanceEngineRecoveryPayload(payload BalanceEngineRecoveryPayload) (json.RawMessage, error) {
	if err := validateRecoveryPayload(payload); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode balance engine recovery payload: %w", err)
	}

	return encoded, nil
}

// DecodeBalanceEngineRecoveryPayload accepts only the supported typed schema.
func DecodeBalanceEngineRecoveryPayload(data []byte) (*BalanceEngineRecoveryPayload, error) {
	var payload BalanceEngineRecoveryPayload
	if err := decodeRecoveryJSON(data, &payload); err != nil {
		return nil, err
	}

	if err := validateRecoveryPayload(payload); err != nil {
		return nil, err
	}

	return &payload, nil
}

// ValidateBalanceEngineRecovery checks request, recovery, and guard correlation.
// The adapter must separately compare the payload tenant with authenticated context.
func ValidateBalanceEngineRecovery(input EngineExecution) error {
	request := input.Request
	if err := validateRecoveryExecutionIdentity(input); err != nil {
		return err
	}

	transactions := make(map[uuid.UUID]engine.Transaction, len(request.Transactions))
	for _, transaction := range request.Transactions {
		if transaction.ID == uuid.Nil {
			return invalidRecovery("missing transaction identity")
		}

		if _, duplicate := transactions[transaction.ID]; duplicate {
			return invalidRecovery("duplicate transaction identity")
		}

		transactions[transaction.ID] = transaction
	}

	seen := make(map[uuid.UUID]bool, len(input.Recovery))
	intents := make(map[uuid.UUID]BalanceEngineTransactionIntent, len(input.Recovery))

	var tenant string

	tenantSet := false

	for _, recovery := range input.Recovery {
		transaction, exists := transactions[recovery.TransactionID]
		if !exists || seen[recovery.TransactionID] {
			return invalidRecovery("unrelated or duplicate recovery transaction")
		}

		seen[recovery.TransactionID] = true

		payload, err := DecodeBalanceEngineRecoveryPayload(recovery.Payload)
		if err != nil {
			return err
		}

		if err := validateRecoveryExecutionScope(input, recovery.TransactionID, payload); err != nil {
			return err
		}

		if tenantSet && tenant != payload.TenantID {
			return invalidRecovery("mixed recovery tenants")
		}

		tenant = payload.TenantID
		tenantSet = true

		if err := validateRecoveryPostings(transaction, payload.Projection); err != nil {
			return err
		}

		if err := validateRecoverySnapshotIdentities(request.Balances, payload.Projection); err != nil {
			return err
		}

		intents[transaction.ID] = recoveryTransactionIntent(transaction, *payload)
	}

	if err := validateRecoveryGuards(input.Guards, transactions); err != nil {
		return err
	}

	return validateFrozenExecutionFingerprint(request, tenant, intents, input.IntentFingerprint)
}

func validateRecoveryGuards(guards []ExecutionGuard, transactions map[uuid.UUID]engine.Transaction) error {
	seen := make(map[uuid.UUID]bool, len(guards))
	for _, guard := range guards {
		if _, exists := transactions[guard.TransactionID]; !exists || seen[guard.TransactionID] || guard.NextToken == "" || guard.ExpectedToken == guard.NextToken {
			return invalidRecovery("invalid execution guard correlation")
		}

		seen[guard.TransactionID] = true
	}

	return nil
}

func validateFrozenExecutionFingerprint(request engine.Request, tenant string, intents map[uuid.UUID]BalanceEngineTransactionIntent, expected string) error {
	intent := BalanceEngineIntent{
		TenantID: tenant, OrganizationID: request.OrganizationID, LedgerID: request.LedgerID, ExecutionID: request.ExecutionID,
		Transactions: make([]BalanceEngineTransactionIntent, 0, len(request.Transactions)),
	}
	for _, transaction := range request.Transactions {
		intent.Transactions = append(intent.Transactions, intents[transaction.ID])
	}

	fingerprint, err := ComputeBalanceEngineIntentFingerprint(intent)
	if err != nil {
		return err
	}

	if fingerprint != expected {
		return invalidRecovery("fingerprint does not match frozen execution intent")
	}

	return nil
}

func validateRecoverySnapshotIdentities(snapshots []engine.BalanceSnapshot, projections []FrozenProjectionContext) error {
	byRef := make(map[string]engine.BalanceSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byRef[snapshot.BalanceRef] = snapshot
	}

	for _, projection := range projections {
		snapshot, exists := byRef[projection.BalanceRef]
		if !exists {
			continue
		}

		balance := projection.Balance
		if balance.ID != snapshot.ID.String() || balance.AccountID != snapshot.AccountID.String() || balance.Key != snapshot.Key || mtransaction.SplitAlias(balance.Alias) != snapshot.Alias || balance.AssetCode != snapshot.AssetCode || balance.AccountType != snapshot.AccountType {
			return invalidRecovery("projection identity does not match execution snapshot")
		}
	}

	return nil
}

func recoveryTransactionIntent(transaction engine.Transaction, payload BalanceEngineRecoveryPayload) BalanceEngineTransactionIntent {
	intent := BalanceEngineTransactionIntent{
		TransactionID: payload.TransactionID, ParentTransactionID: payload.ParentTransactionID,
		FeesSkipped: payload.FeesSkipped, TracerSkipped: payload.TracerSkipped, Action: payload.Action,
		TransactionStatus: payload.TransactionStatus, TransactionDate: payload.TransactionDate, Input: payload.TransactionInput,
		PostingRefs: make([]string, 0, len(transaction.Postings)), Projection: make([]FrozenProjectionIntent, 0, len(payload.Projection)),
	}
	for _, posting := range transaction.Postings {
		intent.PostingRefs = append(intent.PostingRefs, posting.Ref)
	}

	for _, projection := range payload.Projection {
		intent.Projection = append(intent.Projection, projection.Intent())
	}

	return intent
}

func validateRecoveryExecutionIdentity(input EngineExecution) error {
	request := input.Request
	if request.OrganizationID == uuid.Nil || request.LedgerID == uuid.Nil || request.ExecutionID == uuid.Nil || !validIntentFingerprint(input.IntentFingerprint) || len(request.Transactions) == 0 {
		return invalidRecovery("invalid execution identity")
	}

	if len(input.Recovery) != len(request.Transactions) || len(input.Guards) != len(request.Transactions) {
		return invalidRecovery("transactions, recovery and guards must correlate one to one")
	}

	return nil
}

func validateRecoveryExecutionScope(input EngineExecution, transactionID uuid.UUID, payload *BalanceEngineRecoveryPayload) error {
	request := input.Request
	if payload.TransactionID != transactionID || payload.ExecutionID != request.ExecutionID || payload.OrganizationID != request.OrganizationID || payload.LedgerID != request.LedgerID || payload.IntentFingerprint != input.IntentFingerprint {
		return invalidRecovery("recovery scope does not match execution")
	}

	return nil
}

func validateRecoveryPostings(transaction engine.Transaction, projections []FrozenProjectionContext) error {
	postings := make(map[string]engine.Posting, len(transaction.Postings))
	for _, posting := range transaction.Postings {
		if posting.Ref == "" {
			return invalidRecovery("missing posting reference")
		}

		if _, duplicate := postings[posting.Ref]; duplicate {
			return invalidRecovery("duplicate posting reference")
		}

		postings[posting.Ref] = posting
	}

	primaries := make(map[string]bool, len(postings))
	for _, projection := range projections {
		posting, exists := postings[projection.PostingRef]
		if !exists {
			return invalidRecovery("projection references an unrelated posting")
		}

		if projection.Role == engine.RolePrimary {
			if projection.BalanceRef != posting.BalanceRef {
				return invalidRecovery("projection balance does not match posting")
			}

			primaries[projection.PostingRef] = true
		}
	}

	if len(primaries) != len(postings) {
		return invalidRecovery("posting has no primary projection context")
	}

	return nil
}

// EncodeBalanceEngineRecoveryEnvelope validates a completed execution record.
func EncodeBalanceEngineRecoveryEnvelope(envelope BalanceEngineRecoveryEnvelope) (json.RawMessage, error) {
	if err := validateRecoveryEnvelope(envelope); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode balance engine recovery envelope: %w", err)
	}

	return encoded, nil
}

// DecodeBalanceEngineRecoveryEnvelope rejects unrecognized versions and scope drift.
func DecodeBalanceEngineRecoveryEnvelope(data []byte) (*BalanceEngineRecoveryEnvelope, error) {
	var envelope BalanceEngineRecoveryEnvelope
	if err := decodeRecoveryJSON(data, &envelope); err != nil {
		return nil, err
	}

	if err := validateRecoveryEnvelope(envelope); err != nil {
		return nil, err
	}

	return &envelope, nil
}

func validateRecoveryPayload(payload BalanceEngineRecoveryPayload) error {
	if payload.FormatVersion != BalanceEngineRecoveryVersion || payload.TransactionID == uuid.Nil || payload.OrganizationID == uuid.Nil || payload.LedgerID == uuid.Nil || payload.ExecutionID == uuid.Nil || !validIntentFingerprint(payload.IntentFingerprint) {
		return invalidRecovery("invalid payload version or identity")
	}

	if !validRecoveryParent(payload.TransactionID, payload.ParentTransactionID) {
		return invalidRecovery("invalid parent transaction identity")
	}

	if payload.TransactionDate.IsZero() || payload.TTL.IsZero() || payload.Action == "" || payload.TransactionStatus == "" || payload.Projection == nil {
		return invalidRecovery("missing frozen transaction context")
	}

	seen := make(map[uuid.UUID]bool, len(payload.Projection))
	for _, projection := range payload.Projection {
		if err := validateFrozenProjection(payload, projection); err != nil {
			return err
		}

		id, err := DeterministicOperationID(payload.ExecutionID, payload.TransactionID, projection.PostingRef, projection.Role, projection.Ordinal)
		if err != nil || seen[id] {
			return invalidRecovery("duplicate or invalid projection reference")
		}

		seen[id] = true
	}

	contexts, _, err := indexProjectionContexts(payload.Projection)
	if err != nil {
		return err
	}

	return validateProjectionAttribution(contexts)
}

func validateFrozenProjection(payload BalanceEngineRecoveryPayload, projection FrozenProjectionContext) error {
	if projection.TransactionID != payload.TransactionID || projection.BalanceRef == "" || projection.RowType == "" || !projection.RequestedAmount.IsPositive() {
		return invalidRecovery("invalid projection identity or amount")
	}

	if projection.Side != ProjectionSideFrom && projection.Side != ProjectionSideTo {
		return invalidRecovery("invalid projection side")
	}

	if projection.Direction != "debit" && projection.Direction != "credit" {
		return invalidRecovery("invalid row direction")
	}

	switch projection.CompatibilityPath {
	case ProjectionStandard:
	case ProjectionValidatedHoldDebit, ProjectionValidatedHoldReserve, ProjectionValidatedCancelRelease, ProjectionValidatedCancelCredit:
		if projection.OriginRef == "" {
			return invalidRecovery("compatibility projection requires an origin reference")
		}
	default:
		return invalidRecovery("unknown projection compatibility path")
	}

	return validateFrozenProjectionBalance(payload, projection)
}

func validateFrozenProjectionBalance(payload BalanceEngineRecoveryPayload, projection FrozenProjectionContext) error {
	if projection.Balance.OrganizationID != payload.OrganizationID.String() || projection.Balance.LedgerID != payload.LedgerID.String() {
		return invalidRecovery("projection snapshot scope mismatch")
	}

	if projection.Balance.Alias == "" || projection.Balance.Key == "" || projection.Balance.AssetCode == "" || projection.BalanceRef != mtransaction.SplitAlias(projection.Balance.Alias)+"#"+projection.Balance.Key {
		return invalidRecovery("projection logical balance identity mismatch")
	}

	for _, value := range projection.Metadata {
		switch value.(type) {
		case nil, string, bool, json.Number, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		default:
			return invalidRecovery("projection metadata must be flat")
		}
	}

	for _, value := range []string{projection.Balance.ID, projection.Balance.AccountID} {
		parsed, err := uuid.Parse(value)
		if err != nil || parsed == uuid.Nil {
			return invalidRecovery("invalid projection balance identity")
		}
	}

	if projection.Role == engine.RoleOverdraftCompanion && (projection.Balance.Key != "overdraft" || len(projection.Metadata) != 0 || projection.ChartOfAccounts != "") {
		return invalidRecovery("invalid companion projection context")
	}

	return nil
}

func validateRecoveryEnvelope(envelope BalanceEngineRecoveryEnvelope) error {
	if envelope.FormatVersion != BalanceEngineRecoveryVersion || envelope.Result.Movements == nil || envelope.Result.Final == nil {
		return invalidRecovery("invalid envelope version or result shape")
	}

	payload, err := DecodeBalanceEngineRecoveryPayload([]byte(envelope.Payload))
	if err != nil {
		return err
	}

	if envelope.TenantID != payload.TenantID || envelope.OrganizationID != payload.OrganizationID || envelope.LedgerID != payload.LedgerID || envelope.ExecutionID != payload.ExecutionID || envelope.TransactionID != payload.TransactionID || envelope.IntentFingerprint != payload.IntentFingerprint {
		return invalidRecovery("envelope and payload scope mismatch")
	}

	_, err = validateProjectionResult(*payload, envelope.Result)

	return err
}

func validProjectionRole(role string) bool {
	return role == engine.RolePrimary || role == engine.RoleOverdraftCompanion
}

func validRecoveryParent(transactionID uuid.UUID, parentID *uuid.UUID) bool {
	return parentID == nil || (*parentID != uuid.Nil && *parentID != transactionID)
}

func validIntentFingerprint(fingerprint string) bool {
	decoded, err := hex.DecodeString(fingerprint)
	return err == nil && len(decoded) == sha256.Size && fingerprint == strings.ToLower(fingerprint)
}

func invalidRecovery(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidBalanceEngineRecovery, reason)
}

func decodeRecoveryJSON(data []byte, target any) error {
	if !json.Valid(data) || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return invalidRecovery("expected a JSON object")
	}

	tokens := json.NewDecoder(bytes.NewReader(data))
	tokens.UseNumber()

	if err := validateRecoveryJSONValue(tokens); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: decode JSON: %w", ErrInvalidBalanceEngineRecovery, err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return invalidRecovery("unexpected trailing JSON")
	}

	return nil
}

func validateRecoveryJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%w: read JSON token: %w", ErrInvalidBalanceEngineRecovery, err)
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
				return fmt.Errorf("%w: read JSON key: %w", ErrInvalidBalanceEngineRecovery, err)
			}

			key, ok := keyToken.(string)
			if !ok || keys[key] {
				return invalidRecovery("duplicate or invalid JSON object key")
			}

			keys[key] = true
		}

		if err := validateRecoveryJSONValue(decoder); err != nil {
			return err
		}
	}

	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("%w: close JSON value: %w", ErrInvalidBalanceEngineRecovery, err)
	}

	return nil
}
