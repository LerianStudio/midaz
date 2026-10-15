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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func (uc *UseCase) UpdateBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, validate mtransaction.Responses, balances []*mmodel.Balance, balancesAfter []*mmodel.Balance) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctxProcessBalances, spanUpdateBalances := tracer.Start(ctx, "command.update_balances_new")
	defer spanUpdateBalances.End()

	newBalances := make([]*mmodel.Balance, 0, len(balances))

	if len(balancesAfter) > 0 {
		// Primary path: use Lua's AFTER state directly — no recalculation
		afterMap := make(map[string]*mmodel.Balance, len(balancesAfter))
		for _, b := range balancesAfter {
			afterMap[b.Alias] = b
		}

		for _, balance := range balances {
			after, ok := afterMap[balance.Alias]
			if !ok {
				err := fmt.Errorf("missing AFTER state for alias %s", balance.Alias)
				spanUpdateBalances.SetAttributes(
					attribute.String("balances.missing_after_alias", balance.Alias),
					attribute.String("balances.failure_reason", "missing_after_state"),
				)
				libOpentelemetry.HandleSpanError(spanUpdateBalances, "Incomplete AFTER state payload", err)
				logger.Log(ctx, libLog.LevelError, err.Error())

				return err
			}

			newBalances = append(newBalances, &mmodel.Balance{
				ID:        balance.ID,
				Alias:     balance.Alias,
				Available: after.Available,
				OnHold:    after.OnHold,
				Version:   after.Version,
			})
		}
	} else {
		// Fallback path: recalculate via OperateBalances (rolling update compatibility)
		fromTo := make(map[string]mtransaction.Amount, len(validate.From)+len(validate.To))
		for k, v := range validate.From {
			fromTo[k] = v
		}

		for k, v := range validate.To {
			fromTo[k] = v
		}

		for _, balance := range balances {
			_, spanBalance := tracer.Start(ctx, "command.update_balances_new.balance")

			txBal, tErr := balance.ToTransactionBalance()
			if tErr != nil {
				libOpentelemetry.HandleSpanError(spanUpdateBalances, "Corrupted balance for fallback path", tErr)
				logger.Log(ctx, libLog.LevelError, "Corrupted balance OverdraftLimit in fallback path", libLog.Err(tErr))
				spanBalance.End()

				return tErr
			}

			calculateBalances, err := mtransaction.OperateBalances(fromTo[balance.Alias], *txBal)
			if err != nil {
				libOpentelemetry.HandleSpanError(spanUpdateBalances, "Failed to update balances on database", err)
				logger.Log(ctx, libLog.LevelError, "Failed to update balances on database", libLog.Err(err))
				spanBalance.End()

				return err
			}

			newBalances = append(newBalances, &mmodel.Balance{
				ID:        balance.ID,
				Alias:     balance.Alias,
				Available: calculateBalances.Available,
				OnHold:    calculateBalances.OnHold,
				Version:   balance.Version + 1,
			})

			spanBalance.End()
		}
	}

	if len(newBalances) == 0 {
		return nil
	}

	if err := uc.BalanceRepo.BalancesUpdate(ctxProcessBalances, organizationID, ledgerID, newBalances); err != nil {
		recordCommandError(ctx, spanUpdateBalances, logger, "Failed to update balances on database", err)

		return err
	}

	return nil
}

