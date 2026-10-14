// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	configFamilyBalance = "balance"
	configFamilyAccount = "account"
)

// resolveEngineConfiguration stamps every balance of an engine pool with the
// configuration the engine decides with and its versions: the balance family
// (sending, receiving, overdraft, scope) and the owning account's blocked flag.
//
// The configuration keys are the source while they live. One batched read covers
// the whole pool, so a hot balance whose keys are present reads no database row. A
// key that is missing, unreadable or describes another balance (one deleted and
// recreated under the same alias#key) is reloaded from the PostgreSQL primary,
// never a replica that could seed a stale value for the key's whole life, and
// written back under the version rule: a key a concurrent update already moved to
// a newer version wins and is what the balance carries.
//
// Absence is the normal state after a deploy and on every expiry, so it is never
// an error. Neither is a failed key write: the primary row is the truth, the
// engine takes the request's configuration when the key is missing, and the next
// load writes the key again. Only a failed primary read fails the load, as the
// configuration cannot be assumed.
//
// The keys hold configuration only, never money, so this write needs no seed
// admission: no closing, balance creation or deletion removes a configuration
// key or relies on its absence, and a key of a deleted balance is inert for any
// other balance because it carries its balance's identity.
func (uc *UseCase) resolveEngineConfiguration(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	if len(balances) == 0 {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_engine_balances.resolve_configuration")
	defer span.End()

	targets, accountIDs, err := engineConfigurationTargets(balances)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Invalid balance identity in the engine pool", err)
		logger.Log(ctx, libLog.LevelError, "Invalid balance identity in the engine pool", libLog.Err(err))

		return err
	}

	refs := make([]string, len(targets))
	for i, target := range targets {
		refs[i] = target.ref
	}

	keys, err := uc.TransactionRedisRepo.ReadConfigKeys(ctx, organizationID, ledgerID, refs, accountIDs)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Failed to read configuration keys, reloading the configuration from the database", libLog.Err(err))

		keys = redis.ConfigKeyValues{}
	}

	balanceConfigs, err := uc.resolveBalanceConfigs(ctx, span, organizationID, ledgerID, targets, keys.Balances)
	if err != nil {
		return err
	}

	accountConfigs, err := uc.resolveAccountConfigs(ctx, span, organizationID, ledgerID, accountIDs, keys.Accounts)
	if err != nil {
		return err
	}

	var divergent configCopyDivergence

	for _, target := range targets {
		if config, ok := balanceConfigs[target.balanceID]; ok {
			if balanceCopyDiverges(target.balance, config) {
				divergent.balanceRefs = append(divergent.balanceRefs, target.ref)
			}

			applyBalanceConfig(target.balance, config)
		}

		// An account without a row (soft-deleted) is not blocked, as the blocked
		// hydration of a seed has always resolved it.
		account, found := accountConfigs[target.accountID]
		if found && accountCopyDiverges(target.balance, account) {
			divergent.addAccount(target.accountID)
		}

		target.balance.Blocked = account.Blocked
		target.balance.AccountConfigVersion = account.Version
	}

	uc.reportConfigCopyDivergence(ctx, span, divergent)

	return nil
}

// configCopyDivergence collects the balances and accounts of one load whose loaded
// copy diverges from the configuration the engine decides with (see
// balanceCopyDiverges).
type configCopyDivergence struct {
	balanceRefs []string
	accountIDs  []string
	accounts    map[uuid.UUID]struct{}
}

func (d *configCopyDivergence) addAccount(accountID uuid.UUID) {
	if _, seen := d.accounts[accountID]; seen {
		return
	}

	if d.accounts == nil {
		d.accounts = make(map[uuid.UUID]struct{})
	}

	d.accounts[accountID] = struct{}{}
	d.accountIDs = append(d.accountIDs, accountID.String())
}

