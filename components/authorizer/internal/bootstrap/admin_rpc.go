// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/engine"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// adminAuthTokenHeader is the gRPC metadata key carrying AUTHORIZER_ADMIN_TOKEN
// on admin RPCs. Intentionally distinct from the peer-auth headers so a peer
// signed payload cannot accidentally unlock admin surface, and vice versa.
// This is a metadata header name, not a credential — the actual secret lives
// in AUTHORIZER_ADMIN_TOKEN.
const adminAuthTokenHeader = "x-midaz-admin-token" //nolint:gosec // metadata header name, not a credential

// Resolution outcomes surfaced in the ResolveManualInterventionResponse.new_status
// field. Kept as constants so the audit-trail emits stable tokens that
// dashboards and SIEM rules can pin on.
const (
	adminResolutionNewStatusCommitted    = "committed"
	adminResolutionNewStatusAborted      = "aborted"
	adminResolutionNewStatusInvestigated = "investigated"
)

// Action labels for the audit event. These correspond one-to-one with the
// ManualInterventionResolution enum values; the audit log uses the stable
// string form so analysts don't have to cross-reference proto enum numbers.
const (
	adminActionForceCommit       = "force_commit"
	adminActionForceAbort        = "force_abort"
	adminActionMarkInvestigated  = "mark_investigated"
	adminActionUnspecified       = "unspecified"
	adminActionUnknown           = "unknown"
	adminActorLabel              = "admin"
	adminSecurityCategory        = "admin_rpc"
	adminSecuritySeverityWarn    = "warn"
	adminSecuritySeverityError   = "error"
	adminLogEventInvocation      = "admin_rpc_invocation"
	adminLogEventAuthFailure     = "admin_rpc_auth_failed"
	adminLogEventResolution      = "admin_rpc_resolution"
	adminLogEventNotFound        = "admin_rpc_not_found"
	adminLogEventInvalidArgument = "admin_rpc_invalid_argument"
)

// ResolveManualIntervention is the admin RPC entry point for operator-driven
// resolution of stuck prepared transactions. See the proto definition for the
// request/response contract. The method:
//
//  1. Validates the admin credential (x-midaz-admin-token metadata header)
//     using constant-time comparison. Peer-auth HMAC alone is NOT accepted —
//     admin operations require a dedicated credential.
//  2. Emits a security-category log line for every invocation regardless of
//     outcome (authorized or denied) so SIEM pipelines can detect misuse.
//  3. Inspects the engine's prepared-tx store for the given tx_id. Returns
//     codes.NotFound when the ID is absent from both the pending and the
//     committed maps.
//  4. Dispatches on the resolution enum:
//     - FORCE_COMMIT: call engine.CommitPrepared; on success the audit event
//     records actor=admin and action=force_commit.
//     - FORCE_ABORT: call engine.AbortPrepared, releasing the balance locks
//     without mutation.
//     - MARK_INVESTIGATED: keep the state untouched, emit the audit event only
//     (acknowledgement that the operator has reviewed the transaction).
//  5. Emits EmitAuthorizationAuditEvent regardless of branch so the financial
//     audit trail always records the operator action, the notes, and the
//     observed state transition.
//
// The returned response carries the prior_status, new_status, and server-side
// timestamp so callers can reconcile their view of the transaction.
func (s *authorizerService) ResolveManualIntervention(
	ctx context.Context,
	req *authorizerv1.ResolveManualInterventionRequest,
) (*authorizerv1.ResolveManualInterventionResponse, error) {
	if s == nil {
		return nil, status.Error(codes.Internal, "authorizer service not initialized") //nolint:wrapcheck // gRPC status error
	}

	// Always emit an invocation log line — even before credential validation —
	// so unauthorized probing is observable in the SIEM stream. Token material
	// is never logged; the actor is fixed to "admin" because the token itself
	// does not carry a user identity.
	s.logAdminInvocation(ctx, req)

	if err := s.authorizeAdminRPC(ctx); err != nil {
		return nil, err
	}

	if req == nil {
		s.auditAdminInvalidArgument(ctx, "", "", "request is nil")

		return nil, status.Error(codes.InvalidArgument, "request is required") //nolint:wrapcheck // gRPC status error
	}

	txID := strings.TrimSpace(req.GetTxId())
	if txID == "" {
		s.auditAdminInvalidArgument(ctx, "", adminResolutionActionLabel(req.GetResolution()), "tx_id is empty")

		return nil, status.Error(codes.InvalidArgument, "tx_id is required") //nolint:wrapcheck // gRPC status error
	}

	resolution := req.GetResolution()
	actionLabel := adminResolutionActionLabel(resolution)
	notes := strings.TrimSpace(req.GetNotes())

	if resolution == authorizerv1.ManualInterventionResolution_RESOLUTION_UNSPECIFIED {
		s.auditAdminInvalidArgument(ctx, txID, actionLabel, "resolution is RESOLUTION_UNSPECIFIED")

		return nil, status.Error(codes.InvalidArgument, "resolution must not be RESOLUTION_UNSPECIFIED") //nolint:wrapcheck // gRPC status error
	}

	priorStatus := s.engine.InspectPreparedState(txID)
	if priorStatus == engine.PreparedStateNotFound {
		s.auditAdminNotFound(ctx, txID, actionLabel, notes)

		return nil, status.Error(codes.NotFound, "prepared transaction not found") //nolint:wrapcheck // gRPC status error
	}

	newStatus, err := s.applyAdminResolution(ctx, txID, resolution)
	if err != nil {
		// Already-mapped gRPC error from applyAdminResolution.
		return nil, err
	}

	timestamp := time.Now().UTC().Format(time.RFC3339Nano)

	// Single canonical audit line per successful resolution. The audit_and_slo
	// tests rely on the AUTHORIZER_AUDIT prefix and cardinality-safe labels.
	s.auditAdminResolution(ctx, txID, actionLabel, priorStatus, newStatus, notes, timestamp)

	return &authorizerv1.ResolveManualInterventionResponse{
		PriorStatus: priorStatus,
		NewStatus:   newStatus,
		Timestamp:   timestamp,
	}, nil
}

