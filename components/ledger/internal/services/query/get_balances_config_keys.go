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
// A balance in seeds was loaded from its row on a cache miss, so this load already
// holds its configuration and its account's blocked flag as PostgreSQL has them.
// Where that row is newer than the live key, the row decides and is written to the
// key under the same rule; on an equal version the key stays, as everywhere else.
// Where the account key is absent, the account row the seed's blocked hydration
// read from the primary is written to it, so the account is not read again. A
// newer row means a change committed without its key: a PATCH served by a release
// that writes no configuration key, or one whose key write failed before the
// client's retry. Deciding with the older key there would seed a blob with the
// older configuration, which a refusal never heals and an earlier release obeys
// after a rollback. The seed's balance read follows read routing, so its row may
// come from a replica; it still wins only when newer than the key, which it never
// moves back. A cached balance's blob copy is not a row and is left to the
// engine's newest-version decision; it never reads PostgreSQL.
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
func (uc *UseCase) resolveEngineConfiguration(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance, seeds engineSeeds) error {
	if len(balances) == 0 {
		return nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_engine_balances.resolve_configuration")
	defer span.End()

	targets, accountIDs, err := engineConfigurationTargets(balances, seeds.balanceIDs)
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

	seededAccounts, err := seededAccountRows(seeds.accounts)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Invalid account configuration on the seeded row", err)
		logger.Log(ctx, libLog.LevelError, "Invalid account configuration on the seeded row", libLog.Err(err))

		return err
	}

	accountConfigs, err := uc.resolveAccountConfigs(ctx, span, organizationID, ledgerID, accountIDs, keys.Accounts, seededAccounts)
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
// configuration keys are looked up by. seeded marks a balance loaded from its row,
// whose configuration fields hold that row.
type configurationTarget struct {
	balance   *mmodel.Balance
	ref       string
	balanceID uuid.UUID
	accountID uuid.UUID
	seeded    bool
}

// engineConfigurationTargets names the configuration keys of a pool: one per
// balance (alias#key) and one per distinct owning account.
func engineConfigurationTargets(balances []*mmodel.Balance, seeded map[string]struct{}) ([]configurationTarget, []uuid.UUID, error) {
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

		_, fromRow := seeded[b.ID]

		targets = append(targets, configurationTarget{balance: b, ref: ref, balanceID: balanceID, accountID: accountID, seeded: fromRow})

		if _, ok := seen[accountID]; !ok {
			seen[accountID] = struct{}{}

			accountIDs = append(accountIDs, accountID)
		}
	}

	return targets, accountIDs, nil
}

// seededAccountRows returns the configuration of each account row the seeds'
// blocked hydration read from the primary. An account without a row is not among
// them; its key, when absent, is reloaded as for a cached balance.
func seededAccountRows(accounts map[uuid.UUID]*mmodel.Account) (map[uuid.UUID]accounting.AccountConfig, error) {
	rows := make(map[uuid.UUID]accounting.AccountConfig, len(accounts))

	for accountID, acc := range accounts {
		config, err := balancecache.AccountConfigFromAccount(acc)
		if err != nil {
			return nil, err
		}

		rows[accountID] = config
	}

	return rows, nil
}

// resolveBalanceConfigs returns the configuration of each balance by balance ID. A
// balance whose row the primary no longer has under its identity is left out and
// keeps the configuration it was loaded with. A seeded balance whose row is newer
// than its live key decides with the row (see resolveEngineConfiguration).
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
	newerRows := make([]configurationTarget, 0)

	for _, target := range targets {
		if config, ok := keys[target.ref]; ok && config.Describes(target.balanceID) {
			if target.seeded && target.balance.ConfigVersion > config.Version {
				newerRows = append(newerRows, target)

				continue
			}

			resolved[target.balanceID] = config

			continue
		}

		missing[target.ref] = target
		missingRefs = append(missingRefs, target.ref)
	}

	newerRefs := make([]string, 0, len(newerRows))

	for _, target := range newerRows {
		config, err := balancecache.BalanceConfigFromBalance(target.balance)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid balance configuration on the seeded row", err)
			logger.Log(ctx, libLog.LevelError, "Invalid balance configuration on the seeded row", libLog.String("balance_id", target.balance.ID), libLog.Err(err))

			return nil, err
		}

		stored, _, err := uc.TransactionRedisRepo.WriteBalanceConfig(ctx, organizationID, ledgerID, target.ref, config)
		resolved[target.balanceID] = keptConfiguration(ctx, logger, configFamilyBalance, target.balance.ID, config, stored, err)
		newerRefs = append(newerRefs, target.ref)
	}

	reportKeysOlderThanTheRows(ctx, span, configFamilyBalance, newerRefs)

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
// account ID. An account the primary has no row for is left out. An account whose
// row a seed read is newer than its live key, or has no live key, decides with that
// row and writes it to the key (see resolveEngineConfiguration); only the accounts
// left without a key or a seeded row are read from the primary.
func (uc *UseCase) resolveAccountConfigs(
	ctx context.Context,
	span trace.Span,
	organizationID, ledgerID uuid.UUID,
	accountIDs []uuid.UUID,
	keys map[uuid.UUID]accounting.AccountConfig,
	seededRows map[uuid.UUID]accounting.AccountConfig,
) (map[uuid.UUID]accounting.AccountConfig, error) {
	logger := libObservability.NewLoggerFromContext(ctx)

	resolved := make(map[uuid.UUID]accounting.AccountConfig, len(accountIDs))
	missing := make([]uuid.UUID, 0)
	newerIDs := make([]string, 0)
	reloaded := 0

	for _, accountID := range accountIDs {
		config, live := keys[accountID]
		row, seeded := seededRows[accountID]

		switch {
		case live && (!seeded || row.Version <= config.Version):
			resolved[accountID] = config

			continue
		case live:
			newerIDs = append(newerIDs, accountID.String())
		case seeded:
			reloaded++
		default:
			missing = append(missing, accountID)

			continue
		}

		stored, _, err := uc.TransactionRedisRepo.WriteAccountConfig(ctx, organizationID, ledgerID, row)
		resolved[accountID] = keptConfiguration(ctx, logger, configFamilyAccount, accountID.String(), row, stored, err)
	}

	reportKeysOlderThanTheRows(ctx, span, configFamilyAccount, newerIDs)

	if len(missing) == 0 {
		uc.recordConfigReloaded(ctx, span, organizationID, ledgerID, configFamilyAccount, reloaded)

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

	uc.recordConfigReloaded(ctx, span, organizationID, ledgerID, configFamilyAccount, reloaded+len(accounts))

	return resolved, nil
}

// reportKeysOlderThanTheRows logs, once per load and family, the live keys a seeded
// row outranked. It is expected while an earlier release serves configuration
// PATCHes, and after a PATCH whose key write failed until the client retries.
func reportKeysOlderThanTheRows(ctx context.Context, span trace.Span, family string, ids []string) {
	if len(ids) == 0 {
		return
	}

	span.SetAttributes(attribute.Int("app.config_key."+family+".older_than_row_count", len(ids)))

	libObservability.NewLoggerFromContext(ctx).Log(ctx, libLog.LevelWarn, "Configuration key is older than the row the balance was seeded from, deciding with the row",
		libLog.String("family", family), libLog.Any("ids", ids))
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
// key was absent, which is the database cost of the keys' fixed expiry; an account
// row a seed's blocked hydration already read counts once, with no read of its own.
// Best-effort: a metric failure never affects the load.
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
