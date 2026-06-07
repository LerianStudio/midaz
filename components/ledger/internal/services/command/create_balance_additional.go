// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace"
)

const (
	balanceAccountKeyUniqueIndex = "idx_unique_balance_account_key"
	balanceAliasKeyUniqueIndex   = "idx_unique_balance_alias_key"
)

//nolint:gocyclo // Validation + parent lookup + uniqueness + creation; refactor candidate.
func (uc *UseCase) CreateAdditionalBalance(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, cbi *mmodel.CreateAdditionalBalance) (_ *mmodel.Balance, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_additional_balance")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_balance", start, err)
	}()

	// Reserved key guard: "overdraft" (any case) is system-managed and MUST
	// not be created through the public API. This check runs BEFORE any
	// repository call so a rejected request never touches persistence.
	if strings.EqualFold(cbi.Key, constant.OverdraftBalanceKey) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reserved balance key", nil)
		logger.Log(ctx, libLog.LevelWarn, "Rejected reserved balance key", libLog.String("key", cbi.Key))

		return nil, pkg.ValidateBusinessError(constant.ErrReservedBalanceKey, constant.EntityBalance, cbi.Key)
	}

	// Direction validation runs before any repository call so invalid
	// payloads never touch persistence. Nil Direction is allowed and
	// defaults to "credit" at construction time below.
	if cbi.Direction != nil {
		switch *cbi.Direction {
		case constant.DirectionCredit, constant.DirectionDebit:
			// valid
		default:
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid balance direction", nil)
			logger.Log(ctx, libLog.LevelWarn, "Rejected invalid balance direction", libLog.String("direction", *cbi.Direction))

			return nil, pkg.ValidateBusinessError(constant.ErrInvalidBalanceDirection, constant.EntityBalance, *cbi.Direction)
		}
	}

	// Settings validation also runs pre-persistence to keep the repository
	// free of corrupt payloads.
	if cbi.Settings != nil {
		if err := cbi.Settings.Validate(); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid balance settings", err)
			logger.Log(ctx, libLog.LevelWarn, "Rejected invalid balance settings", libLog.Err(err))

			return nil, pkg.ValidateBusinessError(constant.ErrInvalidBalanceSettings, constant.EntityBalance)
		}
	}

	// Reserved scope guard: "internal" is reserved for system-managed
	// balances (e.g., the auto-created overdraft balance). A client-facing
	// request MUST NOT be able to create an internal-scoped balance —
	// those are permanently undeletable and bypass normal controls.
	if cbi.Settings != nil && cbi.Settings.BalanceScope == mmodel.BalanceScopeInternal {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reserved balance scope", nil)
		logger.Log(ctx, libLog.LevelWarn, "Rejected reserved balance scope", libLog.String("scope", cbi.Settings.BalanceScope))

		return nil, pkg.ValidateBusinessError(constant.ErrInvalidBalanceSettings, constant.EntityBalance)
	}

	existingBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, strings.ToLower(cbi.Key))
	if err != nil {
		var notFound pkg.EntityNotFoundError
		if !errors.As(err, &notFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to check if additional balance already exists", err)
			logger.Log(ctx, libLog.LevelError, "Failed to check if additional balance already exists", libLog.Err(err))

			return nil, err
		}
	}

	if existingBalance != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance already exists", nil)
		logger.Log(ctx, libLog.LevelWarn, "Additional balance already exists", libLog.String("key", cbi.Key))

		return nil, pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, constant.EntityBalance, cbi.Key)
	}

	defaultBalance, err := uc.BalanceRepo.FindByAccountIDAndKey(ctx, organizationID, ledgerID, accountID, constant.DefaultBalanceKey)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get default balance", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get default balance", libLog.Err(err))

		return nil, err
	}

	if defaultBalance.AccountType == constant.ExternalAccountType {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance not allowed for external account type", nil)

		return nil, pkg.ValidateBusinessError(constant.ErrAdditionalBalanceNotAllowed, constant.EntityBalance, defaultBalance.Alias)
	}

	// Direction defaults to "credit" when the caller omits it. Validation
	// above guarantees that any provided value is one of the supported
	// enum members.
	direction := constant.DirectionCredit
	if cbi.Direction != nil {
		direction = *cbi.Direction
	}

	additionalBalance := &mmodel.Balance{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		Alias:          defaultBalance.Alias,
		Key:            strings.ToLower(cbi.Key),
		OrganizationID: defaultBalance.OrganizationID,
		LedgerID:       defaultBalance.LedgerID,
		AccountID:      defaultBalance.AccountID,
		AssetCode:      defaultBalance.AssetCode,
		AccountType:    defaultBalance.AccountType,
		AllowSending:   cbi.AllowSending == nil || *cbi.AllowSending,
		AllowReceiving: cbi.AllowReceiving == nil || *cbi.AllowReceiving,
		Direction:      direction,
		Settings:       cbi.Settings,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	created, err := uc.BalanceRepo.Create(ctx, additionalBalance)
	if err != nil {
		// Migration 032 adds a unique balance key index. If another pod wins the
		// race after the precheck, return the same business error as the precheck.
		if isBalanceKeyUniqueViolation(err) {
			berr := pkg.ValidateBusinessError(constant.ErrDuplicatedAliasKeyValue, constant.EntityBalance, strings.ToLower(cbi.Key))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Additional balance already exists", berr)

			logger.Log(ctx, libLog.LevelWarn, "Additional balance already exists", libLog.String("key", strings.ToLower(cbi.Key)))

			return nil, berr
		}

		if isPostgresUniqueViolation(err) {
			libOpentelemetry.HandleSpanEvent(span, "Additional balance unique constraint violation")

			logger.Log(ctx, libLog.LevelWarn, "Additional balance unique constraint violation")

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Error creating additional balance on repo", libLog.Err(err))

		return nil, err
	}

	uc.emitBalanceCreatedEvent(ctx, span, logger, created)

	return created, nil
}

func isBalanceKeyUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr == nil || pgErr.Code != constant.UniqueViolationCode {
		return false
	}

	return pgErr.ConstraintName == balanceAccountKeyUniqueIndex || pgErr.ConstraintName == balanceAliasKeyUniqueIndex
}

func isPostgresUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr != nil && pgErr.Code == constant.UniqueViolationCode
}

// emitBalanceCreatedEvent publishes the balance.created event for a
// balance materialized via CreateAdditionalBalance. IMPORTANT posture:
// build and emit failures are span-recorded and logged at Warn, never
// returned. Durability of the event is owned by PG and (follow-up task)
// the outbox subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after BalanceRepo.Create succeeds on the
// public POST .../accounts/:account_id/balances endpoint. The other
// callers of BalanceRepo.Create — CreateDefaultBalance (auto-default
// path from CreateAccount/CreateAsset) and ensureOverdraftBalance
// (system-managed overdraft companion) — intentionally do NOT emit
// balance.created. The first folds into account.created/asset.created;
// the second into balance.config_changed{changeType=overdraft_enabled}.
//
// Wire-format mapping lives in pkg/streaming/events/balance_created.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitBalanceCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, b *mmodel.Balance) {
	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.BalanceCreatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewBalanceCreated(b).ToEmitRequest(tenantID, b.CreatedAt)
		})
}
