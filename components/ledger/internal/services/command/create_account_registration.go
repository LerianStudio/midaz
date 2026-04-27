// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/v4/commons/opentelemetry/metrics"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// retryBackoff is the cooldown applied before the Phase 5 recovery worker picks up
// a FAILED_RETRYABLE saga again. Five seconds is a deliberately-short default: the
// worker adds its own jitter on top and every transient-failure path here bumps the
// retry count, so exponential backoff will be layered on in Phase 5 without a code
// change here.
const retryBackoff = 5 * time.Second

// Failure reason labels exposed as the "reason" attribute on
// account_registration_failed_total. Kept as typed constants so lint catches typos
// and dashboards have a stable enum.
const (
	reasonHolderNotFound      = "HOLDER_NOT_FOUND"
	reasonCRMTransient        = "CRM_TRANSIENT"
	reasonCRMConflict         = "CRM_CONFLICT"
	reasonCRMBadRequest       = "CRM_BAD_REQUEST"
	reasonAccountCreateFailed = "ACCOUNT_CREATE_FAILED"
	reasonAliasCreateFailed   = "ALIAS_CREATE_FAILED"
	reasonActivateFailed      = "ACTIVATE_FAILED"
	reasonIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	reasonPersistenceFailed   = "PERSISTENCE_FAILED"
)