// authorizeAdminRPC validates the admin credential in the incoming context.
// It is intentionally narrower than authorizePeerRPC — no HMAC, no nonce, no
// body-hash binding, because admin operations are rare operator-initiated
// actions (not high-volume peer traffic). The dedicated token is compared
// using subtle.ConstantTimeCompare so timing-side-channel probing cannot
// leak it.
func (s *authorizerService) authorizeAdminRPC(ctx context.Context) error {
	if s.adminToken == "" {
		s.recordUnauthorizedRPC(ctx, adminRPCMethodResolveManualIntervention, unauthorizedReasonMissingAdminToken)
		s.emitAdminAuthFailure(ctx, "admin token is not configured")

		return status.Error(codes.Unauthenticated, "admin token is not configured") //nolint:wrapcheck // gRPC status error
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		s.recordUnauthorizedRPC(ctx, adminRPCMethodResolveManualIntervention, unauthorizedReasonMissingAdminToken)
		s.emitAdminAuthFailure(ctx, "reason=missing_admin_token")

		return status.Error(codes.Unauthenticated, "missing_admin_token") //nolint:wrapcheck // gRPC status error
	}

	values := md.Get(adminAuthTokenHeader)
	if len(values) == 0 {
		s.recordUnauthorizedRPC(ctx, adminRPCMethodResolveManualIntervention, unauthorizedReasonMissingAdminToken)
		s.emitAdminAuthFailure(ctx, "reason=missing_admin_token")

		return status.Error(codes.Unauthenticated, "missing_admin_token") //nolint:wrapcheck // gRPC status error
	}

	provided := strings.TrimSpace(values[0])
	if provided == "" {
		s.recordUnauthorizedRPC(ctx, adminRPCMethodResolveManualIntervention, unauthorizedReasonMissingAdminToken)
		s.emitAdminAuthFailure(ctx, "reason=missing_admin_token")

		return status.Error(codes.Unauthenticated, "missing_admin_token") //nolint:wrapcheck // gRPC status error
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(s.adminToken)) != 1 {
		s.recordUnauthorizedRPC(ctx, adminRPCMethodResolveManualIntervention, unauthorizedReasonInvalidAdminToken)
		s.emitAdminAuthFailure(ctx, "reason=invalid_admin_token")

		return status.Error(codes.Unauthenticated, "invalid_admin_token") //nolint:wrapcheck // gRPC status error
	}

	return nil
}