// balanceCopyDiverges reports whether the configuration a balance was loaded with —
// the copy in its blob, or its row on a cache miss — carries the version the engine
// decides with but other content. A version names one committed row state, so that
// is an inconsistency: a release that reads the configuration from the blob changed
// the copy without a version it does not know, a copy diverged before versions
// existed, or versions moved back. The engine decides with the key and keeps the
// blob's copy; nothing in the execution reports it, so the load does.
func balanceCopyDiverges(loaded *mmodel.Balance, resolved accounting.BalanceConfig) bool {
	if loaded.ConfigVersion != resolved.Version {
		return false
	}

	copied, err := balancecache.BalanceConfigFromBalance(loaded)
	if err != nil {
		return false
	}

	// A disabled limit is inert, and a balance read from the cache does not carry
	// it, so it is compared only when enabled.
	if !copied.OverdraftLimitEnabled && !resolved.OverdraftLimitEnabled {
		copied.OverdraftLimit = resolved.OverdraftLimit
	}

	return !copied.Equal(resolved)
}

// accountCopyDiverges is balanceCopyDiverges for the account's blocked flag.
func accountCopyDiverges(loaded *mmodel.Balance, resolved accounting.AccountConfig) bool {
	return loaded.AccountConfigVersion == resolved.Version && loaded.Blocked != resolved.Blocked
}

// reportConfigCopyDivergence logs the divergent copies of one load at Warn, with
// the balances and accounts they belong to, and counts them by family. The engine
// still decides, so this is a signal, never a failure; a metric failure never
// affects the load.
func (uc *UseCase) reportConfigCopyDivergence(ctx context.Context, span trace.Span, divergent configCopyDivergence) {
	if len(divergent.balanceRefs) == 0 && len(divergent.accountIDs) == 0 {
		return
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	span.SetAttributes(
		attribute.Int("app.config_copy.balance.divergent_count", len(divergent.balanceRefs)),
		attribute.Int("app.config_copy.account.divergent_count", len(divergent.accountIDs)),
	)

	logger.Log(ctx, libLog.LevelWarn, "Configuration copy holds other content under the version the engine decides with",
		libLog.Any("balance_refs", divergent.balanceRefs), libLog.Any("account_ids", divergent.accountIDs))

	if uc.MetricsFactory == nil {
		return
	}

	counter, err := uc.MetricsFactory.Counter(utils.ConfigCopyDivergent)
	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to create configuration copy divergence counter", libLog.Err(err))

		return
	}

	for family, count := range map[string]int{configFamilyBalance: len(divergent.balanceRefs), configFamilyAccount: len(divergent.accountIDs)} {
		if count == 0 {
			continue
		}

		if addErr := counter.WithLabels(map[string]string{"family": family}).Add(ctx, int64(count)); addErr != nil {
			logger.Log(ctx, libLog.LevelDebug, "Failed to emit configuration copy divergence counter", libLog.Err(addErr))
		}
	}
}

// configurationTarget is one balance of the pool with the identities its
// configuration keys are looked up by.
type configurationTarget struct {
	balance   *mmodel.Balance
	ref       string
	balanceID uuid.UUID
	accountID uuid.UUID
}

// engineConfigurationTargets names the configuration keys of a pool: one per
// balance (alias#key) and one per distinct owning account.
func engineConfigurationTargets(balances []*mmodel.Balance) ([]configurationTarget, []uuid.UUID, error) {
	targets := make([]configurationTarget, 0, len(balances))
	accountIDs := make([]uuid.UUID, 0, len(balances))
	seen := make(map[uuid.UUID]struct{}, len(balances))

	for _, b := range balances {
		ref, err := balanceReference(b)
		if err != nil {
			return nil, nil, err
		}

		balanceID, err := uuid.Parse(b.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve engine configuration: invalid balance ID %q: %w", b.ID, err)
		}

		accountID, err := uuid.Parse(b.AccountID)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve engine configuration: invalid account ID %q: %w", b.AccountID, err)
		}

		targets = append(targets, configurationTarget{balance: b, ref: ref, balanceID: balanceID, accountID: accountID})

		if _, ok := seen[accountID]; !ok {
			seen[accountID] = struct{}{}

			accountIDs = append(accountIDs, accountID)
		}
	}

	return targets, accountIDs, nil
}