// CreateAccountRegistration is the public entry point for the Ledger-owned
// account-registration saga. It orchestrates the coordinated creation of a Ledger
// account and its CRM holder alias across eleven durable checkpoints.
//
// Contract:
//   - On full success: returns the final registration plus the created Account and Alias.
//   - On idempotent replay (existing COMPLETED row with matching hash): returns the
//     stored registration plus the previously-created Account and Alias (reloaded).
//   - On a transient failure: persists FAILED_RETRYABLE and returns the registration
//     with the wrapped error. Phase 5's recovery worker picks it up after retryBackoff.
//   - On a terminal failure: persists FAILED_TERMINAL and returns the registration
//     with the wrapped error. No further automated attempts will be made.
//
// The saga never compensates ledger-side writes on a transient failure. Only the
// recovery worker (Phase 5) drives forward progress or explicit compensation.
//
//nolint:gocyclo // Saga is linear; cyclomatic complexity reflects explicit state-machine branches, not hidden nesting. Splitting would hide the order of steps.
func (uc *UseCase) CreateAccountRegistration(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	input *mmodel.CreateAccountRegistrationInput,
	idempotencyKey, token string,
) (*mmodel.AccountRegistration, *mmodel.Account, *mmodel.Alias, error) {
	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account_registration")
	defer span.End()

	span.SetAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("idempotency_key", idempotencyKey),
	)

	if input == nil {
		err := errors.New("command: CreateAccountRegistration requires a non-nil input")

		libOpentelemetry.HandleSpanError(span, "Nil account-registration input", err)
		logger.Log(ctx, libLog.LevelError, "Nil account-registration input")

		return nil, nil, nil, err
	}

	if idempotencyKey == "" {
		businessErr := pkg.ValidateBusinessError(constant.ErrIdempotencyKeyRequired, constant.EntityAccountRegistration)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Idempotency-Key header missing", businessErr)

		return nil, nil, nil, businessErr
	}

	// Step 1: Canonical hash of the request body. The hash is persisted so a
	// subsequent call with the same key but a different body is rejected.
	requestHash, err := utils.CanonicalHashJSON(input)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to compute canonical request hash", err)
		logger.Log(ctx, libLog.LevelError, "Failed to compute canonical request hash", libLog.Err(err))

		return nil, nil, nil, fmt.Errorf("account-registration: canonical hash: %w", err)
	}

	regID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to allocate registration ID", err)
		logger.Log(ctx, libLog.LevelError, "Failed to allocate registration ID", libLog.Err(err))

		return nil, nil, nil, fmt.Errorf("account-registration: allocate id: %w", err)
	}

	now := time.Now().UTC()

	seed := &mmodel.AccountRegistration{
		ID:             regID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		HolderID:       input.HolderID,
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		Status:         mmodel.AccountRegistrationReceived,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Step 2: Claim the idempotency slot. The repository enforces the
	// (org, ledger, key) uniqueness and returns wasCreated=false for replays.
	reg, wasCreated, err := uc.AccountRegistrationRepo.UpsertByIdempotencyKey(ctx, seed)
	if err != nil {
		if errors.Is(err, constant.ErrAccountRegistrationIdempotencyConflict) {
			uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonIdempotencyConflict)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Idempotency conflict", err)

			return nil, nil, nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to persist account registration", err)
		logger.Log(ctx, libLog.LevelError, "Failed to persist account registration",
			libLog.String("organization_id", organizationID.String()),
			libLog.String("ledger_id", ledgerID.String()),
			libLog.String("idempotency_key", idempotencyKey),
			libLog.Err(err))

		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)

		return nil, nil, nil, fmt.Errorf("account-registration: upsert: %w", err)
	}

	if !wasCreated {
		// Replay path. If the stored registration already reached COMPLETED we load
		// the account + alias and short-circuit. Anything else means the caller is
		// retrying while a prior attempt is still in flight (or failed); return the
		// current state without re-running any steps so the caller can poll GET.
		if reg.Status == mmodel.AccountRegistrationCompleted {
			account, alias := uc.loadReplayArtifacts(ctx, organizationID, ledgerID, reg, token)

			logger.Log(ctx, libLog.LevelInfo, "Account registration replayed",
				libLog.String("id", reg.ID.String()),
				libLog.String("idempotency_key", idempotencyKey))

			return reg, account, alias, nil
		}

		logger.Log(ctx, libLog.LevelInfo, "Account registration already exists and is in progress",
			libLog.String("id", reg.ID.String()),
			libLog.String("status", string(reg.Status)))

		return reg, nil, nil, nil
	}

	uc.emitSagaStarted(ctx, logger, metricFactory, organizationID, ledgerID)

	// Step 3: Validate the holder on CRM.
	if _, err := uc.CRMClient.GetHolder(ctx, organizationID.String(), input.HolderID, token); err != nil {
		reason, status := classifySagaError(err)
		uc.recordFailure(ctx, logger, reg.ID, status, reason, err)
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reason)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "CRM holder validation failed", err)

		reg.Status = status

		return reg, nil, nil, err
	}

	// Step 4: Mark HOLDER_VALIDATED before the next side-effect.
	if err := uc.AccountRegistrationRepo.UpdateStatus(ctx, reg.ID, mmodel.AccountRegistrationHolderValidated); err != nil {
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)
		libOpentelemetry.HandleSpanError(span, "Failed to mark holder_validated", err)

		return reg, nil, nil, fmt.Errorf("account-registration: mark holder_validated: %w", err)
	}

	reg.Status = mmodel.AccountRegistrationHolderValidated

	// Step 5: Create the Ledger account in PENDING_CRM_LINK state.
	account, err := uc.createAccountWithOptions(ctx, organizationID, ledgerID, &input.Account, token, accountCreateOptions{PendingCRMLink: true})
	if err != nil {
		uc.recordFailure(ctx, logger, reg.ID, mmodel.AccountRegistrationFailedRetryable, reasonAccountCreateFailed, err)
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonAccountCreateFailed)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create Ledger account", err)

		reg.Status = mmodel.AccountRegistrationFailedRetryable

		return reg, nil, nil, err
	}

	accountID, parseErr := uuid.Parse(account.ID)
	if parseErr != nil {
		// Defensive: account.ID is set by the ledger side and should always parse.
		// Surface as persistence-level failure rather than silently continuing.
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)
		libOpentelemetry.HandleSpanError(span, "Failed to parse created account ID", parseErr)

		return reg, account, nil, fmt.Errorf("account-registration: parse account id: %w", parseErr)
	}

	// Step 6: Record the account id and transition to LEDGER_ACCOUNT_CREATED.
	if err := uc.AccountRegistrationRepo.AttachAccount(ctx, reg.ID, accountID); err != nil {
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)
		libOpentelemetry.HandleSpanError(span, "Failed to attach account ID", err)

		return reg, account, nil, fmt.Errorf("account-registration: attach account: %w", err)
	}

	reg.AccountID = &accountID

	if err := uc.AccountRegistrationRepo.UpdateStatus(ctx, reg.ID, mmodel.AccountRegistrationLedgerAccountCreated); err != nil {
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)
		libOpentelemetry.HandleSpanError(span, "Failed to mark ledger_account_created", err)

		return reg, account, nil, fmt.Errorf("account-registration: mark ledger_account_created: %w", err)
	}

	reg.Status = mmodel.AccountRegistrationLedgerAccountCreated

	// Step 7: Create the CRM alias. Scope the alias input to the ledger+account the
	// saga just produced so the client cannot point the alias at a different account.
	aliasInput := input.CRMAlias
	aliasInput.LedgerID = ledgerID.String()
	aliasInput.AccountID = account.ID

	aliasIdempotencyKey := fmt.Sprintf("account-registration:%s:%s:%s:crm-create-alias",
		organizationID, ledgerID, idempotencyKey)

	alias, err := uc.CRMClient.CreateAccountAlias(ctx, organizationID.String(), input.HolderID, &aliasInput, aliasIdempotencyKey, token)
	if err != nil {
		reason, status := classifySagaError(err)
		// Remap generic categories onto alias-specific reasons for dashboards.
		if reason == reasonCRMTransient {
			reason = reasonAliasCreateFailed
			status = mmodel.AccountRegistrationFailedRetryable
		}

		uc.recordFailure(ctx, logger, reg.ID, status, reason, err)
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reason)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create CRM alias", err)

		reg.Status = status

		return reg, account, nil, err
	}

	if alias == nil || alias.ID == nil {
		err := errors.New("account-registration: CRM returned nil alias ID")
		uc.recordFailure(ctx, logger, reg.ID, mmodel.AccountRegistrationFailedRetryable, reasonAliasCreateFailed, err)
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonAliasCreateFailed)
		libOpentelemetry.HandleSpanError(span, "CRM returned nil alias ID", err)

		reg.Status = mmodel.AccountRegistrationFailedRetryable

		return reg, account, alias, err
	}

	// Step 8: Record the alias id and transition to CRM_ALIAS_CREATED.
	if err := uc.AccountRegistrationRepo.AttachCRMAlias(ctx, reg.ID, *alias.ID); err != nil {
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)
		libOpentelemetry.HandleSpanError(span, "Failed to attach CRM alias ID", err)

		return reg, account, alias, fmt.Errorf("account-registration: attach alias: %w", err)
	}

	aliasID := *alias.ID
	reg.CRMAliasID = &aliasID

	if err := uc.AccountRegistrationRepo.UpdateStatus(ctx, reg.ID, mmodel.AccountRegistrationCRMAliasCreated); err != nil {
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)
		libOpentelemetry.HandleSpanError(span, "Failed to mark crm_alias_created", err)

		return reg, account, alias, fmt.Errorf("account-registration: mark crm_alias_created: %w", err)
	}

	reg.Status = mmodel.AccountRegistrationCRMAliasCreated

	// Step 9: Activate the account. This flips PENDING_CRM_LINK -> ACTIVE and
	// unblocks the default balance. Failure is retryable: the worker will re-run
	// activation on the next recovery cycle.
	if err := uc.ActivateAccount(ctx, organizationID, ledgerID, accountID); err != nil {
		uc.recordFailure(ctx, logger, reg.ID, mmodel.AccountRegistrationFailedRetryable, reasonActivateFailed, err)
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonActivateFailed)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to activate account", err)

		reg.Status = mmodel.AccountRegistrationFailedRetryable

		return reg, account, alias, err
	}

	// Step 10: Mark ACCOUNT_ACTIVATED. A crash between 10 and 11 leaves the account
	// live and the registration one hop short of COMPLETED — the recovery worker
	// finishes the flip on the next pass.
	if err := uc.AccountRegistrationRepo.UpdateStatus(ctx, reg.ID, mmodel.AccountRegistrationAccountActivated); err != nil {
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)
		libOpentelemetry.HandleSpanError(span, "Failed to mark account_activated", err)

		return reg, account, alias, fmt.Errorf("account-registration: mark account_activated: %w", err)
	}

	reg.Status = mmodel.AccountRegistrationAccountActivated

	// Step 11: Final COMPLETED flip with completed_at timestamp.
	completedAt := time.Now().UTC()

	if err := uc.AccountRegistrationRepo.MarkCompleted(ctx, reg.ID, completedAt); err != nil {
		uc.emitSagaFailed(ctx, logger, metricFactory, organizationID, ledgerID, reasonPersistenceFailed)
		libOpentelemetry.HandleSpanError(span, "Failed to mark completed", err)

		return reg, account, alias, fmt.Errorf("account-registration: mark completed: %w", err)
	}

	reg.Status = mmodel.AccountRegistrationCompleted
	reg.CompletedAt = &completedAt

	uc.emitSagaCompleted(ctx, logger, metricFactory, organizationID, ledgerID)

	logger.Log(ctx, libLog.LevelInfo, "Account registration completed",
		libLog.String("id", reg.ID.String()),
		libLog.String("account_id", account.ID),
		libLog.String("alias_id", aliasID.String()))

	return reg, account, alias, nil
}

