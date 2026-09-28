// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	// TransactionWriteBehindFormatVersion identifies the projection envelope.
	// TransactionCompletionFormatVersion remains the version of its embedded
	// immutable accounting evidence.
	TransactionWriteBehindFormatVersion = 1

	TransactionApplicationConfirmed = "confirmed"

	TransactionReplayReconstructible = "reconstructible"
	TransactionReplayMaterialized    = "materialized"

	TransactionDurabilityPending  = "pending"
	TransactionDurabilityComplete = "complete"

	TransactionDependencyPredecessor = "predecessor"
	TransactionDependencyOrigin      = "origin"

	TransactionEvidenceIndexFormatVersion = 1
)

// TransactionEvidenceReference identifies an immutable execution on which a
// projection depends. Scope is repeated deliberately: consumers validate it
// before resolving evidence and never inherit tenant or ledger identity from a
// broker route or caller-provided body.
type TransactionEvidenceReference struct {
	Kind           string    `json:"kind"`
	TenantID       string    `json:"tenantId"`
	OrganizationID uuid.UUID `json:"organizationId"`
	LedgerID       uuid.UUID `json:"ledgerId"`
	TransactionID  uuid.UUID `json:"transactionId"`
	ExecutionID    uuid.UUID `json:"executionId"`
}

// TransactionWriteBehindEnvelope separates the three facts that used to be
// inferred from one synchronous completion call: accounting is confirmed,
// replay is available or reconstructible, and the SQL/Mongo projection is
// pending or complete. Record is immutable evidence produced by the engine.
type TransactionWriteBehindEnvelope struct {
	FormatVersion    int                            `json:"formatVersion"`
	ApplicationState string                         `json:"applicationState"`
	ReplayState      string                         `json:"replayState"`
	DurabilityState  string                         `json:"durabilityState"`
	Record           TransactionCompletionRecord    `json:"record"`
	Dependencies     []TransactionEvidenceReference `json:"dependencies"`
}

// TransactionEvidenceIndex is the bounded lookup pointer published atomically
// with accounting. It contains no HTTP DTO: readers resolve the immutable
// record and receipt named here, then compose the requested representation in
// Go.
type TransactionEvidenceIndex struct {
	FormatVersion         int                            `json:"formatVersion"`
	TenantID              string                         `json:"tenantId"`
	OrganizationID        uuid.UUID                      `json:"organizationId"`
	LedgerID              uuid.UUID                      `json:"ledgerId"`
	TransactionID         uuid.UUID                      `json:"transactionId"`
	ExecutionID           uuid.UUID                      `json:"executionId"`
	Action                string                         `json:"action"`
	ApplicationState      string                         `json:"applicationState"`
	ReplayState           string                         `json:"replayState"`
	DurabilityState       string                         `json:"durabilityState"`
	RecoveryField         string                         `json:"recoveryField"`
	ReceiptField          string                         `json:"receiptField"`
	ReceiptOrganizationID *uuid.UUID                     `json:"receiptOrganizationId,omitempty"`
	ReceiptLedgerID       *uuid.UUID                     `json:"receiptLedgerId,omitempty"`
	Dependencies          []TransactionEvidenceReference `json:"dependencies"`
}

func EncodeTransactionEvidenceIndex(index TransactionEvidenceIndex) (json.RawMessage, error) {
	if err := validateTransactionEvidenceIndex(index); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("encode transaction evidence index: %w", err)
	}

	return encoded, nil
}

func DecodeTransactionEvidenceIndex(data []byte) (*TransactionEvidenceIndex, error) {
	fields, err := decodeTransactionWriteBehindFields(data)
	if err != nil {
		return nil, err
	}

	data, err = normalizeLegacyEmptyDependenciesObject(data, fields)
	if err != nil {
		return nil, err
	}

	var index TransactionEvidenceIndex
	if err := decodeTransactionCompletionJSON(data, &index); err != nil {
		return nil, err
	}

	if err := validateTransactionEvidenceIndex(index); err != nil {
		return nil, err
	}

	return &index, nil
}

// EncodeTransactionWriteBehindEnvelope emits only the new versioned wrapper.
// Legacy completion records remain a read-only compatibility format.
func EncodeTransactionWriteBehindEnvelope(envelope TransactionWriteBehindEnvelope) (json.RawMessage, error) {
	if err := validateTransactionWriteBehindEnvelope(envelope); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode transaction write-behind envelope: %w", err)
	}

	return encoded, nil
}