// applyAdminResolution dispatches on the resolution enum and returns the
// externally-observable new_status string, or a gRPC-typed error.
func (s *authorizerService) applyAdminResolution(
	ctx context.Context,
	txID string,
	resolution authorizerv1.ManualInterventionResolution,
) (string, error) {
	switch resolution {
	case authorizerv1.ManualInterventionResolution_RESOLUTION_FORCE_COMMIT:
		if _, err := s.engine.CommitPrepared(txID); err != nil {
			// Classify engine errors into the appropriate gRPC code. We treat
			// NotFound explicitly so race conditions (tx expired between
			// InspectPreparedState and CommitPrepared) surface as NotFound
			// rather than Internal.
			if errors.Is(err, engine.ErrPreparedTxNotFound) {
				s.auditAdminNotFound(ctx, txID, adminActionForceCommit, "force_commit_race")

				return "", status.Error(codes.NotFound, "prepared transaction not found") //nolint:wrapcheck // gRPC status error
			}

			s.emitAdminAuthFailure(ctx, fmt.Sprintf("force_commit failed tx_id=%s err=%v", txID, err))

			return "", status.Error(codes.Internal, "force commit failed") //nolint:wrapcheck // gRPC status error
		}

		return adminResolutionNewStatusCommitted, nil

	case authorizerv1.ManualInterventionResolution_RESOLUTION_FORCE_ABORT:
		if err := s.engine.AbortPrepared(txID); err != nil {
			if errors.Is(err, engine.ErrPreparedTxNotFound) {
				s.auditAdminNotFound(ctx, txID, adminActionForceAbort, "force_abort_race")

				return "", status.Error(codes.NotFound, "prepared transaction not found") //nolint:wrapcheck // gRPC status error
			}

			if errors.Is(err, engine.ErrPreparedTxAlreadyCommitted) || errors.Is(err, engine.ErrPreparedTxCommitDecided) {
				return "", status.Error(codes.FailedPrecondition, "prepared transaction commit already decided") //nolint:wrapcheck // gRPC status error
			}

			s.emitAdminAuthFailure(ctx, fmt.Sprintf("force_abort failed tx_id=%s err=%v", txID, err))

			return "", status.Error(codes.Internal, "force abort failed") //nolint:wrapcheck // gRPC status error
		}

		return adminResolutionNewStatusAborted, nil

	case authorizerv1.ManualInterventionResolution_RESOLUTION_MARK_INVESTIGATED:
		// No state mutation — the operator is acknowledging that the
		// transaction has been reviewed. The audit event emitted by the caller
		// is the only side effect.
		return adminResolutionNewStatusInvestigated, nil

	case authorizerv1.ManualInterventionResolution_RESOLUTION_UNSPECIFIED:
		// Should have been caught earlier — keep as defense in depth.
		return "", status.Error(codes.InvalidArgument, "resolution must not be RESOLUTION_UNSPECIFIED") //nolint:wrapcheck // gRPC status error

	default:
		return "", status.Errorf(codes.InvalidArgument, "unknown resolution enum value %d", resolution) //nolint:wrapcheck // gRPC status error
	}
}

// adminResolutionActionLabel converts the proto enum into the stable
// audit-trail label.
func adminResolutionActionLabel(resolution authorizerv1.ManualInterventionResolution) string {
	switch resolution {
	case authorizerv1.ManualInterventionResolution_RESOLUTION_FORCE_COMMIT:
		return adminActionForceCommit
	case authorizerv1.ManualInterventionResolution_RESOLUTION_FORCE_ABORT:
		return adminActionForceAbort
	case authorizerv1.ManualInterventionResolution_RESOLUTION_MARK_INVESTIGATED:
		return adminActionMarkInvestigated
	case authorizerv1.ManualInterventionResolution_RESOLUTION_UNSPECIFIED:
		return adminActionUnspecified
	default:
		return adminActionUnknown
	}
}

// logAdminInvocation emits a security-category log line for every admin RPC
// invocation, regardless of outcome. Provides SIEM pipelines with a single
// pattern to tail on (AUTHORIZER_SECURITY category=admin_rpc) so unauthorized
// probing and successful resolutions are both visible.
func (s *authorizerService) logAdminInvocation(ctx context.Context, req *authorizerv1.ResolveManualInterventionRequest) {
	if s == nil || s.metrics == nil {
		return
	}

	_ = ctx // accepted for future correlation (e.g. propagated trace ID)

	txID := ""
	action := adminActionUnspecified

	if req != nil {
		txID = strings.TrimSpace(req.GetTxId())
		action = adminResolutionActionLabel(req.GetResolution())
	}

	detail := fmt.Sprintf(
		"event=%s tx_id=%s action=%s",
		adminLogEventInvocation,
		safeLogToken(txID),
		action,
	)
	s.metrics.RecordSecurityEvent(adminSecuritySeverityWarn, adminSecurityCategory, detail)
}

