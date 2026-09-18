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
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// accountClosingAttempt is the protection one closing attempt holds over its
// account: the administrative ownership and the closing marker, both carrying the
// same token so reconciliation can recognize them as one attempt.
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
// the account is read from the PRIMARY, the administrative ownership and the
// closing marker are taken BEFORE the final checks, and only then are balances,
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

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "close_account", start, err)
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

	return uc.finalizeAccountClosing(ctx, span, logger, attempt, states)
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
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to read the account to close", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to read the account to close", libLog.Err(err))

		return err
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

// protectAccountClosing takes the administrative ownership of the account and
// installs the closing marker under the same token.
//
// The ownership is what serializes this attempt against balance creation, balance
// deletion and cache-miss admission; the marker is what the engine reads, so a new
// execution is refused from here on. The order matters: the ownership is taken
// first, so two attempts cannot both reach the marker, and the marker is installed
// BEFORE the final verification, so what the verification observes can no longer
// change underneath it.
func (uc *UseCase) protectAccountClosing(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	organizationID, ledgerID, accountID uuid.UUID,
) (*accountClosingAttempt, error) {
	admission, err := uc.acquireAccountAdmission(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to protect the account for closing", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to protect the account for closing", libLog.Err(err))

		return nil, err
	}

	attempt := &accountClosingAttempt{
		organizationID: organizationID,
		ledgerID:       ledgerID,
		accountID:      accountID,
		admission:      admission,
		token:          admission.Token(),
	}

	installed, err := uc.TransactionRedisRepo.AcquireAccountClosingMarker(ctx, organizationID, ledgerID, accountID, attempt.token)
	if err != nil {
		indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

		libOpentelemetry.HandleSpanError(span, "Failed to install the account closing marker", err)
		logger.Log(ctx, libLog.LevelError, "Failed to install the account closing marker", libLog.Err(err))

		// The marker write may have landed with its answer lost, so the account stays
		// protected and reconciliation resolves it.
		attempt.retain()
		uc.releaseAccountClosingAttempt(ctx, attempt)

		return nil, indeterminate
	}

	if !installed {
		conflict := pkg.ValidateBusinessError(constant.ErrAccountClosingInProgress, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Another attempt already owns the account closing", conflict)

		admission.Release(ctx)

		return nil, conflict
	}

	return attempt, nil
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
// transaction can still move the account, on either side.
//
// Every monetary component may read zero and the account still be one commit away
// from moving, so this is a refusal of its own class and not a balance residual.
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
// closing marker first, the ownership after, both only where the key still carries
// this attempt's token.
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

	if !attempt.markerReleased {
		released, err := uc.TransactionRedisRepo.ReleaseAccountClosingAttempt(cleanupCtx, attempt.organizationID, attempt.ledgerID, attempt.accountID, attempt.token)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to remove the account closing marker", err)
			logger.Log(cleanupCtx, libLog.LevelWarn, "Failed to remove the account closing marker", libLog.Err(err))
		}

		if !released && err == nil {
			logger.Log(cleanupCtx, libLog.LevelDebug, "The account closing marker was not owned at release")
		}
	}

	attempt.admission.Release(cleanupCtx)
}