// DecodeTransactionWriteBehindEnvelope accepts the new wrapper and the legacy
// version-two completion record. Legacy evidence is normalized in memory to a
// confirmed, reconstructible, pending envelope; callers must not write that
// synthesized wrapper back as proof that a legacy publisher emitted it.
func DecodeTransactionWriteBehindEnvelope(data []byte) (*TransactionWriteBehindEnvelope, error) {
	fields, err := decodeTransactionWriteBehindFields(data)
	if err != nil {
		return nil, err
	}

	if len(fields["record"]) == 0 {
		legacy, err := DecodeTransactionCompletionRecord(data)
		if err != nil {
			return nil, err
		}

		return &TransactionWriteBehindEnvelope{
			FormatVersion:    TransactionWriteBehindFormatVersion,
			ApplicationState: TransactionApplicationConfirmed,
			ReplayState:      TransactionReplayReconstructible,
			DurabilityState:  TransactionDurabilityPending,
			Record:           *legacy,
			Dependencies:     []TransactionEvidenceReference{},
		}, nil
	}

	data, err = normalizeLegacyEmptyDependenciesObject(data, fields)
	if err != nil {
		return nil, err
	}

	var envelope TransactionWriteBehindEnvelope
	if err := decodeTransactionCompletionJSON(data, &envelope); err != nil {
		return nil, err
	}

	if err := validateTransactionWriteBehindEnvelope(envelope); err != nil {
		return nil, err
	}

	return &envelope, nil
}

func decodeTransactionWriteBehindFields(data []byte) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return nil, invalidTransactionCompletionRecord("expected a JSON object")
	}

	tokens := json.NewDecoder(bytes.NewReader(trimmed))
	tokens.UseNumber()

	if err := validateCompletionJSONValue(tokens); err != nil {
		return nil, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil, fmt.Errorf("%w: decode JSON fields: %w", ErrInvalidTransactionCompletionRecord, err)
	}

	return fields, nil
}

// normalizeLegacyEmptyDependenciesObject accepts only the shape emitted by the
// former Valkey ACK round-trip, where cjson encoded an empty array as {}. New
// writers always emit an array; objects with members remain invalid evidence.
func normalizeLegacyEmptyDependenciesObject(data []byte, fields map[string]json.RawMessage) ([]byte, error) {
	raw, exists := fields["dependencies"]
	if !exists || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return data, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("%w: decode dependencies compatibility object: %w", ErrInvalidTransactionCompletionRecord, err)
	}

	if len(object) != 0 {
		return nil, invalidTransactionCompletionRecord("dependencies compatibility object is not empty")
	}

	fields["dependencies"] = json.RawMessage(`[]`)

	normalized, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("normalize transaction dependencies: %w", err)
	}

	return normalized, nil
}

func validateTransactionWriteBehindEnvelope(envelope TransactionWriteBehindEnvelope) error {
	if envelope.FormatVersion != TransactionWriteBehindFormatVersion {
		return invalidTransactionCompletionRecord("unsupported write-behind envelope version")
	}

	if envelope.ApplicationState != TransactionApplicationConfirmed {
		return invalidTransactionCompletionRecord("write-behind application is not confirmed")
	}

	if envelope.ReplayState != TransactionReplayReconstructible && envelope.ReplayState != TransactionReplayMaterialized {
		return invalidTransactionCompletionRecord("invalid write-behind replay state")
	}

	if envelope.DurabilityState != TransactionDurabilityPending && envelope.DurabilityState != TransactionDurabilityComplete {
		return invalidTransactionCompletionRecord("invalid write-behind durability state")
	}

	if err := validateTransactionCompletionRecord(envelope.Record); err != nil {
		return err
	}

	return validateTransactionEvidenceReferences(envelope.Record, envelope.Dependencies)
}

