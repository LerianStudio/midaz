// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import (
	"encoding/json"
	"fmt"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// TransactionPostedDefinition is the routing contract for
// transaction.posted.
//
// Emission anchor: components/ledger/internal/services/command/send_transaction_events.go,
// inside SendTransactionEvents alongside the legacy
// transaction.transaction_events rabbit publish. Fires when a freshly
// created transaction (no parent) has been committed to PostgreSQL AND
// all its operations have been persisted. Distinguished from
// transaction.reverted by tran.ParentTransactionID == nil.
//
// Anchor placement note: the original instrumentation map pinned this
// event to CreateOrUpdateTransaction:193, but emitting there would fire
// BEFORE the operations loop persists operation rows
// (create_balance_transaction_operations_async.go:115-148). The
// SendTransactionEvents call site at L150 of the same file fires only
// after the full ops loop succeeds, which matches the safety contract
// the legacy rabbit publish has always honoured. Single-transaction
// async, single-transaction sync (write_transaction.go:154 →
// CreateBalanceTransactionOperationsAsync), and the bulk path
// (create_bulk_transaction_operations_async.go:555) all reach
// SendTransactionEvents, so this anchor covers every code path.
//
// Cutover window: this event coexists with the legacy
// transaction.transaction_events rabbit publish. The rabbit publish is
// removed in a follow-up task once consumer migration is verified. The
// disabled flag RABBITMQ_TRANSACTION_EVENTS_ENABLED=false short-circuits
// BOTH transports during cutover.
//
// Posture note: the catalog marks this event CRITICAL (outbox: always,
// direct: skip). The outbox subsystem is NOT yet wired in midaz, so
// emission today goes through pkgStreaming.EmitImportant (direct emit,
// no atomic guarantee). When the outbox lands the call site is the only
// place that needs to switch helpers; the Definition stays unchanged.
var TransactionPostedDefinition = Definition{
	ResourceType:  "transaction",
	EventType:     "posted",
	SchemaVersion: "1.0.0",
}

// TransactionCommittedDefinition is the routing contract for
// transaction.committed.
//
// Emission anchor: same as TransactionPostedDefinition. Fires when a
// PENDING transaction transitions PENDING → APPROVED via the
// unique-violation idempotency branch of CreateOrUpdateTransaction
// (UpdateTransactionStatus call at L198). Discriminated from
// transaction.posted by the lifecycle phase tracked through
// CreateOrUpdateTransaction's return value: phase=="updated" + status
// APPROVED → committed; phase=="created" + status APPROVED → posted.
//
// Same cutover and posture caveats as TransactionPostedDefinition.
var TransactionCommittedDefinition = Definition{
	ResourceType:  "transaction",
	EventType:     "committed",
	SchemaVersion: "1.0.0",
}

// TransactionCanceledDefinition is the routing contract for
// transaction.canceled.
//
// Emission anchor: same as TransactionCommittedDefinition. Fires when a
// PENDING transaction transitions PENDING → CANCELED via the
// unique-violation idempotency branch's UpdateTransactionStatus call.
// Same anchor as transaction.committed but distinguished by the
// terminal status code.
//
// Same cutover and posture caveats as TransactionPostedDefinition.
var TransactionCanceledDefinition = Definition{
	ResourceType:  "transaction",
	EventType:     "canceled",
	SchemaVersion: "1.0.0",
}

// TransactionRevertedDefinition is the routing contract for
// transaction.reverted.
//
// Emission anchor: same as TransactionPostedDefinition. Fires when a
// revert flow creates a child transaction. Distinguished from
// transaction.posted by tran.ParentTransactionID != nil (the child
// carries the parent's UUID). The revert HTTP handler at
// transaction_state_handlers.go:166-289 (RevertTransaction) flows
// through the standard write path and lands at the same
// SendTransactionEvents anchor.
//
// The new child transaction id is the idempotency key; consumers
// correlate to the original via parentTransactionId.
//
// Same cutover and posture caveats as TransactionPostedDefinition.
var TransactionRevertedDefinition = Definition{
	ResourceType:  "transaction",
	EventType:     "reverted",
	SchemaVersion: "1.0.0",
}

