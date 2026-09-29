// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// reservationOutcomeKind enumerates the three branches the reserve anchor can
// take before the balance commit.
type reservationOutcomeKind int

const (
	// reservationProceed: the create path continues to accounting-engine execution.
	// Handle holds the reservation ids to confirm/release post-commit (it is
	// empty when the tracer was skipped — off/advisory/nil/fail-open).
	reservationProceed reservationOutcomeKind = iota

	// reservationReject: the transaction MUST be rejected before any balance
	// move (a DENIED limit decision, or a fail-closed unavailable tracer). Err
	// carries the business error for the HTTP response; the caller releases the
	// idempotency key and removes the Redis-queue entry, mirroring the
	// post-fee re-validation rejection mechanics.
	reservationReject
)

// reservationOutcome is the decision the reserve anchor returns to the create
// seam. It is deliberately a value type with no balance data — the reserve seam
// observes amounts and gates execution; it never alters Send.Value or balance
// math (third rail).
type reservationOutcome struct {
	Kind   reservationOutcomeKind
	Handle reservationHandle
	Err    error
}

// reservationHandle carries the reservation ids produced by a successful
// reserve so the post-commit transport (confirm on success, release on abort)
// can address them. An empty handle means there is nothing to confirm or
// release (tracer skipped or no capacity-backed limit applied).
//
// It also carries the transaction the capacity was held for and the amount that
// was held. Neither is needed to address the transition — the reservation id
// alone does that — but both are needed to REPORT one that could not be
// delivered: an operator reading a lost confirm has to know which transaction
// and how much spending went uncounted, and the reservation id alone says
// neither.
type reservationHandle struct {
	ReservationIDs []uuid.UUID
	TransactionID  uuid.UUID
	Amount         decimal.Decimal
	Asset          string
}

// reservationTTLPolicy selects the reservation lifetime hint passed to the
// tracer. Direct transactions get the tracer's default (short, reaper-swept)
// TTL; PENDING transactions get a long-lived hint so a reservation does not
// expire under a still-valid pending that has no existing sweep.
type reservationTTLPolicy bool

const (
	reservationTTLDefault   reservationTTLPolicy = false
	reservationTTLLongLived reservationTTLPolicy = true
)

// reservationPurpose tells the tracer whether the reservation is held for the
// revert of an applied transaction. A revert still reserves capacity, because
// limits measure gross activity, but the tracer does not evaluate transaction
// validation rules for it: a rule that could refuse a revert would leave an
// applied movement impossible to correct.
type reservationPurpose bool

const (
	reservationForCreate reservationPurpose = false
	reservationForRevert reservationPurpose = true
)

// reservationDecisionReview is the tracer decision for a transaction a rule
// flagged for review, or one a rule could not be evaluated for. It arrives with denied=true; every other denied result —
// DENY, or a tracer that predates the decision field — is a limit denial.
const reservationDecisionReview = "REVIEW"

// Tracer metadata bounds. The tracer refuses the whole reserve request when any
// key breaks them, so the anchor forwards only what it would accept.
const (
	reserveMetadataMaxEntries   = 50
	reserveMetadataMaxKeyLength = 64
)

// reserveMetadataKeyPattern is the key grammar the tracer accepts.
var reserveMetadataKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// reservationRequestIDNamespace is the UUIDv5 namespace used to derive a
// reserve requestId from a transactionID. A fixed namespace makes the requestId
// deterministic per transaction, so a retried reserve carries the same requestId
// and dedups against the prior attempt rather than minting a fresh request.
var reservationRequestIDNamespace = uuid.MustParse("6f3c2d1e-4b5a-4c6d-8e7f-0a1b2c3d4e5f")

// transitions expands the handle into one addressable transition per held
// reservation, each carrying the full identity of what is being settled.
func (h reservationHandle) transitions(action string) []reservationTransition {
	out := make([]reservationTransition, 0, len(h.ReservationIDs))

	for _, id := range h.ReservationIDs {
		out = append(out, reservationTransition{
			Action:        action,
			TransactionID: h.TransactionID,
			ReservationID: id,
			Amount:        h.Amount,
			Asset:         h.Asset,
		})
	}

	return out
}