// classifySagaError inspects a saga error and returns (reason, status) so the caller
// can record the right failure kind and terminal-vs-retryable flag without repeating
// the errors.Is cascade inline.
func classifySagaError(err error) (string, mmodel.AccountRegistrationStatus) {
	switch {
	case errors.Is(err, constant.ErrHolderNotFound):
		return reasonHolderNotFound, mmodel.AccountRegistrationFailedTerminal
	case errors.Is(err, constant.ErrCRMTransient):
		return reasonCRMTransient, mmodel.AccountRegistrationFailedRetryable
	case errors.Is(err, constant.ErrCRMConflict),
		errors.Is(err, constant.ErrAliasHolderConflict),
		errors.Is(err, constant.ErrIdempotencyKey):
		return reasonCRMConflict, mmodel.AccountRegistrationFailedTerminal
	case errors.Is(err, constant.ErrCRMBadRequest):
		return reasonCRMBadRequest, mmodel.AccountRegistrationFailedTerminal
	default:
		// Conservative default: treat unknown errors as retryable so Phase 5 picks
		// them up. A stuck registration is preferable to a silently-terminal one.
		return reasonCRMTransient, mmodel.AccountRegistrationFailedRetryable
	}
}

// recordFailure persists a saga failure in a single statement, attaching a
// next_retry_at when the status is retryable.
func (uc *UseCase) recordFailure(ctx context.Context, logger libLog.Logger, id uuid.UUID, status mmodel.AccountRegistrationStatus, reason string, cause error) {
	message := ""
	if cause != nil {
		message = cause.Error()
	}

	if err := uc.AccountRegistrationRepo.MarkFailed(ctx, id, status, reason, message); err != nil {
		// Persistence failure after a downstream failure is logged at Error: the
		// registration is now out of sync with the actual side-effect state and
		// operators need to know.
		logger.Log(ctx, libLog.LevelError, "Failed to persist saga failure",
			libLog.String("registration_id", id.String()),
			libLog.String("reason", reason),
			libLog.Err(err))

		return
	}

	if status == mmodel.AccountRegistrationFailedRetryable {
		retryAt := time.Now().UTC().Add(retryBackoff)

		mutator := func(builder squirrel.UpdateBuilder) squirrel.UpdateBuilder {
			return builder.Set("next_retry_at", retryAt)
		}

		if err := uc.AccountRegistrationRepo.UpdateStatus(ctx, id, status, mutator); err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to set next_retry_at on retryable failure",
				libLog.String("registration_id", id.String()),
				libLog.Err(err))
		}
	}
}

