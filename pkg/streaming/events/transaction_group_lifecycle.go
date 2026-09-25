// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming/v4"
)

// Roles a transaction plays inside a cross-ledger group. An origin part sends
// value out of its ledger through that ledger's @external/<asset> bridge; a
// destination part receives it through its own bridge. A participant whose
// legs net to zero inside its ledger owns source funds and is an origin.
const (
	TransactionGroupRoleOrigin      = "origin"
	TransactionGroupRoleDestination = "destination"
)

// TransactionGroupPostedDefinition is the routing contract for
// transaction_group.posted: one direct /v2 request that spanned ledgers was
// applied atomically.
//
// Emission anchor: CreateCrossLedgerTransactionV2 in
// components/ledger/internal/services/command, once the grouped accounting
// execution has been applied and its completion has returned. A replayed
// request emits nothing. The per-part transaction.posted events still fire, one
// per ledger; this is the single fact that says the movement as a whole closed.
var TransactionGroupPostedDefinition = Definition{
	ResourceType:  "transaction_group",
	EventType:     "posted",
	SchemaVersion: "1.0.0",
}

// TransactionGroupCommittedDefinition is the routing contract for
// transaction_group.committed: a cross-ledger hold was committed, approving
// every origin and creating every destination in one execution.
//
// Emission anchor: whichever writer moves the durable group row from PENDING to
// APPROVED — the commit coordinator, or the transaction-group reconciler when
// the coordinator did not reach that step.
var TransactionGroupCommittedDefinition = Definition{
	ResourceType:  "transaction_group",
	EventType:     "committed",
	SchemaVersion: "1.0.0",
}

// TransactionGroupCanceledDefinition is the routing contract for
// transaction_group.canceled: a cross-ledger hold was canceled, releasing every
// origin without creating a destination.
//
// Emission anchor: same rule as TransactionGroupCommittedDefinition, for the
// PENDING to CANCELED move.
var TransactionGroupCanceledDefinition = Definition{
	ResourceType:  "transaction_group",
	EventType:     "canceled",
	SchemaVersion: "1.0.0",
}

// TransactionGroupRevertedDefinition is the routing contract for
// transaction_group.reverted: every part of an approved group was reversed in
// one execution under a new group.
//
// Emission anchor: the grouped revert, once its execution has been applied and
// completion has returned. groupId is the new group; revertedGroupId is the
// group it reverses. A replayed revert emits nothing.
var TransactionGroupRevertedDefinition = Definition{
	ResourceType:  "transaction_group",
	EventType:     "reverted",
	SchemaVersion: "1.0.0",
}

// TransactionGroupPayload is the shared wire payload of the four
// transaction_group events. Only the routing DefinitionKey differs between them.
//
// Parts lists the transactions the operation materialized, in execution order:
// a cancel carries only its origins, because destinations are never created.
// LedgerCount is the number of distinct ledgers among those parts.
type TransactionGroupPayload struct {
	GroupID         string                 `json:"groupId"`
	RevertedGroupID *string                `json:"revertedGroupId,omitempty"`
	Status          string                 `json:"status"`
	AssetCode       string                 `json:"assetCode"`
	LedgerCount     int                    `json:"ledgerCount"`
	Parts           []TransactionGroupPart `json:"parts"`
	OccurredAt      string                 `json:"occurredAt"`
}

// TransactionGroupPart is one ledger-scoped transaction of the group.
type TransactionGroupPart struct {
	TransactionID  string `json:"transactionId"`
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	Role           string `json:"role"`
	Status         string `json:"status"`
}

// TransactionGroupSource carries the wire-ready fields the constructor needs.
// The caller assembles it from the internal transaction rows, which keeps this
// package decoupled from components/ledger/internal.
type TransactionGroupSource struct {
	GroupID         string
	RevertedGroupID *string
	Status          string
	AssetCode       string
	Parts           []TransactionGroupPartSource
	OccurredAt      time.Time
}

// TransactionGroupPartSource is the caller-side shape of one group part.
type TransactionGroupPartSource struct {
	TransactionID  string
	OrganizationID string
	LedgerID       string
	Role           string
	Status         string
}

// NewTransactionGroup maps a TransactionGroupSource into the wire payload
// shared by the four transaction_group events.
func NewTransactionGroup(src TransactionGroupSource) TransactionGroupPayload {
	parts := make([]TransactionGroupPart, len(src.Parts))
	ledgers := make(map[string]struct{}, len(src.Parts))

	for index, part := range src.Parts {
		parts[index] = TransactionGroupPart(part)
		ledgers[part.OrganizationID+"/"+part.LedgerID] = struct{}{}
	}

	return TransactionGroupPayload{
		GroupID:         src.GroupID,
		RevertedGroupID: src.RevertedGroupID,
		Status:          src.Status,
		AssetCode:       src.AssetCode,
		LedgerCount:     len(ledgers),
		Parts:           parts,
		OccurredAt:      src.OccurredAt.Format(time.RFC3339),
	}
}

// ToEmitRequestPosted assembles the EmitRequest for transaction_group.posted.
// Subject is the group id.
func (p TransactionGroupPayload) ToEmitRequestPosted(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(TransactionGroupPostedDefinition, tenantID, ts)
}

// ToEmitRequestCommitted assembles the EmitRequest for
// transaction_group.committed. Subject is the group id.
func (p TransactionGroupPayload) ToEmitRequestCommitted(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(TransactionGroupCommittedDefinition, tenantID, ts)
}

// ToEmitRequestCanceled assembles the EmitRequest for
// transaction_group.canceled. Subject is the group id.
func (p TransactionGroupPayload) ToEmitRequestCanceled(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(TransactionGroupCanceledDefinition, tenantID, ts)
}

// ToEmitRequestReverted assembles the EmitRequest for
// transaction_group.reverted. Subject is the new group id; consumers correlate
// to the reversed group through revertedGroupId.
func (p TransactionGroupPayload) ToEmitRequestReverted(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(TransactionGroupRevertedDefinition, tenantID, ts)
}

func (p TransactionGroupPayload) toEmitRequest(def Definition, tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", def.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: def.Key(),
		TenantID:      tenantID,
		Subject:       p.GroupID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
