// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// accountClosingAttempt is the protection one closing attempt holds over its
// account: the closing marker and the administrative ownership, both carrying the
// same token so reconciliation can recognize them as one attempt. The ownership
// only ever exists while the marker does: it is taken after the marker and given
// back before it, so the marker is always the anchor through which an interrupted
// attempt's ownership is found and released. admission stays nil until the
// ownership is taken.
//
// retained records that the attempt's outcome could not be established. The
// protection of such an attempt is NOT given back on the way out: a write that may
// still land has to keep the account protected until it is resolved, and the
// alternative — releasing on the way out — is what would let a movement through
// while a closing is still able to commit.
type accountClosingAttempt struct {
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	accountID      uuid.UUID
	admission      *accountprotection.Admission
	token          string
	retained       bool
	markerReleased bool
}

// CloseAccount closes one account after proving that nothing it holds can still
// move, and returns the instant the database recorded.
//
// Authorization is resolved at the transport, so this use case answers only for
// eligibility and ordering. The order is what makes the verification meaningful:
// the account is read from the PRIMARY, the closing marker and the administrative
// ownership are taken BEFORE the final checks, and only then are balances,
// completion and pending transactions examined — an execution that started before
// the protection participates in the verification, and one that starts after it is
// refused.
//
// A blocked account closes like any other: blocking is a live control over
// movement and closing is a state of the account, so neither implies the other.
// An external account never closes, and every refusal below leaves the account
// exactly as it was — no timestamp, no movement, no event.
func (uc *UseCase) CloseAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (closedAt time.Time, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.close_account")
	defer span.End()

	start := time.Now()

	// Single exit boundary of the closing telemetry. The domain metric answers
	// whether the operation succeeded and how long it took; the closing metrics
	// beside it answer, over a bounded vocabulary, why one did not. Neither carries
	// an account, an organization, a ledger or an amount — those live on the span.
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "close_account", start, err)
		recordAccountClosingOutcome(ctx, uc.MetricsFactory, logger, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	if uc.AccountRepo == nil || uc.TransactionRedisRepo == nil {
		err = pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

		libOpentelemetry.HandleSpanError(span, "The account protection surface is not configured", err)
		logger.Log(ctx, libLog.LevelError, "The account protection surface is not configured", libLog.Err(err))

		return time.Time{}, err
	}

	if err = uc.verifyAccountClosingEligibleAccount(ctx, span, logger, organizationID, ledgerID, accountID); err != nil {
		return time.Time{}, err
	}

	attempt, err := uc.protectAccountClosing(ctx, span, logger, organizationID, ledgerID, accountID)
	if err != nil {
		return time.Time{}, err
	}

	defer func() { uc.releaseAccountClosingAttempt(ctx, attempt) }()

	states, err := uc.verifyAccountClosingEligibility(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		return time.Time{}, err
	}

	closedAt, err = uc.finalizeAccountClosing(ctx, span, logger, attempt, states)
	if err != nil {
		return time.Time{}, err
	}

	uc.emitAccountClosedEvent(ctx, span, logger, organizationID, ledgerID, accountID, closedAt)

	return closedAt, nil
}

// emitAccountClosedEvent publishes account.closed for a closing that finished.
//
// Anchor: the single path that answers success. The finalization it follows has
// already recorded the instant, evicted the cached balances, installed the closed
// marker and removed the closing one, so nothing the event announces can still be
// in doubt. Every other path — a refusal, a technical failure, a finalization whose
// outcome stayed unknown — leaves this call unreached, and a repeat of the command
// is refused as already closed before it, which is what keeps one closing to one
// event.
//
// IMPORTANT posture: a build or emit failure is recorded and logged at Warn, never
// returned. The closing is durable in the account row and the answer stays 204.
// There is no outbox behind it, so a process that dies here loses the event rather
// than replaying it — the closing state remains queryable, which is what consumers
// fall back on.
//
// The instant is the database's, carried through from the conditional write: no
// clock of this process reaches either the payload or the timestamp.
//
// Wire-format mapping lives in pkg/streaming/events/account_closed.go; payload
// changes belong there, not here.
func (uc *UseCase) emitAccountClosedEvent(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	organizationID, ledgerID, accountID uuid.UUID,
	closedAt time.Time,
) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.AccountClosedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountClosed(accountID.String(), organizationID.String(), ledgerID.String(), closedAt).
				ToEmitRequest(tenantID, closedAt)
		})
}