// loadReplayArtifacts re-resolves the Account and Alias for a COMPLETED saga replay.
// Failures are logged but not propagated: the caller still receives a valid
// registration and can read the replay's artifacts via the regular GET endpoints.
func (uc *UseCase) loadReplayArtifacts(ctx context.Context, organizationID, ledgerID uuid.UUID, reg *mmodel.AccountRegistration, token string) (*mmodel.Account, *mmodel.Alias) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_account_registration.load_replay_artifacts")
	defer span.End()

	var (
		account *mmodel.Account
		alias   *mmodel.Alias
	)

	if reg.AccountID != nil {
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, *reg.AccountID)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to reload account for replay",
				libLog.String("registration_id", reg.ID.String()),
				libLog.String("account_id", reg.AccountID.String()),
				libLog.Err(err))
		} else {
			account = acc
		}
	}

	if reg.AccountID != nil {
		a, err := uc.CRMClient.GetAliasByAccount(ctx, organizationID.String(), ledgerID.String(), reg.AccountID.String(), token)
		if err != nil && !errors.Is(err, constant.ErrAliasNotFound) {
			logger.Log(ctx, libLog.LevelWarn, "Failed to reload alias for replay",
				libLog.String("registration_id", reg.ID.String()),
				libLog.String("account_id", reg.AccountID.String()),
				libLog.Err(err))
		} else if err == nil {
			alias = a
		}
	}

	return account, alias
}