// validateTransactionEvidenceReferences correlates every declared causal
// reference with the record it travels on. An origin reference is optional: a
// parent transaction whose evidence was already reaped after a durable write
// has no execution left to reference, and a reversal of it must still post.
//
//nolint:gocyclo // dependency validation keeps every cross-scope, self-reference, and causal rule explicit
func validateTransactionEvidenceReferences(record TransactionCompletionRecord, dependencies []TransactionEvidenceReference) error {
	payload, err := DecodeTransactionCompletionPlan([]byte(record.Payload))
	if err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(dependencies))
	kinds := make(map[string]struct{}, len(dependencies))

	for _, dependency := range dependencies {
		if dependency.Kind != TransactionDependencyPredecessor && dependency.Kind != TransactionDependencyOrigin {
			return invalidTransactionCompletionRecord("invalid write-behind dependency kind")
		}

		if dependency.TenantID != record.TenantID || dependency.OrganizationID != record.OrganizationID || dependency.LedgerID != record.LedgerID {
			return invalidTransactionCompletionRecord("write-behind dependency scope mismatch")
		}

		if dependency.TransactionID == uuid.Nil || dependency.ExecutionID == uuid.Nil {
			return invalidTransactionCompletionRecord("missing write-behind dependency identity")
		}

		if dependency.TransactionID == record.TransactionID && dependency.ExecutionID == record.ExecutionID {
			return invalidTransactionCompletionRecord("cyclic write-behind dependency")
		}

		key := dependency.Kind + ":" + dependency.TransactionID.String() + ":" + dependency.ExecutionID.String()
		if _, duplicate := seen[key]; duplicate {
			return invalidTransactionCompletionRecord("duplicate write-behind dependency")
		}

		seen[key] = struct{}{}

		if _, duplicateKind := kinds[dependency.Kind]; duplicateKind {
			return invalidTransactionCompletionRecord("ambiguous write-behind dependency")
		}

		kinds[dependency.Kind] = struct{}{}

		switch dependency.Kind {
		case TransactionDependencyPredecessor:
			if dependency.TransactionID != record.TransactionID {
				return invalidTransactionCompletionRecord("predecessor transaction correlation mismatch")
			}
		case TransactionDependencyOrigin:
			if payload.ParentTransactionID == nil || dependency.TransactionID != *payload.ParentTransactionID || dependency.TransactionID == record.TransactionID {
				return invalidTransactionCompletionRecord("origin transaction correlation mismatch")
			}
		}
	}

	return nil
}

//nolint:gocyclo,gocognit // versioned index validation rejects each malformed identity and reference combination explicitly
func validateTransactionEvidenceIndex(index TransactionEvidenceIndex) error {
	if index.FormatVersion != TransactionEvidenceIndexFormatVersion || index.OrganizationID == uuid.Nil || index.LedgerID == uuid.Nil || index.TransactionID == uuid.Nil || index.ExecutionID == uuid.Nil || index.Action == "" {
		return invalidTransactionCompletionRecord("invalid transaction evidence index identity")
	}

	if index.ApplicationState != TransactionApplicationConfirmed ||
		(index.ReplayState != TransactionReplayReconstructible && index.ReplayState != TransactionReplayMaterialized) ||
		(index.DurabilityState != TransactionDurabilityPending && index.DurabilityState != TransactionDurabilityComplete) {
		return invalidTransactionCompletionRecord("invalid transaction evidence index state")
	}

	if index.RecoveryField != index.TransactionID.String()+":"+index.ExecutionID.String() || index.ReceiptField != index.ExecutionID.String() || len(index.Dependencies) > 2 {
		return invalidTransactionCompletionRecord("invalid transaction evidence index correlation")
	}

	if (index.ReceiptOrganizationID == nil) != (index.ReceiptLedgerID == nil) ||
		(index.ReceiptOrganizationID != nil && (*index.ReceiptOrganizationID == uuid.Nil || *index.ReceiptLedgerID == uuid.Nil)) {
		return invalidTransactionCompletionRecord("invalid transaction evidence receipt scope")
	}

	seenKinds := make(map[string]struct{}, len(index.Dependencies))

	seenIdentities := make(map[string]struct{}, len(index.Dependencies))
	for _, dependency := range index.Dependencies {
		if dependency.Kind != TransactionDependencyPredecessor && dependency.Kind != TransactionDependencyOrigin {
			return invalidTransactionCompletionRecord("invalid transaction evidence index dependency kind")
		}

		if dependency.TenantID != index.TenantID || dependency.OrganizationID != index.OrganizationID || dependency.LedgerID != index.LedgerID || dependency.TransactionID == uuid.Nil || dependency.ExecutionID == uuid.Nil {
			return invalidTransactionCompletionRecord("transaction evidence index dependency scope mismatch")
		}

		if dependency.TransactionID == index.TransactionID && dependency.ExecutionID == index.ExecutionID {
			return invalidTransactionCompletionRecord("cyclic transaction evidence index dependency")
		}

		if dependency.Kind == TransactionDependencyPredecessor && dependency.TransactionID != index.TransactionID {
			return invalidTransactionCompletionRecord("transaction evidence index predecessor mismatch")
		}

		if dependency.Kind == TransactionDependencyOrigin && dependency.TransactionID == index.TransactionID {
			return invalidTransactionCompletionRecord("transaction evidence index origin mismatch")
		}

		identity := dependency.Kind + ":" + dependency.TransactionID.String() + ":" + dependency.ExecutionID.String()
		if _, exists := seenKinds[dependency.Kind]; exists {
			return invalidTransactionCompletionRecord("ambiguous transaction evidence index dependency")
		}

		if _, exists := seenIdentities[identity]; exists {
			return invalidTransactionCompletionRecord("duplicate transaction evidence index dependency")
		}

		seenKinds[dependency.Kind] = struct{}{}
		seenIdentities[identity] = struct{}{}
	}

	return nil
}