// TransactionPayload is the shared wire payload for the four
// transaction lifecycle events (posted, committed, canceled, reverted).
// All four events carry an identical field set — only the routing
// DefinitionKey differs. Following the BalanceOverdraftPayload precedent,
// a single shared type with per-event constructors avoids duplicating
// four near-identical mirror structs.
//
// Field shape note: this payload uses [][]byte (json.RawMessage) for
// Operations so the wire shape carries each operation's full JSON
// verbatim without coupling the events package to the
// components/ledger/internal/adapters/postgres/operation domain type
// (which lives behind an internal/ boundary). The caller —
// emitTransactionLifecycleEvent in services/command/send_transaction_events.go
// — marshals each *operation.Operation once and passes the RawMessage
// slice in. The wire bytes are byte-identical to what the legacy
// transaction.transaction_events rabbit publish emits (which also
// marshals operations as-is), easing consumer migration during the
// cutover window.
//
// Scale is intentionally OMITTED for the same reason as the balance.*
// payloads: it is an asset-level property, not a transaction-level one.
// Consumers needing scale should join against the asset.created event.
//
// ParentTransactionID is `*string` with omitempty so the wire shape
// distinguishes "no parent" (field absent) from a valid parent UUID.
// transaction.posted, transaction.committed and transaction.canceled
// will all omit this field; transaction.reverted will always populate
// it.
//
// Amount is `*decimal.Decimal` because the underlying Transaction.Amount
// is also a pointer (some PENDING transactions can have unset amount
// until the operations resolve). omitempty drops the field when nil.
type TransactionPayload struct {
	ID                       string            `json:"id"`
	ParentTransactionID      *string           `json:"parentTransactionId,omitempty"`
	OrganizationID           string            `json:"organizationId"`
	LedgerID                 string            `json:"ledgerId"`
	Status                   mmodel.Status     `json:"status"`
	Amount                   *decimal.Decimal  `json:"amount,omitempty"`
	AssetCode                string            `json:"assetCode"`
	ChartOfAccountsGroupName string            `json:"chartOfAccountsGroupName,omitempty"`
	Description              string            `json:"description,omitempty"`
	Source                   []string          `json:"source,omitempty"`
	Destination              []string          `json:"destination,omitempty"`
	Route                    string            `json:"route,omitempty"`
	RouteID                  *string           `json:"routeId,omitempty"`
	Operations               []json.RawMessage `json:"operations"`
	Metadata                 map[string]any    `json:"metadata,omitempty"`
	CreatedAt                string            `json:"createdAt"`
	UpdatedAt                string            `json:"updatedAt"`
}