// Metric helpers. Emission failures are non-fatal — the saga's durable state is the
// source of truth, metrics are a debugging aid — so every error here downgrades to a
// warn log and swallows.

func (uc *UseCase) emitSagaStarted(ctx context.Context, logger libLog.Logger, factory *metrics.MetricsFactory, organizationID, ledgerID uuid.UUID) {
	emitCounter(ctx, logger, factory, utils.AccountRegistrationStartedTotal, map[string]string{
		"organization_id": organizationID.String(),
		"ledger_id":       ledgerID.String(),
	})
}

func (uc *UseCase) emitSagaCompleted(ctx context.Context, logger libLog.Logger, factory *metrics.MetricsFactory, organizationID, ledgerID uuid.UUID) {
	emitCounter(ctx, logger, factory, utils.AccountRegistrationCompletedTotal, map[string]string{
		"organization_id": organizationID.String(),
		"ledger_id":       ledgerID.String(),
	})
}

func (uc *UseCase) emitSagaFailed(ctx context.Context, logger libLog.Logger, factory *metrics.MetricsFactory, organizationID, ledgerID uuid.UUID, reason string) {
	emitCounter(ctx, logger, factory, utils.AccountRegistrationFailedTotal, map[string]string{
		"organization_id": organizationID.String(),
		"ledger_id":       ledgerID.String(),
		"reason":          reason,
	})
}

func emitCounter(ctx context.Context, logger libLog.Logger, factory *metrics.MetricsFactory, metric metrics.Metric, labels map[string]string) {
	if factory == nil {
		return
	}

	counter, err := factory.Counter(metric)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to build counter",
			libLog.String("metric", metric.Name),
			libLog.Err(err))

		return
	}

	if err := counter.WithLabels(labels).AddOne(ctx); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to emit counter",
			libLog.String("metric", metric.Name),
			libLog.Err(err))
	}
}
