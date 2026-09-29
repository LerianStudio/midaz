// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tracer holds the ledger-side client for the tracer service's
// two-phase reservation API (reserve, then confirm or release). The transport
// is gRPC behind the TracerReserver port. Service identity is mutual TLS, so
// the client carries no static shared secret; the tenant travels as trusted
// x-tenant-id metadata over the mTLS-verified connection.
package tracer

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// defaultOperationTimeout is the per-operation context timeout applied when
// the caller does not configure one via WithGRPCOperationTimeout. It mirrors
// the tracer.timeoutMs default (250ms) so a misconfigured client still fails
// fast rather than holding the transaction create path open.
const defaultOperationTimeout = 250 * time.Millisecond

// tenantMetadataKey is the gRPC outgoing-metadata key carrying the trusted
// tenant id. The tracer reads the same key from incoming metadata; it trusts
// the value because the connection is mTLS-verified (the peer is a known
// service).
const tenantMetadataKey = "x-tenant-id"

// ErrTracerUnavailable is the typed error returned when the reservation
// transport fails for an availability reason — a per-operation timeout, a
// transport error, or an open circuit breaker. The reserve anchor
// branches on this with the ledger's tracer.failPosture: open proceeds
// (records SKIPPED), closed rejects. It is intentionally distinct from a
// reservation DENIED decision (a successful reserve with denied=true), which is
// a business outcome the anchor handles separately.
var ErrTracerUnavailable = errors.New("tracer reservation service unavailable")

// ErrTracerRejected is the typed error returned when the tracer refuses the
// reserve request itself — a gRPC InvalidArgument or FailedPrecondition. The
// tracer is up and answered; the request it received cannot be evaluated. It
// is distinct from ErrTracerUnavailable so the anchor does not route a refusal
// through tracer.failPosture, and distinct from a denied decision, which is a
// successful response.
var ErrTracerRejected = errors.New("tracer rejected the reservation request")

// ReserveAccount is the account scope the tracer matches limits and rules
// against. The ledger populates AccountID with the source balance's account
// UUID and Type with that account's free-form type; the tracer's account
// status is left empty, which it treats as an unconstrained optional field.
type ReserveAccount struct {
	// AccountID is empty when the ledger has no internal source account (an
	// external-only source). The tracer treats an empty account id as an absent
	// account, which the relaxed reserve validation accepts.
	AccountID string `json:"accountId,omitempty"`
	// Type is the ledger account type, verbatim. Optional.
	Type string `json:"type,omitempty"`
}

// ReserveRequest is the reserve input the ledger sends the tracer. It is typed
// independently of the tracer's internal model so the tracer's domain
// evolution does not leak onto the ledger's outbound contract; the gRPC client
// maps it field-for-field onto the proto reserve message. The reserve anchor
// populates it from the fee-inclusive transaction state; the client only
// transports it.
//
// The tracer's reserve validation requires requestId, a positive amount, a
// valid asset code (1 to 100 uppercase letters), and a transactionTimestamp that
// is not in the future. account.accountId is OPTIONAL on the relaxed reserve
// path: an external-only source omits it and the tracer accepts the accountless
// request.
// transactionType is OPTIONAL too (the ledger has no card-rail nature to
// honestly report; when empty the tracer matches account-scoped limits without
// a transaction-type constraint).
type ReserveRequest struct {
	TransactionID uuid.UUID      `json:"transactionId"`
	RequestID     string         `json:"requestId"`
	Amount        string         `json:"amount"`
	Asset         string         `json:"asset"`
	Account       ReserveAccount `json:"account"`
	SegmentID     string         `json:"segmentId,omitempty"`
	PortfolioID   string         `json:"portfolioId,omitempty"`
	MerchantID    string         `json:"merchantId,omitempty"`
	// TransactionType is optional on reserve. When set it must be a valid
	// tracer transaction type; the ledger leaves it empty.
	TransactionType string `json:"transactionType,omitempty"`
	// TransactionTimestamp is RFC3339; the tracer rejects a future timestamp
	// against its injected clock and does not bound its age on reserve.
	TransactionTimestamp string `json:"transactionTimestamp"`
	// LongLived hints the tracer to assign a long-lived reservation lifetime to
	// a PENDING-transaction reservation.
	LongLived bool `json:"longLived,omitempty"`
	// Metadata is the transaction's flat metadata. The tracer accepts keys
	// matching ^[a-zA-Z0-9_]+$, at most 64 characters, and at most 50 entries.
	Metadata map[string]string `json:"metadata,omitempty"`
	// Revert marks the reservation as the revert of an applied transaction. The
	// tracer skips rule evaluation for it and still reserves limit capacity.
	Revert bool `json:"revert,omitempty"`
}

// ReserveResult is the handle returned by a successful reserve. Denied is the
// refusal flag (no capacity held, ReservationIDs empty). Otherwise
// ReservationIDs holds one id per counter-backed limit the ledger must later
// confirm or release. Decision (ALLOW, DENY or REVIEW), Reason and
// MatchedRuleIDs refine Denied; a tracer that predates them leaves them zero,
// and Denied stays authoritative either way.
type ReserveResult struct {
	TransactionID  uuid.UUID   `json:"transactionId"`
	Denied         bool        `json:"denied"`
	Decision       string      `json:"decision"`
	Reason         string      `json:"reason,omitempty"`
	MatchedRuleIDs []uuid.UUID `json:"matchedRuleIds"`
	ReservationIDs []uuid.UUID `json:"reservationIds"`
}
