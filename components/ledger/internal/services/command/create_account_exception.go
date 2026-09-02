// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CreateAccountException registers a new account exception scoped to one account.
//
// Registering an exception NEVER mutates the account's blocked flag or any allow flag
// (RF-05): the only write is the new row in the account_exception table. The audit
// trail of who/when is carried by the emitted account_exception.created event (RF-08),
// exactly as the phase-1 block/unblock path carries it through account.updated.
//
// Sequence, and why the order is load-bearing:
//
//  1. Load the account. A missing account is a 404, decided before anything is written.
//  2. Reject an exception on an external account (0074): value crossing the ledger
//     boundary settles through external accounts, and an exception there has no
//     semantics — the same guard, code and position block/unblock and UpdateAccount use.
//  3. Validate the validity window (0505) over the input (the create-time final state).
//  4. Build the entity (UUIDv7 identity, UTC timestamps) and persist it.
//  5. Emit account_exception.created, best-effort, after the row is durable.
func (uc *UseCase) CreateAccountException(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, input *mmodel.CreateAccountExceptionInput) (_ *mmodel.AccountException, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account_exception")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_account_exception", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	// Invalidate-first: empty the exceptions cache entry BEFORE any Postgres write.
	// A failure here refuses the write while nothing has changed, so the cache stays
	// consistent with the database (see invalidateAccountExceptionsCache).
	if err = uc.invalidateAccountExceptionsCache(ctx, span, logger, organizationID, ledgerID, accountID); err != nil {
		return nil, err
	}

	if err = uc.assertAccountAcceptsException(ctx, span, logger, organizationID, ledgerID, accountID); err != nil {
		return nil, err
	}

	if err = mmodel.ValidateAccountExceptionWindow(input.EffectiveAt, input.ExpiresAt); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account exception validity window", err)
		logger.Log(ctx, libLog.LevelWarn, "Invalid account exception validity window", libLog.Err(err))

		return nil, err
	}

	exceptionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate account exception ID", err)
		logger.Log(ctx, libLog.LevelError, "Failed to generate account exception ID", libLog.Err(err))

		return nil, err
	}

	now := time.Now().UTC()

	exception := &mmodel.AccountException{
		ID:                   exceptionID.String(),
		OrganizationID:       organizationID.String(),
		LedgerID:             ledgerID.String(),
		AccountID:            accountID.String(),
		OperationalTypeCodes: input.OperationalTypeCodes,
		BalanceKey:           input.BalanceKey,
		Context:              input.Context,
		EffectiveAt:          input.EffectiveAt,
		ExpiresAt:            input.ExpiresAt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	created, err := uc.AccountExceptionRepo.Create(ctx, organizationID, ledgerID, exception)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create account exception", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create account exception", libLog.Err(err))

		return nil, err
	}

	uc.emitAccountExceptionCreatedEvent(ctx, span, logger, created)

	return created, nil
}

// invalidateAccountExceptionsCache empties the per-account exceptions cache entry
// BEFORE the Postgres write (write-through invalidate-first). Failing here is free —
// nothing has committed — so an unreachable Redis or a failed Del REFUSES the write,
// keeping the cache consistent with the database. There is deliberately NO post-commit
// repopulate: the key stays empty and the read path (GetActiveAccountExceptions)
// refills it on the next miss, which removes the writer-reordering race entirely.
//
// A nil OnboardingRedisRepo (cache disabled) is a no-op, so single binaries running
// without a cache keep working. The log field carrying the key is named "cache_entry"
// on purpose: lib-observability's zap logger redacts any field name containing the
// token "key", which would strip the one datum an operator needs from the alert.
func (uc *UseCase) invalidateAccountExceptionsCache(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID, accountID uuid.UUID) error {
	if uc.OnboardingRedisRepo == nil {
		return nil
	}

	cacheKey := utils.AccountExceptionsInternalKey(organizationID, ledgerID, accountID)

	if err := uc.OnboardingRedisRepo.Del(ctx, cacheKey); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to invalidate account exceptions cache before write", err)
		logger.Log(ctx, libLog.LevelError, "Failed to invalidate account exceptions cache, refusing write",
			libLog.String("cache_entry", cacheKey), libLog.Err(err))

		return err
	}

	return nil
}

// assertAccountAcceptsException loads the target account and rejects the two states an
// exception cannot be registered against: a missing account (404, ErrAccountIDNotFound)
// and an external account (0074, ErrForbiddenExternalAccountManipulation).
func (uc *UseCase) assertAccountAcceptsException(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID, accountID uuid.UUID) error {
	account, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, accountID, mmodel.HolderOffV1)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find account for exception", err)
		logger.Log(ctx, libLog.LevelError, "Failed to find account for exception", libLog.Err(err))

		return err
	}

	if account == nil {
		err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account not found for exception", err)
		logger.Log(ctx, libLog.LevelWarn, "Account not found for exception",
			libLog.String("account_id", accountID.String()))

		return err
	}

	if account.Type == "external" {
		err = pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rejected exception on external account", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected exception on external account",
			libLog.String("account_id", accountID.String()))

		return err
	}

	return nil
}

// emitAccountExceptionCreatedEvent publishes account_exception.created for a persisted
// exception. IMPORTANT posture: build/emit failures are span-recorded and logged at Warn
// by the helper, never returned; durability is owned by PG, not by broker delivery.
//
// Wire-format mapping lives in pkg/streaming/events/account_exception_created.go.
func (uc *UseCase) emitAccountExceptionCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, e *mmodel.AccountException) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.AccountExceptionCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewAccountExceptionCreated(e).ToEmitRequest(tenantID, e.CreatedAt)
		})
}