// Update balance in the repository and returns the updated balance.
// Overlays Redis cached values for Available, OnHold, and Version to ensure freshest data.
//
// The method always loads the current balance first to enforce scope
// protection: internal-scope balances (e.g. overdraft companions) are
// rejected with 0175 regardless of which fields the payload carries.
//
// When update.Settings is non-nil, the use case additionally:
//  1. Validates the Settings payload (HARD GATE — fails closed, no partial writes).
//  2. Enforces overdraft transition invariants:
//     - reducing limit below current usage is rejected
//  3. Readies the system-managed "overdraft" companion before the UPDATE when
//     the payload enables the overdraft (existing companions are reused).
//
// After the UPDATE commits, the configuration events are published, then the
// committed configuration is written where transactions read it, failing the
// PATCH when that write cannot be completed (see writeBalanceConfiguration).
func (uc *UseCase) Update(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, update mmodel.UpdateBalance) (_ *mmodel.Balance, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.update_balance")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_balance", start, err)
	}()

	// Always load the current balance so the scope guard can fire
	// regardless of which fields the payload contains (Settings,
	// AllowSending, AllowReceiving, or any combination). Without this
	// unconditional Find, a payload without Settings bypasses the guard.
	current, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to fetch current balance for update", err)

		return nil, err
	}

	// Scope protection: internal-scope balances are managed exclusively by
	// the system (e.g. overdraft reserves) and cannot be updated via the
	// public API. This guard runs AFTER Find (so a non-existent balance
	// returns 404, not 422) and BEFORE any Settings validation, overdraft
	// transition enforcement, or repo Update — no partial mutations.
	if current != nil && current.Settings != nil && current.Settings.BalanceScope == mmodel.BalanceScopeInternal {
		err = pkg.ValidateBusinessError(constant.ErrUpdateOfInternalBalance, constant.EntityBalance)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rejected update on internal-scope balance", err)
		logger.Log(ctx, libLog.LevelWarn, "Rejected update on internal-scope balance",
			libLog.String("alias", current.Alias),
			libLog.String("key", current.Key))

		return nil, err
	}

	if update.Settings != nil {
		if err := validateUpdateSettings(ctx, logger, span, update.Settings); err != nil {
			return nil, err
		}

		if err := enforceOverdraftTransition(ctx, logger, span, current, update.Settings); err != nil {
			return nil, err
		}
	}

	// Ready the overdraft companion BEFORE persisting the settings, so that a
	// failure leaves the parent untouched and the payload of the
	// overdraft_enabled event exists before the UPDATE that may switch the
	// overdraft on.
	var companion *mmodel.Balance

	if update.Settings != nil {
		c, _, err := uc.ensureOverdraftBalance(ctx, logger, span, organizationID, ledgerID, current, update.Settings)
		if err != nil {
			return nil, err
		}

		companion = c
	}

	balance, enabledOverdraft, err := uc.BalanceRepo.Update(ctx, organizationID, ledgerID, balanceID, update)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to update balance on repo", err)

		return nil, err
	}

	// The events announce the persisted configuration, so they go out right
	// after the commit, before the cache write that may still fail the PATCH:
	// a cache failure must not lose them, and the client's retry commits
	// nothing new to announce. overdraft_enabled belongs to the UPDATE that
	// switched the overdraft on — only that request publishes it, even when the
	// companion existed before (a retry after a failure) — and is published
	// first, then the parent's settings_updated.
	if enabledOverdraft && companion != nil {
		uc.emitBalanceConfigChangedEvent(ctx, span, logger, companion, events.BalanceConfigChangeTypeOverdraftEnabled)
	}

	uc.emitBalanceConfigChangedEvent(ctx, span, logger, balance, events.BalanceConfigChangeTypeSettingsUpdated)

	if err := uc.writeBalanceConfiguration(ctx, span, logger, organizationID, ledgerID, balance); err != nil {
		return nil, err
	}

	uc.overlayBalanceAmountsFromCache(ctx, span, logger, organizationID, ledgerID, balance)

	return balance, nil
}

// errConfigurationCopyNotConfirmed reports a configuration copy that kept losing
// its compare-and-set to concurrent writes of the balance blob.
var errConfigurationCopyNotConfirmed = errors.New("configuration copy in the balance cache not confirmed")