// verifyAccountClosingEligibleAccount answers the questions that do not need the
// protection: the account exists in the scope, it is not external, and it is not
// already closed.
//
// The row is read from the PRIMARY because a closing that committed a moment ago
// must be visible here — replica lag would present a closed account as open and
// turn the single-shot transition into a second one. The conditional write still
// decides the race; this read exists so the common repeat is answered as the
// conflict it is instead of reaching the protection.
func (uc *UseCase) verifyAccountClosingEligibleAccount(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	organizationID, ledgerID, accountID uuid.UUID,
) error {
	acc, err := uc.AccountRepo.Find(readrouting.WithPrimaryRead(ctx), organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1)
	if err != nil {
		// The repository answers a missing row with a business error and everything
		// else — a lost connection, a deadline, a scan failure — with the driver's
		// own. Only the first describes the account; the second says the closing
		// state could not be established at all, which is the dependency refusal
		// rather than a fact about this account.
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to read the account to close", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to read the account to close", libLog.Err(err))

			return err
		}

		indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

		libOpentelemetry.HandleSpanError(span, "Failed to read the account to close", err)
		logger.Log(ctx, libLog.LevelError, "Failed to read the account to close", libLog.Err(err))

		return indeterminate
	}

	if acc == nil {
		notFound := pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account to close was not found in the scope", notFound)

		return notFound
	}

	if strings.EqualFold(acc.Type, constant.ExternalAccountType) {
		forbidden := pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "An external account cannot be closed", forbidden)

		return forbidden
	}

	if acc.ClosedAt != nil {
		alreadyClosed := pkg.ValidateBusinessError(constant.ErrAccountAlreadyClosed, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account is already closed", alreadyClosed)

		return alreadyClosed
	}

	return nil
}

// protectAccountClosing installs the closing marker and then takes the
// administrative ownership of the account under the same token.
//
// The marker is what the engine and every other protected operation read, so a
// new execution, seed admission, balance creation or deletion is refused as a
// closing from here on. The ownership is what excludes the operations already in
// flight: live seed admissions, a balance creation or a deletion make it fail.
//
// The marker goes first because it is also what decides between two closings:
// only one attempt can install it, so a second closing always loses on the marker
// and is told a closing holds the account — it never meets an ownership without a
// marker, which would read as some other operation. Both are in place BEFORE the
// final verification, so what the verification observes can no longer change
// underneath it.
//
// An attempt refused after installing its marker never issued its write, so it
// gives the marker back — unless the refusal left an outcome unknown, in which
// case the marker stays as the anchor reconciliation resolves.
func (uc *UseCase) protectAccountClosing(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	organizationID, ledgerID, accountID uuid.UUID,
) (*accountClosingAttempt, error) {
	attempt := &accountClosingAttempt{
		organizationID: organizationID,
		ledgerID:       ledgerID,
		accountID:      accountID,
		token:          uuid.NewString(),
	}

	installed, err := uc.TransactionRedisRepo.AcquireAccountClosingMarker(ctx, organizationID, ledgerID, accountID, attempt.token)
	if err != nil {
		indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

		libOpentelemetry.HandleSpanError(span, "Failed to install the account closing marker", err)
		logger.Log(ctx, libLog.LevelError, "Failed to install the account closing marker", libLog.Err(err))

		// The marker write may have landed with its answer lost, so it stays for
		// reconciliation, which gives back an attempt that never wrote. Nothing is
		// cleaned up here on purpose.
		return nil, indeterminate
	}

	if !installed {
		return nil, uc.refuseAccountClosingMarker(ctx, span, logger, organizationID, ledgerID, accountID)
	}

	admission, err := uc.accountProtectionGuard().AcquireClosingOwnership(ctx, organizationID, ledgerID, accountID, attempt.token)
	if err != nil {
		// The acquisition refuses in two classes: another operation holding the
		// account, which is the coordination doing its job, and a protection surface
		// that could not be read at all, which is a dependency failing. The second
		// must reach the span as the technical failure the guard already recorded it
		// as, or this span would stay green over a red one.
		const message = "Failed to protect the account for closing"

		if isAccountClosingIndeterminate(err) {
			libOpentelemetry.HandleSpanError(span, message, err)
			logger.Log(ctx, libLog.LevelError, message, libLog.Err(err))

			// The ownership write may have landed with its answer lost. The marker is
			// the anchor through which reconciliation releases both, so nothing is
			// cleaned up here on purpose.
			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, message, err)
		logger.Log(ctx, libLog.LevelWarn, message, libLog.Err(err))

		uc.releaseAccountClosingAttempt(ctx, attempt)

		return nil, err
	}

	attempt.admission = admission

	return attempt, nil
}

// refuseAccountClosingMarker answers an attempt whose closing marker could not be
// installed because the key is already there. That key is another attempt's
// marker, and the answer is the closing in progress — unless what it holds cannot
// be read, which is a protection failure rather than a closing and is refused as
// indeterminate. The key is read once more only to tell those two apart; a marker
// that went away in between still belonged to the closing that refused this one.
func (uc *UseCase) refuseAccountClosingMarker(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	organizationID, ledgerID, accountID uuid.UUID,
) error {
	if _, _, err := uc.TransactionRedisRepo.GetAccountClosingMarker(ctx, organizationID, ledgerID, accountID); err != nil {
		indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

		libOpentelemetry.HandleSpanError(span, "Failed to read the account closing marker", err)
		logger.Log(ctx, libLog.LevelError, "Failed to read the account closing marker", libLog.Err(err))

		return indeterminate
	}

	conflict := pkg.ValidateBusinessError(constant.ErrAccountClosingInProgress, constant.EntityAccount)

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Another attempt already owns the account closing", conflict)
	logger.Log(ctx, libLog.LevelWarn, "Another attempt already owns the account closing", libLog.Err(conflict))

	return conflict
}