// reserveTransaction is the reserve anchor. It is called immediately before
// accounting-engine execution on FEE-INCLUSIVE amounts and gates execution on
// the per-ledger tracer settings. Only the /v2 pipelines call it: the /v1 contract
// shipped before the tracer existed, so a /v1 create is never gated by a
// reservation, builds no request and dials nothing.
//
// The request carries the source account id and type, the tracer-accepted
// subset of the transaction metadata (see reserveMetadata), and whether the
// reservation is for a revert. A positive tracer.timeoutMs bounds the reserve
// call; the client timeout (TRACER_TIMEOUT_MS) stays the ceiling, because a
// context deadline can only tighten it.
//
//   - mode=off (or nil reserver): skipped — returns proceed with an empty handle.
//   - mode=advisory: the reserve is called but never blocks — a DENY or REVIEW
//     decision, a refused request or an unavailable tracer still returns proceed
//     (advisory observes, the real gate is enforce).
//   - mode=enforce: a DENY decision rejects with 0177 and a REVIEW decision (a
//     matched rule, or a rule the tracer could not evaluate) with 0531, both
//     before the balance commit. A request the tracer refused
//     (tracer.ErrTracerRejected) rejects with 0532 whatever the failPosture,
//     because the tracer answered. An unavailable tracer branches on failPosture
//     (open → proceed + SKIPPED audit, closed → reject with 0178).
//
// It NEVER mutates Send.Value or any balance state; amount/asset are read-only
// inputs observed for the reservation request.
func (uc *UseCase) reserveTransaction(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	settings mmodel.TracerSettings,
	transactionID uuid.UUID,
	amount decimal.Decimal,
	asset string,
	account tracer.ReserveAccount,
	metadata map[string]any,
	transactionTimestamp time.Time,
	ttl reservationTTLPolicy,
	purpose reservationPurpose,
	honoredTracerSkip bool,
) reservationOutcome {
	// off, unconfigured, no client injected, or an honored per-call tracer skip:
	// the create path is unchanged and no reserve request is built or sent. An
	// honored skip wins over advisory/enforce — the operator explicitly allowed
	// the caller to opt out.
	if uc.TracerReserver == nil || settings.Mode == mmodel.TracerModeOff || settings.Mode == "" || honoredTracerSkip {
		return reservationOutcome{Kind: reservationProceed}
	}

	advisory := settings.Mode == mmodel.TracerModeAdvisory

	reserveMD, droppedMD := reserveMetadata(metadata)
	if droppedMD > 0 {
		span.SetAttributes(attribute.Int("app.tracer.metadata_dropped", droppedMD))
	}

	req := tracer.ReserveRequest{
		TransactionID:        transactionID,
		RequestID:            reservationRequestID(transactionID).String(),
		Amount:               amount.String(),
		Asset:                asset,
		Account:              account,
		TransactionTimestamp: transactionTimestamp.UTC().Format(time.RFC3339Nano),
		LongLived:            ttl == reservationTTLLongLived,
		Revert:               purpose == reservationForRevert,
		Metadata:             reserveMD,
	}

	reserveCtx := ctx

	if settings.TimeoutMs > 0 {
		span.SetAttributes(attribute.Int("app.tracer.timeout_ms", settings.TimeoutMs))

		var cancel context.CancelFunc

		reserveCtx, cancel = context.WithTimeout(ctx, time.Duration(settings.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	result, err := uc.TracerReserver.Reserve(reserveCtx, req)
	if err != nil {
		return uc.handleReserveError(ctx, span, logger, settings, transactionID, advisory, err)
	}

	span.SetAttributes(
		attribute.String("app.tracer.decision", result.Decision),
		attribute.Int("app.tracer.matched_rule_count", len(result.MatchedRuleIDs)),
	)

	if result.Denied {
		return uc.handleReserveDenied(ctx, span, logger, transactionID, advisory, result)
	}

	return reservationOutcome{
		Kind: reservationProceed,
		Handle: reservationHandle{
			ReservationIDs: result.ReservationIDs,
			TransactionID:  transactionID,
			Amount:         amount,
			Asset:          asset,
		},
	}
}

// handleReserveDenied maps a denied reserve result to an outcome. Advisory
// observes it and proceeds; enforce rejects with the code that tells a review
// flag from a limit denial. No capacity is held on a denied result, so the
// handle is empty either way.
func (uc *UseCase) handleReserveDenied(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	transactionID uuid.UUID,
	advisory bool,
	result *tracer.ReserveResult,
) reservationOutcome {
	review := result.Decision == reservationDecisionReview

	if advisory {
		logger.Log(ctx, libLog.LevelWarn, "Tracer reservation denied in advisory mode; proceeding without gating",
			libLog.String("transaction_id", transactionID.String()),
			libLog.String("decision", result.Decision),
			libLog.String("reason", result.Reason))

		return reservationOutcome{Kind: reservationProceed}
	}

	sentinel := constant.ErrTransactionReservationDenied
	if review {
		sentinel = constant.ErrTransactionReservationReview
	}

	rejectErr := pkg.ValidateBusinessError(sentinel, constant.EntityTransaction)
	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Tracer reservation denied", rejectErr)
	logger.Log(ctx, libLog.LevelWarn, "Tracer reservation denied; rejecting before balance commit",
		libLog.String("transaction_id", transactionID.String()),
		libLog.String("decision", result.Decision),
		libLog.String("reason", result.Reason))

	return reservationOutcome{Kind: reservationReject, Err: rejectErr}
}

// handleReserveError maps a reserve call failure to an outcome. A refused
// request (tracer.ErrTracerRejected) is a business outcome, not an outage: the
// tracer answered, so it never reaches failPosture — advisory proceeds, enforce
// rejects with 0532. Any other failure is gated by failPosture; advisory never
// blocks regardless. A non-availability error that is not a refusal is treated
// like an availability failure so a tracer defect cannot silently let an
// enforce ledger commit unchecked under fail-closed, while fail-open still
// proceeds.
func (uc *UseCase) handleReserveError(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	settings mmodel.TracerSettings,
	transactionID uuid.UUID,
	advisory bool,
	err error,
) reservationOutcome {
	if errors.Is(err, tracer.ErrTracerRejected) {
		return uc.handleReserveRejected(ctx, span, logger, transactionID, advisory, err)
	}

	libOpentelemetry.HandleSpanError(span, "Tracer reservation call failed", err)

	if advisory {
		logger.Log(ctx, libLog.LevelWarn, "Tracer reservation failed in advisory mode; proceeding",
			libLog.String("transaction_id", transactionID.String()),
			libLog.Err(err))

		return reservationOutcome{Kind: reservationProceed}
	}

	if settings.FailPosture == mmodel.TracerFailPostureClosed {
		rejectErr := pkg.ValidateBusinessError(constant.ErrTransactionReservationUnavailable, constant.EntityTransaction)

		logger.Log(ctx, libLog.LevelWarn, "Tracer unavailable and failPosture=closed; rejecting transaction",
			libLog.String("transaction_id", transactionID.String()),
			libLog.Err(err))

		return reservationOutcome{Kind: reservationReject, Err: rejectErr}
	}

	// failPosture=open (the default): record a SKIPPED audit and proceed so a
	// degraded tracer cannot block all transactions. The SKIPPED audit is
	// the tracer's own record — best-effort via Release on no ids is a no-op, so
	// the audit is emitted by the tracer reserve attempt itself; here we mark
	// the span and continue.
	span.SetAttributes(attribute.Bool("app.tracer.reservation_skipped", true))
	logger.Log(ctx, libLog.LevelWarn, "Tracer unavailable and failPosture=open; skipping reservation and proceeding",
		libLog.String("transaction_id", transactionID.String()),
		libLog.Err(err))

	return reservationOutcome{Kind: reservationProceed}
}

// handleReserveRejected maps a refused reserve request to an outcome. The
// refusal is a business event on a span that stays green; its error is logged,
// never returned to the client, which sees only 0532.
func (uc *UseCase) handleReserveRejected(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	transactionID uuid.UUID,
	advisory bool,
	err error,
) reservationOutcome {
	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Tracer rejected the reservation request", err)

	if advisory {
		logger.Log(ctx, libLog.LevelWarn, "Tracer rejected the reservation request in advisory mode; proceeding",
			libLog.String("transaction_id", transactionID.String()),
			libLog.Err(err))

		return reservationOutcome{Kind: reservationProceed}
	}

	rejectErr := pkg.ValidateBusinessError(constant.ErrTransactionReservationRejected, constant.EntityTransaction)

	logger.Log(ctx, libLog.LevelWarn, "Tracer rejected the reservation request; rejecting transaction",
		libLog.String("transaction_id", transactionID.String()),
		libLog.Err(err))

	return reservationOutcome{Kind: reservationReject, Err: rejectErr}
}

// reservationRequestID derives the deterministic reserve requestId for a
// transaction. The tracer reserve contract requires a non-nil requestId; the
// ledger has no separate request handle at the anchor, so it derives one from
// the transactionID. Determinism is the contract: identical transactionID →
// identical requestId, so retries do not present as distinct requests.
func reservationRequestID(transactionID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(reservationRequestIDNamespace, transactionID[:])
}

// firstSourceAccount resolves the account of the first internal source leg so
// the reserve request carries the account scope the tracer matches
// account-scoped limits and rules against. Spend/usage limits apply to the
// debited (source) account, so the first source's account is the honest scope
// handle. sources carries "<alias>#<balanceKey>" entries; the bare alias is
// matched against the loaded balances, and the account id and type come from
// the same balance row. Companion (overdraft) and any source with no matching
// internal balance are skipped. Returns the zero account when no internal
// source account resolves (e.g. an external-only source) — the reserve path
// treats the account scope as optional, so the tracer still matches
// non-account-scoped limits.
func firstSourceAccount(sources []string, balances []*mmodel.Balance) tracer.ReserveAccount {
	if len(sources) == 0 || len(balances) == 0 {
		return tracer.ReserveAccount{}
	}

	byAlias := make(map[string]*mmodel.Balance, len(balances))
	for _, b := range balances {
		if b == nil || b.Key == constant.OverdraftBalanceKey {
			continue
		}

		if _, seen := byAlias[b.Alias]; !seen {
			byAlias[b.Alias] = b
		}
	}

	for _, src := range filterCompanionAliases(sources) {
		alias := src
		if idx := strings.IndexByte(alias, '#'); idx >= 0 {
			alias = alias[:idx]
		}

		if b, ok := byAlias[alias]; ok && b.AccountID != "" {
			return tracer.ReserveAccount{AccountID: b.AccountID, Type: b.AccountType}
		}
	}

	return tracer.ReserveAccount{}
}

// reserveMetadata projects the transaction metadata onto the tracer's metadata
// contract. Keys are visited in lexicographic order so the cap is
// deterministic; a key must match the tracer grammar and length, and its value
// must be a scalar, rendered as a string (numbers in plain decimal notation).
// Anything else, and every entry past the cap, is dropped and counted. It
// returns nil when nothing survives, so the request carries no metadata.
func reserveMetadata(md map[string]any) (map[string]string, int) {
	if len(md) == 0 {
		return nil, 0
	}

	keys := make([]string, 0, len(md))
	for key := range md {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	out := make(map[string]string, min(len(keys), reserveMetadataMaxEntries))
	dropped := 0

	for _, key := range keys {
		if len(out) == reserveMetadataMaxEntries ||
			len(key) > reserveMetadataMaxKeyLength ||
			!reserveMetadataKeyPattern.MatchString(key) {
			dropped++

			continue
		}

		value, ok := reserveMetadataValue(md[key])
		if !ok {
			dropped++

			continue
		}

		out[key] = value
	}

	if len(out) == 0 {
		return nil, dropped
	}

	return out, dropped
}

// reserveMetadataValue renders a scalar metadata value as a string. It reports
// false for nil, nested and unsupported values.
func reserveMetadataValue(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", false
		}

		return decimal.NewFromFloat(v).String(), true
	case json.Number:
		d, err := decimal.NewFromString(v.String())
		if err != nil {
			return "", false
		}

		return d.String(), true
	case decimal.Decimal:
		return v.String(), true
	default:
		return "", false
	}
}

// confirmReservations commits held reservations after a successful balance
// commit (the success phase). Transport never blocks the request: a
// failure is logged at Warn, span-recorded, and never propagated, because the
// money has already moved and the response is owed now. The failure is NOT
// dropped, though — it is handed to the retrier, which keeps trying off the
// request path until the tracer accepts it or the budget runs out. A nil
// reserver or empty handle is a no-op.
func (uc *UseCase) confirmReservations(ctx context.Context, span trace.Span, logger libLog.Logger, handle reservationHandle) {
	if uc.TracerReserver == nil {
		return
	}

	for _, transition := range handle.transitions(reservationActionConfirm) {
		if err := uc.TracerReserver.Confirm(ctx, transition.ReservationID); err != nil {
			uc.recordReservationTransportFailure(ctx, span, logger, transition, err)
		}
	}
}

// releaseReservations returns held reservations on an aborted transaction
// (the abort phase). Same non-blocking, retried posture as
// confirmReservations: a failure never fails the request and is redelivered off
// the request path. The direction of the loss is the opposite one — a release
// that never lands leaves capacity held against a transaction that moved no
// money — which is why the report distinguishes them.
func (uc *UseCase) releaseReservations(ctx context.Context, span trace.Span, logger libLog.Logger, handle reservationHandle) {
	if uc.TracerReserver == nil {
		return
	}

	for _, transition := range handle.transitions(reservationActionRelease) {
		if err := uc.TracerReserver.Release(ctx, transition.ReservationID); err != nil {
			uc.recordReservationTransportFailure(ctx, span, logger, transition, err)
		}
	}
}

// recordReservationTransportFailure logs and span-records a confirm/release
// transport failure without propagating it. Both an availability failure
// (tracer.ErrTracerUnavailable) and any other transport error are the
// lost-transport case, so both are Warn-logged and swallowed here rather than
// failing a transaction whose money has already moved.
//
// The record names the transaction and the amount, not only the reservation id.
// A lost confirm means a committed spend the limit never counted; an operator
// reading this line has to be able to say WHICH transaction and HOW MUCH
// without joining against the tracer's own store, which may be the thing that
// is down.
func (uc *UseCase) recordReservationTransportFailure(ctx context.Context, span trace.Span, logger libLog.Logger, transition reservationTransition, err error) {
	libOpentelemetry.HandleSpanError(span, "Tracer reservation "+transition.Action+" transport failed", err)

	logger.Log(ctx, libLog.LevelWarn, "Tracer reservation transport failed on the first attempt; retrying off the request path",
		append(transition.logFields(), libLog.Err(err)))

	uc.scheduleReservationRetry(ctx, logger, transition, err)
}

// confirmReservationsByTransaction commits a transaction's held reservations at
// /commit (the PENDING success phase). At /commit the ledger holds only the
// transaction id — the reserve handle from create-pending does not survive the
// separate commit request — so the tracer flips every RESERVED reservation the
// transaction holds, addressed by transaction id. Only transitionPendingV2 names it:
// the /v1 contract carries no tracer. Beyond that it is gated on the per-ledger tracer
// settings (off / nil reserver → no call) and on an honored per-call tracer skip (so a
// skip honored at create removes the gRPC cost here rather than relocating it to
// commit); same non-blocking posture as the by-id transport: a failure is logged at
// Warn, span-recorded and never propagated, and then retried off the request path.
//
// Living only on the /v2 pipeline carries an accepted cost. A by-transaction call cannot
// tell whether the transaction holds reservations, so a PENDING created on /v2 and
// committed through /v1 never gets its confirm: the reservation stays RESERVED until the
// reaper releases it, and the committed amount is never counted against the usage limit.
// Mixing mounts across one transaction lifecycle is therefore unsupported — see
// docs/api/SCOPING.md. Closing it needs create-time reservation state persisted on the
// transaction row for the /v1 pipeline to read.
func (uc *UseCase) confirmReservationsByTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, settings mmodel.TracerSettings, identity reservationHandle, honoredTracerSkip bool) {
	if honoredTracerSkip || !uc.tracerReservationEnabled(settings) {
		return
	}

	if err := uc.TracerReserver.ConfirmByTransaction(ctx, identity.TransactionID); err != nil {
		uc.recordReservationByTransactionFailure(ctx, span, logger, identity.transitionByTransaction(reservationActionConfirm), err)
	}
}

// releaseReservationsByTransaction returns a transaction's held reservations at
// /cancel (the PENDING abort phase). Same transaction-id addressing, gating,
// and non-blocking posture as confirmReservationsByTransaction.
func (uc *UseCase) releaseReservationsByTransaction(ctx context.Context, span trace.Span, logger libLog.Logger, settings mmodel.TracerSettings, identity reservationHandle, honoredTracerSkip bool) {
	if honoredTracerSkip || !uc.tracerReservationEnabled(settings) {
		return
	}

	if err := uc.TracerReserver.ReleaseByTransaction(ctx, identity.TransactionID); err != nil {
		uc.recordReservationByTransactionFailure(ctx, span, logger, identity.transitionByTransaction(reservationActionRelease), err)
	}
}

// tracerReservationEnabled reports whether the by-transaction confirm/release
// transport should fire: a reserver must be injected and the per-ledger mode must
// not be off/unset, mirroring the gate the reserve anchor applies at create time.
// Advisory and enforce both confirm/release — advisory observes the lifecycle, it
// only declines to BLOCK the request, and a confirm/release here never blocks.
func (uc *UseCase) tracerReservationEnabled(settings mmodel.TracerSettings) bool {
	return uc.TracerReserver != nil && settings.Mode != mmodel.TracerModeOff && settings.Mode != ""
}

// recordReservationByTransactionFailure logs and span-records a by-transaction
// confirm/release transport failure without propagating it. Like the by-id
// record it names the amount as well as the transaction, so a lost confirm is
// legible as "this much spending went uncounted" rather than as an opaque id.
func (uc *UseCase) recordReservationByTransactionFailure(ctx context.Context, span trace.Span, logger libLog.Logger, transition reservationTransition, err error) {
	libOpentelemetry.HandleSpanError(span, "Tracer reservation "+transition.Action+" by transaction transport failed", err)

	logger.Log(ctx, libLog.LevelWarn, "Tracer reservation by-transaction transport failed on the first attempt; retrying off the request path",
		append(transition.logFields(), libLog.Err(err)))

	uc.scheduleReservationRetry(ctx, logger, transition, err)
}

// reservationTTLForStatus selects the TTL policy from the transaction status:
// PENDING transactions get the long-lived hint, everything else gets the
// default reaper-swept TTL.
func reservationTTLForStatus(transactionStatus string) reservationTTLPolicy {
	if transactionStatus == constant.PENDING {
		return reservationTTLLongLived
	}

	return reservationTTLDefault
}

// reservationPurposeForAction maps a run's action to the reservation purpose:
// only a revert is marked as one.
func reservationPurposeForAction(action string) reservationPurpose {
	if action == constant.ActionRevert {
		return reservationForRevert
	}

	return reservationForCreate
}