// writeBalanceConfiguration makes the configuration a balance UPDATE committed
// the one transactions obey. It writes the balance's configuration key with the
// row the UPDATE returned and its version, then confirms the configuration copy
// in the balance blob, which releases reading the configuration from the blob
// obey (a compatibility bridge, removable once no rollback target reads it
// there). The copy never touches money: it is written through a compare-and-set
// on the exact bytes read, and a blob whose copy already holds this version, or a
// newer one another PATCH wrote, needs no write. Transactions never write the
// copy of a blob that exists, so on a busy balance this PATCH writes it itself.
//
// Fail-closed: PostgreSQL already committed, but answering success while the key
// or the copy still holds the previous configuration would let transactions keep
// obeying it. A Redis failure, or a copy still unconfirmed after the bounded
// compare-and-set attempts, fails the PATCH as a technical error. The client's
// retry is idempotent: the version does not rise without a real change, and the
// same version with the same content writes nothing.
func (uc *UseCase) writeBalanceConfiguration(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) error {
	config, err := balancecache.BalanceConfigFromBalance(balance)
	if err != nil {
		recordCommandError(ctx, span, logger, "Failed to read the committed balance configuration", err)

		return err
	}

	ref := balanceCacheRef(balance.Alias, balance.Key)

	if _, _, err := uc.TransactionRedisRepo.WriteBalanceConfig(ctx, organizationID, ledgerID, ref, config); err != nil {
		recordCommandError(ctx, span, logger, "Failed to write the balance configuration key", err,
			libLog.String("balance_ref", ref), libLog.Any("config_version", config.Version))

		return err
	}

	outcome, err := uc.TransactionRedisRepo.CopyBalanceConfigToBlob(ctx, organizationID, ledgerID, ref, config)
	if err != nil {
		uc.recordConfigBlobCopies(ctx, logger, configCopyFamilyBalance, nil, 1)
		recordCommandError(ctx, span, logger, "Failed to copy the balance configuration to the balance cache", err,
			libLog.String("balance_ref", ref), libLog.Any("config_version", config.Version))

		return err
	}

	uc.recordConfigBlobCopies(ctx, logger, configCopyFamilyBalance, []redis.BlobCopyOutcome{outcome}, 0)
	span.SetAttributes(attribute.String("app.response.balance_config.blob_copy", outcome.String()))

	if !outcome.Confirmed() {
		err := fmt.Errorf("balance %s version %d: %w", config.BalanceID, config.Version, errConfigurationCopyNotConfirmed)
		recordCommandError(ctx, span, logger, "Balance configuration copy not confirmed", err,
			libLog.String("balance_ref", ref), libLog.String("outcome", outcome.String()))

		return err
	}

	return nil
}

// balanceCacheRef is the alias#key a balance is cached under; a legacy row with
// an empty key is cached under the default key.
func balanceCacheRef(alias, key string) string {
	if key == "" {
		key = constant.DefaultBalanceKey
	}

	return alias + "#" + key
}

// overlayBalanceAmountsFromCache overlays amounts from the Redis cache onto a
// balance when available, to ensure the freshest values: the cached blob is the
// live transactional state the Lua atomic script mutates, and may be ahead of
// PostgreSQL while write-behind sync is pending.
//
// Best-effort: a Redis read failure or an undecodable blob leaves the
// PostgreSQL-derived values in place and is logged at Warn.
func (uc *UseCase) overlayBalanceAmountsFromCache(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) {
	internalKey := utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)

	value, rerr := uc.TransactionRedisRepo.Get(ctx, internalKey)
	if rerr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance cache value on redis", rerr)
		logger.Log(ctx, libLog.LevelWarn, "Failed to get balance cache value on Redis", libLog.Err(rerr))
	}

	if value == "" {
		return
	}

	cached := mmodel.BalanceRedis{}
	if uerr := json.Unmarshal([]byte(value), &cached); uerr != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to unmarshal balance cache value", libLog.Err(uerr))

		return
	}

	balance.Available = cached.Available
	balance.OnHold = cached.OnHold
	balance.Version = cached.Version

	if ou, perr := decimal.NewFromString(cached.OverdraftUsed); perr == nil {
		balance.OverdraftUsed = ou
	}
}

// emitBalanceConfigChangedEvent publishes the balance.config_changed
// event for a successfully persisted balance configuration mutation.
// IMPORTANT posture: build and emit failures are span-recorded and
// logged at Warn, never returned. Durability of the event is owned by
// the persisted database mutation; this helper does not make broker delivery transactional.
//
// Anchor (callers):
//   - UseCase.Update right after the UPDATE commits, with the parent
//     balance and changeType=settings_updated, and with the COMPANION
//     balance and changeType=overdraft_enabled when that UPDATE switched
//     the overdraft on.
//   - UseCase.CreateAdditionalBalance, with the companion it created.
//
// Caller invariant: b must be a persisted row — returned by
// BalanceRepo.Update (settings_updated) or the companion readied by
// ensureOverdraftBalance (overdraft_enabled) — so b.UpdatedAt reflects it.
//
// Wire-format mapping lives in pkg/streaming/events/balance_config_changed.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitBalanceConfigChangedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, b *mmodel.Balance, changeType string) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.BalanceConfigChangedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewBalanceConfigChanged(b, changeType).ToEmitRequest(tenantID, b.UpdatedAt)
		})
}
