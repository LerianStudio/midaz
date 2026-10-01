// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tracer holds the ledger-side client for the tracer service's
// two-phase reservation API (reserve, then confirm or release). The transport
// is gRPC behind the TracerReserver port. The ledger identifies itself on the
// seam with at most one credential: its Access Manager application token (per
// tenant in multi-tenant mode), or the tracer API key; with neither, identity is
// left to the transport (mTLS or a mesh). The tenant also travels as x-tenant-id
// metadata.
package tracer

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DefaultOperationTimeout is the per-operation context timeout applied when
// the caller does not configure one via WithGRPCOperationTimeout. It mirrors
// the tracer.timeoutMs default (250ms) so a misconfigured client still fails
// fast rather than holding the transaction create path open.
const DefaultOperationTimeout = 250 * time.Millisecond

// tenantMetadataKey is the gRPC outgoing-metadata key carrying the tenant id.
// Under token identity the tracer resolves the tenant from the token and reads
// this header only as a cross-check; under every other identity it is the
// carrier.
const tenantMetadataKey = "x-tenant-id"

// authorizationMetadataKey carries the ledger's Access Manager application
// token as "Bearer <token>" on every seam call when the ledger runs with plugin
// auth enabled.
const authorizationMetadataKey = "authorization"

// apiKeyMetadataKey carries the tracer API key on every seam call when the
// ledger identifies itself with TRACER_API_KEY instead of a token.
const apiKeyMetadataKey = "x-api-key"

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

// ErrTracerNoAnswer marks a call that was sent and then ran out of time or was
// cancelled before the tracer answered, so the tracer may still have committed
// it. It always travels alongside ErrTracerUnavailable, which keeps
// tracer.failPosture in charge of the request; this sentinel only tells the
// ledger that capacity may be held with no reservation id to address it. A call
// the tracer answered with an error, and one that never left the ledger, are
// not marked.
var ErrTracerNoAnswer = errors.New("tracer did not answer the reservation call")

// ErrTracerUnauthorized is the typed error returned when the tracer answers a
// seam call with codes.Unauthenticated or codes.PermissionDenied: the ledger's
// credential (its Access Manager token or the tracer API key) is missing,
// invalid or not granted. The tracer answered, so it is not ErrTracerUnavailable
// and never ErrTracerNoAnswer; the request itself was never evaluated, so it is
// not ErrTracerRejected either. It is a configuration error that does not heal
// on its own.
var ErrTracerUnauthorized = errors.New("tracer: credential rejected")

// ErrTracerCredentialUnavailable marks a call that never left the ledger
// because its seam credential could not be resolved: no usable tenant
// credential, a failed or empty token mint, or an invalid tenant id. A caller
// that stopped waiting for a mint is plain ErrTracerUnavailable. It always
// travels with ErrTracerUnavailable, so
// tracer.failPosture still decides the request, and is never marked
// ErrTracerNoAnswer, whatever context error caused it.
var ErrTracerCredentialUnavailable = errors.New("tracer: seam credential unavailable, call not sent")

// credentialNotSent wraps cause as an unavailable, never-sent seam call.
func credentialNotSent(cause error) error {
	return fmt.Errorf("%w: %w: %w", ErrTracerUnavailable, ErrTracerCredentialUnavailable, cause)
}

// ReserveAccount is the account scope the tracer matches limits and rules
// against. The ledger populates AccountID with the source balance's account
// UUID and Type with that account's free-form type; the tracer's account
// status is left empty, which it treats as an unconstrained optional field.
type ReserveAccount struct {
	// AccountID is empty when the ledger has no internal source account (an
	// external-only source). The tracer treats an empty account id as an absent
	// account, which the relaxed reserve validation accepts.
	AccountID string
	// Type is the ledger account type, verbatim. Optional.
	Type string
}

// ReserveRequest is the reserve input the ledger sends the tracer. It is typed
// independently of the tracer's internal model so the tracer's domain
// evolution does not leak onto the ledger's outbound contract; the gRPC client
// maps it field-for-field onto the proto reserve message. The reserve anchor
// populates it from the fee-inclusive transaction state; the client only
// transports it.
//
// The tracer's reserve validation requires request_id, a positive amount, a
// valid asset code (1 to 100 uppercase letters), and a transaction_timestamp
// that is not in the future. account.account_id is OPTIONAL on the relaxed
// reserve path: an external-only source omits it and the tracer accepts the
// accountless request. transaction_type is OPTIONAL too (the ledger has no
// card-rail nature to honestly report; when empty the tracer matches
// account-scoped limits without a transaction-type constraint).
type ReserveRequest struct {
	TransactionID uuid.UUID
	RequestID     string
	Amount        string
	Asset         string
	Account       ReserveAccount
	SegmentID     string
	PortfolioID   string
	MerchantID    string
	// TransactionType is optional on reserve. When set it must be a valid
	// tracer transaction type; the ledger leaves it empty.
	TransactionType string
	// TransactionTimestamp is RFC3339; the tracer rejects a future timestamp
	// against its injected clock and does not bound its age on reserve.
	TransactionTimestamp string
	// LongLived hints the tracer to assign a long-lived reservation lifetime to
	// a PENDING-transaction reservation.
	LongLived bool
	// Metadata is the transaction's flat metadata. The tracer accepts keys
	// matching ^[a-zA-Z0-9_]+$, at most 64 characters, and at most 50 entries.
	Metadata map[string]string
	// Revert marks the reservation as the revert of an applied transaction. The
	// tracer skips rule evaluation for it and still reserves limit capacity.
	Revert bool
}

// ReserveResult is the handle returned by a successful reserve. Denied is the
// refusal flag (no capacity held, ReservationIDs empty). Otherwise
// ReservationIDs holds one id per counter-backed limit the ledger must later
// confirm or release. Decision (ALLOW, DENY or REVIEW), Reason and
// MatchedRuleIDs refine Denied; a tracer that predates them leaves them zero,
// and Denied stays authoritative either way.
type ReserveResult struct {
	TransactionID  uuid.UUID
	Denied         bool
	Decision       string
	Reason         string
	MatchedRuleIDs []uuid.UUID
	ReservationIDs []uuid.UUID
}

// ConfirmOutcome is what a successful confirm found. AlreadyReleased counts the
// reservations the tracer had already released by an earlier release, so their
// spend is never counted against the limit. An expired reservation is not among
// them: confirm settles it and counts its spend.
//
// Confirmed means different things per call. A by-transaction confirm counts
// the reservations the call settled. A by-id confirm reports exactly one of the
// two fields, and there Confirmed means only "not already released": the wire
// cannot tell a reservation settled by this call from one an earlier confirm
// already settled, so an idempotent re-confirm also reports Confirmed 1.
type ConfirmOutcome struct {
	Confirmed       int
	AlreadyReleased int
}