// verifyAccountClosingEligibility runs the checks that decide whether the account
// may close, and returns the balance states the finalization evicts.
//
// The order is the one the evidence requires. Live money answers first, because a
// residual settles the question without any further read. Completion comes next,
// since a row can only be read as final once the work behind it concluded, and the
// pending query comes LAST for the same reason: a pending transaction whose rows
// are still being projected is invisible to SQL, so asking before completion was
// proven would read an absence that has not happened yet.
func (uc *UseCase) verifyAccountClosingEligibility(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]accountClosingBalanceState, error) {
	states, err := uc.loadAccountClosingBalanceStates(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		return nil, err
	}

	if err := verifyAccountClosingBalancesZeroed(states); err != nil {
		return nil, err
	}

	if err := uc.verifyNoAccountClosingRecoveryPending(ctx, organizationID, ledgerID, accountID); err != nil {
		return nil, err
	}

	if err := uc.verifyAccountClosingBalancePersistence(ctx, organizationID, ledgerID, states); err != nil {
		return nil, err
	}

	if err := uc.verifyNoAccountClosingPendingTransaction(ctx, organizationID, ledgerID, accountID); err != nil {
		return nil, err
	}

	return states, nil
}

// verifyNoAccountClosingPendingTransaction refuses the closing while a pending
// transaction still encumbers the account as source. Participation is read from
// the durable operation record, independently of the live-balance evidence, so a
// disagreement between the two fails closed.
//
// A pending naming the account only as destination is deliberately no impediment:
// the hold reserves nothing on the destination side, and the closed-account
// refusal at commit answers that side instead.
func (uc *UseCase) verifyNoAccountClosingPendingTransaction(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.verify_account_closing_pending_transactions")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	found, err := uc.TransactionRepo.HasPendingByAccount(readrouting.WithPrimaryRead(ctx), organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to query the pending transactions of the account", err)
		logger.Log(ctx, libLog.LevelError, "Failed to query the pending transactions of the account", libLog.Err(err))

		return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
	}

	if !found {
		return nil
	}

	pending := pkg.ValidateBusinessError(constant.ErrAccountHasPendingTransactions, constant.EntityAccount)

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "The account participates in a pending transaction", pending)
	logger.Log(ctx, libLog.LevelWarn, "The account participates in a pending transaction", libLog.Err(pending))

	return pending
}

// retain records that the outcome of this attempt could not be established, so its
// protection stays in place for reconciliation instead of being given back.
func (a *accountClosingAttempt) retain() {
	if a == nil {
		return
	}

	a.retained = true

	a.admission.MarkIndeterminate()
}

// accountClosingCleanupTimeout bounds the cleanup that runs once the attempt knows
// its own outcome. It is the deadline of the cleanup itself, not of the request:
// the caller is already leaving, and a cache that stopped answering must not hold
// it any longer than this.
const accountClosingCleanupTimeout = 5 * time.Second

// releaseAccountClosingAttempt gives back the protection of this attempt: the
// ownership first, the closing marker after, both only where the key still carries
// this attempt's token. The ownership goes first so a second closing arriving in
// between still meets the marker, and so an ownership that could not be released
// keeps its anchor: the marker then stays, and reconciliation releases both.
//
// The cleanup runs on a context DECOUPLED from the request, with a deadline of its
// own. A cancelled request is one of the reasons a marker exists in the first
// place, so a cleanup inheriting that cancellation would abandon exactly the
// protection it was installed to release. Nothing else is touched: a marker or
// ownership carrying another attempt's token, the account's blocking and its
// permissions are all outside what this attempt installed.
//
// It never reports failure. The refusal that brought the attempt here is the
// answer the caller gets, and a cleanup that could not run leaves its keys for
// reconciliation rather than replacing that answer with its own.
//
// A retained attempt is left untouched. Removing the marker of a write that may
// still land is the one thing this cleanup must never do, and no amount of elapsed
// time turns an unresolved write into a resolved one.
func (uc *UseCase) releaseAccountClosingAttempt(ctx context.Context, attempt *accountClosingAttempt) {
	if attempt == nil || attempt.retained {
		return
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), accountClosingCleanupTimeout)
	defer cancel()

	cleanupCtx, span := tracer.Start(cleanupCtx, "exec.release_account_closing_attempt")
	defer span.End()

	if err := attempt.admission.ReleaseConfirmed(cleanupCtx); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to release the ownership of the account closing", err)
		logger.Log(cleanupCtx, libLog.LevelWarn, "Failed to release the ownership of the account closing", libLog.Err(err))

		return
	}

	if attempt.markerReleased {
		return
	}

	released, err := uc.TransactionRedisRepo.ReleaseAccountClosingAttempt(cleanupCtx, attempt.organizationID, attempt.ledgerID, attempt.accountID, attempt.token)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to remove the account closing marker", err)
		logger.Log(cleanupCtx, libLog.LevelWarn, "Failed to remove the account closing marker", libLog.Err(err))
	}

	if !released && err == nil {
		logger.Log(cleanupCtx, libLog.LevelDebug, "The account closing marker was not owned at release")
	}
}