// emitAdminAuthFailure emits an ERROR-severity security event for admin-token
// rejections.
func (s *authorizerService) emitAdminAuthFailure(ctx context.Context, detail string) {
	if s == nil || s.metrics == nil {
		return
	}

	_ = ctx

	s.metrics.RecordSecurityEvent(
		adminSecuritySeverityError,
		adminSecurityCategory,
		fmt.Sprintf("event=%s %s", adminLogEventAuthFailure, detail),
	)
}

// auditAdminResolution emits the structured audit event for a successful
// admin resolution. Hooks into the same EmitAuthorizationAuditEvent primitive
// used by the authorize path so the audit stream stays single-source.
func (s *authorizerService) auditAdminResolution(
	ctx context.Context,
	txID, action, priorStatus, newStatus, notes, timestamp string,
) {
	if s == nil || s.metrics == nil {
		return
	}

	// Financial audit trail: one canonical line per resolution, same tag
	// ("AUTHORIZER_AUDIT") the transaction-authorize path uses so downstream
	// log pipelines need only one tail.
	s.metrics.EmitAuthorizationAuditEvent(
		ctx,
		"",   // organization_id: not known to admin RPC
		"",   // ledger_id: not known to admin RPC
		txID, // transaction_id (prepared_tx_id in this surface)
		adminActorLabel,
		action,
		newStatus,
		"",    // amount_bucket: N/A for admin resolutions
		false, // cross_shard: N/A for admin resolutions
	)

	// Supplementary security-category line carrying the operator's notes and
	// the state transition. Notes go through safeLogToken so control chars
	// cannot break log parsers.
	s.metrics.RecordSecurityEvent(
		adminSecuritySeverityWarn,
		adminSecurityCategory,
		fmt.Sprintf(
			"event=%s tx_id=%s action=%s prior_status=%s new_status=%s notes=%s timestamp=%s",
			adminLogEventResolution,
			safeLogToken(txID),
			action,
			safeLogToken(priorStatus),
			safeLogToken(newStatus),
			safeLogToken(notes),
			safeLogToken(timestamp),
		),
	)
}

// auditAdminNotFound emits the audit event for a NotFound outcome. Recorded
// with the same primitive as successful resolutions so analysts can see all
// admin actions regardless of outcome.
func (s *authorizerService) auditAdminNotFound(ctx context.Context, txID, action, notes string) {
	if s == nil || s.metrics == nil {
		return
	}

	s.metrics.EmitAuthorizationAuditEvent(
		ctx,
		"",
		"",
		txID,
		adminActorLabel,
		action,
		"not_found",
		"",
		false,
	)

	s.metrics.RecordSecurityEvent(
		adminSecuritySeverityWarn,
		adminSecurityCategory,
		fmt.Sprintf(
			"event=%s tx_id=%s action=%s notes=%s",
			adminLogEventNotFound,
			safeLogToken(txID),
			action,
			safeLogToken(notes),
		),
	)
}

// auditAdminInvalidArgument emits the audit event for rejected requests that
// never reached the engine (malformed tx_id, unspecified resolution, etc.).
func (s *authorizerService) auditAdminInvalidArgument(ctx context.Context, txID, action, reason string) {
	if s == nil || s.metrics == nil {
		return
	}

	s.metrics.EmitAuthorizationAuditEvent(
		ctx,
		"",
		"",
		txID,
		adminActorLabel,
		action,
		"invalid_argument",
		"",
		false,
	)

	s.metrics.RecordSecurityEvent(
		adminSecuritySeverityWarn,
		adminSecurityCategory,
		fmt.Sprintf(
			"event=%s tx_id=%s action=%s reason=%s",
			adminLogEventInvalidArgument,
			safeLogToken(txID),
			action,
			safeLogToken(reason),
		),
	)
}

// safeLogToken wraps normalizeLogToken so admin_rpc.go can emit operator-
// supplied tokens (tx_id, notes) without accidentally propagating control
// characters or invisible codepoints into downstream log pipelines.
func safeLogToken(value string) string {
	return normalizeLogToken(value)
}