// TransactionSource carries the wire-ready fields a constructor needs to
// build a TransactionPayload. The caller — in services/command, which
// has direct access to the internal domain Transaction / Operation
// types — assembles this struct (notably pre-marshaling each operation
// into RawMessage) and passes it to the per-event constructor. Keeping
// the marshaling in the caller is what lets the events package stay
// decoupled from components/ledger/internal/.
//
// Status uses mmodel.Status (publicly exposed) which is shape-compatible
// with the transaction.Status domain type via shared JSON tags.
type TransactionSource struct {
	ID                       string
	ParentTransactionID      *string
	OrganizationID           string
	LedgerID                 string
	Status                   mmodel.Status
	Amount                   *decimal.Decimal
	AssetCode                string
	ChartOfAccountsGroupName string
	Description              string
	Source                   []string
	Destination              []string
	Route                    string
	RouteID                  *string
	Operations               []json.RawMessage
	Metadata                 map[string]any
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// newTransactionPayload is the shared mapping from a TransactionSource
// into the wire payload. The four per-event constructors share this
// implementation because the field set is identical — distinguishing
// the four events is done by the constructor's corresponding Definition
// (and ToEmitRequest method), not by mutating the payload.
//
// Caller invariant: src must reflect the post-commit state of the
// transaction (id / createdAt / updatedAt already assigned by the DB).
func newTransactionPayload(src TransactionSource) TransactionPayload {
	return TransactionPayload{
		ID:                       src.ID,
		ParentTransactionID:      src.ParentTransactionID,
		OrganizationID:           src.OrganizationID,
		LedgerID:                 src.LedgerID,
		Status:                   src.Status,
		Amount:                   src.Amount,
		AssetCode:                src.AssetCode,
		ChartOfAccountsGroupName: src.ChartOfAccountsGroupName,
		Description:              src.Description,
		Source:                   src.Source,
		Destination:              src.Destination,
		Route:                    src.Route,
		RouteID:                  src.RouteID,
		Operations:               src.Operations,
		Metadata:                 src.Metadata,
		CreatedAt:                src.CreatedAt.Format(time.RFC3339),
		UpdatedAt:                src.UpdatedAt.Format(time.RFC3339),
	}
}

// NewTransactionPosted maps a TransactionSource into the wire payload
// for transaction.posted. Caller invariant: src.ParentTransactionID
// must be nil — the constructor does not enforce this because the
// caller (emitTransactionLifecycleEvent) already discriminates on the
// parent pointer to pick this constructor.
func NewTransactionPosted(src TransactionSource) TransactionPayload {
	return newTransactionPayload(src)
}

// NewTransactionCommitted maps a TransactionSource into the wire payload
// for transaction.committed. Caller invariant: src.Status.Code must be
// APPROVED and the transaction must have been reached via the
// unique-violation idempotency branch.
func NewTransactionCommitted(src TransactionSource) TransactionPayload {
	return newTransactionPayload(src)
}

// NewTransactionCanceled maps a TransactionSource into the wire payload
// for transaction.canceled. Caller invariant: src.Status.Code must be
// CANCELED and the transaction must have been reached via the
// unique-violation idempotency branch.
func NewTransactionCanceled(src TransactionSource) TransactionPayload {
	return newTransactionPayload(src)
}

// NewTransactionReverted maps a TransactionSource into the wire payload
// for transaction.reverted. Caller invariant: src.ParentTransactionID
// must be non-nil — the child transaction in a revert flow always
// carries the parent's UUID.
func NewTransactionReverted(src TransactionSource) TransactionPayload {
	return newTransactionPayload(src)
}

// ToEmitRequestPosted assembles a libStreaming.EmitRequest for
// transaction.posted. Subject is t.ID (the canonical idempotency key
// per the instrumentation map: consumer_idempotency_key_candidate="id").
func (p TransactionPayload) ToEmitRequestPosted(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(TransactionPostedDefinition, tenantID, ts)
}

// ToEmitRequestCommitted assembles a libStreaming.EmitRequest for
// transaction.committed. Subject is t.ID; consumer dedup key per the
// instrumentation map is id+updatedAt — consumers compose this from
// payload fields rather than from Subject.
func (p TransactionPayload) ToEmitRequestCommitted(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(TransactionCommittedDefinition, tenantID, ts)
}

// ToEmitRequestCanceled assembles a libStreaming.EmitRequest for
// transaction.canceled. Subject is t.ID; consumer dedup key per the
// instrumentation map is id+updatedAt.
func (p TransactionPayload) ToEmitRequestCanceled(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(TransactionCanceledDefinition, tenantID, ts)
}

// ToEmitRequestReverted assembles a libStreaming.EmitRequest for
// transaction.reverted. Subject is t.ID (the child transaction's UUID);
// consumers correlate to the original via payload.parentTransactionId.
func (p TransactionPayload) ToEmitRequestReverted(tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	return p.toEmitRequest(TransactionRevertedDefinition, tenantID, ts)
}

func (p TransactionPayload) toEmitRequest(def Definition, tenantID string, ts time.Time) (libStreaming.EmitRequest, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return libStreaming.EmitRequest{}, fmt.Errorf("marshal %s payload: %w", def.Key(), err)
	}

	return libStreaming.EmitRequest{
		DefinitionKey: def.Key(),
		TenantID:      tenantID,
		Subject:       p.ID,
		Timestamp:     ts,
		Payload:       data,
	}, nil
}