// resolveBalanceConfigs returns the configuration of each balance by balance ID. A
// balance whose row the primary no longer has under its identity is left out and
// keeps the configuration it was loaded with.
func (uc *UseCase) resolveBalanceConfigs(
	ctx context.Context,
	span trace.Span,
	organizationID, ledgerID uuid.UUID,
	targets []configurationTarget,
	keys map[string]accounting.BalanceConfig,
) (map[uuid.UUID]accounting.BalanceConfig, error) {
	logger := libObservability.NewLoggerFromContext(ctx)

	resolved := make(map[uuid.UUID]accounting.BalanceConfig, len(targets))
	missing := make(map[string]configurationTarget)
	missingRefs := make([]string, 0)

	for _, target := range targets {
		if config, ok := keys[target.ref]; ok && config.Describes(target.balanceID) {
			resolved[target.balanceID] = config

			continue
		}

		missing[target.ref] = target
		missingRefs = append(missingRefs, target.ref)
	}

	if len(missingRefs) == 0 {
		return resolved, nil
	}

	rows, err := uc.BalanceRepo.ListByAliasesWithKeysOnPrimary(ctx, organizationID, ledgerID, missingRefs)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to reload balance configuration from the primary", err)
		logger.Log(ctx, libLog.LevelError, "Failed to reload balance configuration from the primary", libLog.Err(err))

		return nil, err
	}

	reloaded := 0

	for _, row := range rows {
		ref, err := balanceReference(row)
		if err != nil {
			return nil, err
		}

		target, ok := missing[ref]
		if !ok || row.ID != target.balance.ID {
			continue
		}

		config, err := balancecache.BalanceConfigFromBalance(row)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid balance configuration on the primary row", err)
			logger.Log(ctx, libLog.LevelError, "Invalid balance configuration on the primary row", libLog.String("balance_id", row.ID), libLog.Err(err))

			return nil, err
		}

		stored, _, err := uc.TransactionRedisRepo.WriteBalanceConfig(ctx, organizationID, ledgerID, ref, config)
		resolved[target.balanceID] = keptConfiguration(ctx, logger, configFamilyBalance, row.ID, config, stored, err)
		reloaded++
	}

	if unresolved := len(missingRefs) - reloaded; unresolved > 0 {
		logger.Log(ctx, libLog.LevelWarn, "Balance has no configuration row on the primary under its identity, keeping the loaded configuration",
			libLog.Int("balances", unresolved))
	}

	uc.recordConfigReloaded(ctx, span, organizationID, ledgerID, configFamilyBalance, reloaded)

	return resolved, nil
}

// resolveAccountConfigs returns the blocked configuration of each account by
// account ID. An account the primary has no row for is left out.
func (uc *UseCase) resolveAccountConfigs(
	ctx context.Context,
	span trace.Span,
	organizationID, ledgerID uuid.UUID,
	accountIDs []uuid.UUID,
	keys map[uuid.UUID]accounting.AccountConfig,
) (map[uuid.UUID]accounting.AccountConfig, error) {
	logger := libObservability.NewLoggerFromContext(ctx)

	resolved := make(map[uuid.UUID]accounting.AccountConfig, len(accountIDs))
	missing := make([]uuid.UUID, 0)

	for _, accountID := range accountIDs {
		if config, ok := keys[accountID]; ok {
			resolved[accountID] = config

			continue
		}

		missing = append(missing, accountID)
	}

	if len(missing) == 0 {
		return resolved, nil
	}

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, missing)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to reload account configuration from the primary", err)
		logger.Log(ctx, libLog.LevelError, "Failed to reload account configuration from the primary", libLog.Err(err))

		return nil, err
	}

	for _, acc := range accounts {
		config, err := balancecache.AccountConfigFromAccount(acc)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid account configuration on the primary row", err)
			logger.Log(ctx, libLog.LevelError, "Invalid account configuration on the primary row", libLog.String("account_id", acc.ID), libLog.Err(err))

			return nil, err
		}

		stored, _, err := uc.TransactionRedisRepo.WriteAccountConfig(ctx, organizationID, ledgerID, config)
		resolved[config.AccountID] = keptConfiguration(ctx, logger, configFamilyAccount, acc.ID, config, stored, err)
	}

	uc.recordConfigReloaded(ctx, span, organizationID, ledgerID, configFamilyAccount, len(accounts))

	return resolved, nil
}

