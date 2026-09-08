// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func (uc *UseCase) DeleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.delete_balance")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.balance_id", balanceID.String()),
	)

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "delete_balance", start, err)
	}()

	readCtx := readrouting.WithPrimaryRead(ctx)

	balance, err := uc.BalanceRepo.Find(readCtx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance on repo by id", err)
		logger.Log(ctx, libLog.LevelError, "Error getting balance", libLog.Err(err))

		return err
	}

	// Scope protection: internal-scope balances are managed exclusively by
	// the system (e.g. overdraft reserves) and cannot be deleted via the
	// public API. This guard runs BEFORE the funds-check so the caller
	// receives the specific 0169 code instead of a generic validation.
	if balance != nil && balance.Settings != nil && balance.Settings.BalanceScope == mmodel.BalanceScopeInternal {
		err = pkg.ValidateBusinessError(constant.ErrDeletionOfInternalBalance, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Internal-scope balance cannot be deleted", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected deletion of internal-scope balance", libLog.Err(err))

		return err
	}

	// Plant a delete marker so the honored-lock pre-pass rejects concurrent mutations while the
	// delete is in flight. Release it only when the delete fails, so a rejected guard or a
	// failed soft-delete leaves the balance usable. Successful deletion evicts the cache and then
	// shortens the owned marker atomically; a failed eviction deliberately keeps the long TTL.
	var markerLease []balanceDeleteMarker

	if balance != nil {
		markers, markerErr := uc.plantBalanceDeleteMarkerLease(ctx, organizationID, ledgerID, []*mmodel.Balance{balance})
		if markerErr != nil {
			err = markerErr

			var conflictErr pkg.EntityConflictError
			if errors.As(err, &conflictErr) {
				logger.Log(ctx, libLog.LevelWarn, "Balance delete marker is already owned", libLog.Err(err))
			} else {
				logger.Log(ctx, libLog.LevelError, "Error planting balance delete marker", libLog.Err(err))
			}

			return err
		}

		markerLease = markers

		defer func() {
			if err != nil {
				uc.releaseBalanceDeleteMarkers(ctx, markers)
			}
		}()
	}

	if balanceHasFunds(balance) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance cannot be deleted because it still has funds in it.", err)
		logger.Log(ctx, libLog.LevelWarn, "Error deleting balance", libLog.Err(err))

		return err
	}

	if balance != nil {
		// The Redis snapshot may contain a newer balance than PostgreSQL while its
		// transaction is waiting for asynchronous synchronization. A cache hit must
		// therefore pass the same funds guard before the persisted row is deleted.
		cacheKey := balanceCacheKeyFor(organizationID, ledgerID, balance)

		cacheValue, cacheErr := uc.TransactionRedisRepo.Get(ctx, cacheKey)
		if cacheErr != nil {
			err = fmt.Errorf("failed to get balance cache value: %w", cacheErr)
			libOpentelemetry.HandleSpanError(span, "Failed to get balance cache value on redis", err)
			logger.Log(ctx, libLog.LevelError, "Error getting balance cache value", libLog.Err(err))

			return err
		}

		if cacheValue != "" {
			cachedBalance := mmodel.BalanceRedis{}
			if decodeErr := json.Unmarshal([]byte(cacheValue), &cachedBalance); decodeErr != nil {
				err = fmt.Errorf("failed to decode balance cache value: %w", decodeErr)
				libOpentelemetry.HandleSpanError(span, "Failed to decode balance cache value from redis", err)
				logger.Log(ctx, libLog.LevelError, "Error decoding balance cache value", libLog.Err(err))

				return err
			}

			hasFunds, overdraftErr := balanceRedisHasFunds(cachedBalance)
			if overdraftErr != nil {
				err = fmt.Errorf("failed to parse overdraft used from balance cache: %w", overdraftErr)
				libOpentelemetry.HandleSpanError(span, "Failed to parse overdraft used from balance cache", err)
				logger.Log(ctx, libLog.LevelError, "Error parsing overdraft used from balance cache", libLog.Err(err))

				return err
			}

			if hasFunds {
				err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, constant.EntityBalance)
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance cannot be deleted because it still has funds in its cache snapshot", err)
				logger.Log(ctx, libLog.LevelWarn, "Error deleting balance", libLog.Err(err))

				return err
			}
		}
	}

	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balance on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error delete balance", libLog.Err(err))

		return err
	}

	// Drop the stale cache entry now that the row is soft-deleted. Non-fatal: a failed eviction
	// is logged and never fails the already-committed delete.
	if balance != nil {
		uc.evictBalanceCaches(ctx, organizationID, ledgerID, []*mmodel.Balance{balance}, markerLease)
	}

	uc.emitBalanceDeletedEvent(ctx, span, logger, balance, time.Now())

	return nil
}

// balanceHasFunds reports whether a persisted balance still carries spendable, held, or
// overdraft debt. OverdraftUsed is part of the balance position, so a zero available amount does
// not make the balance safe to delete while overdraft debt remains outstanding.
func balanceHasFunds(balance *mmodel.Balance) bool {
	return balance != nil &&
		(!balance.Available.IsZero() || !balance.OnHold.IsZero() || !balance.OverdraftUsed.IsZero())
}

// balanceRedisHasFunds applies the same deletion guard to a Redis snapshot. BalanceRedis stores
// OverdraftUsed as a decimal string for Lua compatibility: an empty value is the pre-overdraft
// snapshot shape and reads as zero, while a non-empty malformed value returns an error so the
// delete fails closed rather than silently treating unreadable debt as zero.
func balanceRedisHasFunds(balance mmodel.BalanceRedis) (bool, error) {
	parsedOverdraftUsed := decimal.Zero

	if balance.OverdraftUsed != "" {
		parsed, err := decimal.NewFromString(balance.OverdraftUsed)
		if err != nil {
			return false, err
		}

		parsedOverdraftUsed = parsed
	}

	return !balance.Available.IsZero() || !balance.OnHold.IsZero() || !parsedOverdraftUsed.IsZero(), nil
}

// emitBalanceDeletedEvent publishes the balance.deleted event for a
// successfully soft-deleted balance. IMPORTANT posture: build and emit
// failures are span-recorded and logged at Warn, never returned.
// Durability of the event is owned by PG and (follow-up task) the outbox
// subsystem + DLQ, not by the synchronous Emit call.
//
// Anchor: invoked immediately after BalanceRepo.Delete succeeds on the
// explicit DELETE .../balances/:balance_id endpoint. The repository's
// SQL is `UPDATE balance SET deleted_at = NOW()`, so the wall-clock
// captured by the caller and the persisted timestamp differ only by
// clock skew. The cascade delete-all-by-account-id path is NOT covered:
// account.deleted carries that fan-out signal.
//
// The pre-delete Find call (already required by the scope + funds
// guards) supplies the balance's accountId for the payload — the repo's
// Delete signature carries only the balanceID. If Find returned nil
// (unexpected: the guards run after Find), the emission is skipped
// rather than producing a payload with empty identity.
//
// Wire-format mapping lives in pkg/streaming/events/balance_deleted.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitBalanceDeletedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, b *mmodel.Balance, deletedAt time.Time) {
	if b == nil {
		return
	}

	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.BalanceDeletedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewBalanceDeleted(b.ID, b.OrganizationID, b.LedgerID, b.AccountID, deletedAt).ToEmitRequest(tenantID, deletedAt)
		})
}