// keptConfiguration picks what a balance carries after a reload wrote its key:
// what the key holds afterwards (the reloaded row, or a newer version of the same
// row a concurrent update stored first), or the reloaded row itself when the write
// failed. A write failure never fails the load. A key holding other content under
// the same version is an inconsistency worth an error log; the primary row still
// decides.
func keptConfiguration[C any](ctx context.Context, logger libLog.Logger, family, id string, reloaded, stored C, writeErr error) C {
	switch {
	case writeErr == nil:
		return stored
	case errors.Is(writeErr, redis.ErrConfigVersionConflict):
		logger.Log(ctx, libLog.LevelError, "Configuration key holds other content under the reloaded version, using the primary row",
			libLog.String("family", family), libLog.String("id", id), libLog.Err(writeErr))
	default:
		logger.Log(ctx, libLog.LevelWarn, "Failed to write the reloaded configuration key, using the primary row",
			libLog.String("family", family), libLog.String("id", id), libLog.Err(writeErr))
	}

	return reloaded
}

// applyBalanceConfig sets a balance's configuration fields to the given
// configuration, so that BalanceConfigFromBalance of the balance returns it again.
// Settings stay nil when every value is the default, as a legacy row has them.
func applyBalanceConfig(b *mmodel.Balance, config accounting.BalanceConfig) {
	b.AllowSending = config.AllowSending
	b.AllowReceiving = config.AllowReceiving
	b.ConfigVersion = config.Version
	b.Settings = nil

	if !config.AllowOverdraft && !config.OverdraftLimitEnabled && config.OverdraftLimit.IsZero() &&
		config.BalanceScope == mmodel.BalanceScopeTransactional {
		return
	}

	b.Settings = &mmodel.BalanceSettings{
		BalanceScope:          config.BalanceScope,
		AllowOverdraft:        config.AllowOverdraft,
		OverdraftLimitEnabled: config.OverdraftLimitEnabled,
	}

	if config.OverdraftLimitEnabled || !config.OverdraftLimit.IsZero() {
		limit := config.OverdraftLimit.String()
		b.Settings.OverdraftLimit = &limit
	}
}

// recordConfigReloaded counts configurations reloaded from the primary because their
// key was absent, which is the database cost of the keys' fixed expiry. Best-effort:
// a metric failure never affects the load.
func (uc *UseCase) recordConfigReloaded(ctx context.Context, span trace.Span, organizationID, ledgerID uuid.UUID, family string, reloaded int) {
	if reloaded == 0 {
		return
	}

	span.SetAttributes(attribute.Int("app.config_key."+family+".reloaded_count", reloaded))

	if uc.MetricsFactory == nil {
		return
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	counter, err := uc.MetricsFactory.Counter(utils.ConfigKeyReloaded)
	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to create configuration reload counter", libLog.Err(err))

		return
	}

	if addErr := counter.WithLabels(map[string]string{
		"organization_id": organizationID.String(),
		"ledger_id":       ledgerID.String(),
		"tenant_id":       tmcore.GetTenantIDContext(ctx),
		"family":          family,
	}).Add(ctx, int64(reloaded)); addErr != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit configuration reload counter", libLog.Err(addErr))
	}
}
