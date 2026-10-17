// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"sort"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type balanceLoader func(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error)

// GetEngineBalances returns the explicitly requested balances separately
// from the complete set of balances required for engine execution. Every balance
// carries the configuration the engine decides with and its versions, taken from
// the configuration keys or, where a key is absent, from the primary.
func (uc *UseCase) GetEngineBalances(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	explicitAliases []string,
) (explicitBalances, executionBalances []*mmodel.Balance, err error) {
	// What this load reads from PostgreSQL on a cache miss: unlike a cached
	// balance's blob copy, that configuration comes from the rows.
	seeds := newEngineSeeds()

	explicitBalances, executionBalances, err = loadEngineBalances(ctx, organizationID, ledgerID, explicitAliases, uc.seedRecordingLoader(seeds))
	if err != nil {
		return nil, nil, err
	}

	// The explicit balances are members of the execution pool, so resolving the
	// pool resolves them too.
	if err := uc.resolveEngineConfiguration(ctx, organizationID, ledgerID, executionBalances, seeds); err != nil {
		return nil, nil, fmt.Errorf("resolve engine balance configuration: %w", err)
	}

	return explicitBalances, executionBalances, nil
}

// engineSeeds records what an engine balance load read from PostgreSQL on a cache
// miss: the balances it seeded from their rows, by ID, and the account rows the
// blocked hydration read from the primary for them, by account ID.
type engineSeeds struct {
	balanceIDs map[string]struct{}
	accounts   map[uuid.UUID]*mmodel.Account
}

func newEngineSeeds() engineSeeds {
	return engineSeeds{balanceIDs: make(map[string]struct{}), accounts: make(map[uuid.UUID]*mmodel.Account)}
}

// seedRecordingLoader is GetBalances recording in seeds every balance it seeds from
// its row on a cache miss and the account rows it read for them.
func (uc *UseCase) seedRecordingLoader(seeds engineSeeds) balanceLoader {
	return func(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
		cached, fromRows, accounts, err := uc.getBalances(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			return nil, err
		}

		for _, b := range fromRows {
			seeds.balanceIDs[b.ID] = struct{}{}
		}

		for _, acc := range accounts {
			accountID, err := uuid.Parse(acc.ID)
			if err != nil {
				return nil, fmt.Errorf("record seeded account: invalid account ID %q: %w", acc.ID, err)
			}

			seeds.accounts[accountID] = acc
		}

		return append(cached, fromRows...), nil
	}
}

func loadEngineBalances(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	explicitAliases []string,
	loader balanceLoader,
) ([]*mmodel.Balance, []*mmodel.Balance, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("load engine balances: %w", err)
	}

	if organizationID == uuid.Nil || ledgerID == uuid.Nil {
		return nil, nil, fmt.Errorf("load engine balances: organization and ledger IDs must be nonzero")
	}

	if loader == nil {
		return nil, nil, fmt.Errorf("load engine balances: balance loader is required")
	}

	explicitAliases = sortedUniqueBalanceAliases(explicitAliases)
	lookupAliases := engineLookupAliases(explicitAliases)

	loadedBalances, err := loader(ctx, organizationID, ledgerID, lookupAliases)
	if err != nil {
		return nil, nil, fmt.Errorf("load engine balance pool: %w", err)
	}

	loadedByRef, err := indexEngineBalances(lookupAliases, loadedBalances)
	if err != nil {
		return nil, nil, err
	}

	explicitBalances := selectEngineBalances(explicitAliases, loadedByRef)
	explicitByRef := selectEngineBalanceIndex(explicitAliases, loadedByRef)

	companionAliases, companionAccounts, err := engineCompanionAliases(explicitByRef)
	if err != nil {
		return nil, nil, err
	}

	companions := selectEngineBalances(companionAliases, loadedByRef)
	if err := validateEngineCompanions(companions, companionAccounts); err != nil {
		return nil, nil, err
	}

	sortBalancesByReference(explicitBalances)
	executionBalances := append(append(make([]*mmodel.Balance, 0, len(explicitBalances)+len(companions)), explicitBalances...), companions...)
	sortBalancesByReference(executionBalances)

	return explicitBalances, executionBalances, nil
}

func engineLookupAliases(explicitAliases []string) []string {
	lookupAliases := append([]string(nil), explicitAliases...)
	for _, ref := range explicitAliases {
		if strings.HasSuffix(ref, mtransaction.AliasSeparatorString+constant.OverdraftBalanceKey) {
			continue
		}

		alias := ref
		if separator := strings.LastIndex(ref, mtransaction.AliasSeparatorString); separator >= 0 {
			alias = ref[:separator]
		}

		lookupAliases = append(lookupAliases,
			mtransaction.AliasKey(alias, constant.OverdraftBalanceKey))
	}

	return sortedUniqueBalanceAliases(lookupAliases)
}

func indexEngineBalances(aliases []string, balances []*mmodel.Balance) (map[string]*mmodel.Balance, error) {
	requested := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		requested[alias] = struct{}{}
	}

	indexed := make(map[string]*mmodel.Balance, len(balances))
	for _, balance := range balances {
		ref, err := balanceReference(balance)
		if err != nil {
			return nil, err
		}

		if _, ok := requested[ref]; !ok {
			return nil, fmt.Errorf("load engine balances: loader returned unrequested balance %q", ref)
		}

		if _, exists := indexed[ref]; exists {
			return nil, fmt.Errorf("load engine balances: duplicate balance %q", ref)
		}

		indexed[ref] = balance
	}

	return indexed, nil
}

func selectEngineBalances(aliases []string, indexed map[string]*mmodel.Balance) []*mmodel.Balance {
	balances := make([]*mmodel.Balance, 0, len(aliases))
	for _, alias := range aliases {
		if balance, exists := indexed[alias]; exists {
			balances = append(balances, balance)
		}
	}

	return balances
}

func selectEngineBalanceIndex(aliases []string, indexed map[string]*mmodel.Balance) map[string]*mmodel.Balance {
	selected := make(map[string]*mmodel.Balance, len(aliases))
	for _, alias := range aliases {
		if balance, exists := indexed[alias]; exists {
			selected[alias] = balance
		}
	}

	return selected
}

func engineCompanionAliases(explicit map[string]*mmodel.Balance) ([]string, map[string]string, error) {
	accounts := make(map[string]string, len(explicit))
	for ref, balance := range explicit {
		if strings.HasSuffix(ref, "#"+constant.OverdraftBalanceKey) {
			continue
		}

		companionRef := mtransaction.AliasKey(mtransaction.SplitAlias(balance.Alias), constant.OverdraftBalanceKey)
		if companion, exists := explicit[companionRef]; exists {
			if companion.AccountID != balance.AccountID {
				return nil, nil, fmt.Errorf("load engine balances: explicit companion %q has inconsistent account identity", companionRef)
			}

			continue
		}

		if accountID, exists := accounts[companionRef]; exists && accountID != balance.AccountID {
			return nil, nil, fmt.Errorf("load engine balances: companion %q has ambiguous account identity", companionRef)
		}

		accounts[companionRef] = balance.AccountID
	}

	aliases := make([]string, 0, len(accounts))
	for alias := range accounts {
		aliases = append(aliases, alias)
	}

	sort.Strings(aliases)

	return aliases, accounts, nil
}

func validateEngineCompanions(companions []*mmodel.Balance, accounts map[string]string) error {
	seen := make(map[string]struct{}, len(companions))
	for _, balance := range companions {
		ref, err := balanceReference(balance)
		if err != nil {
			return err
		}

		expectedAccountID, ok := accounts[ref]
		if !ok {
			return fmt.Errorf("load engine balances: companion loader returned unrequested balance %q", ref)
		}

		if balance.AccountID != expectedAccountID {
			return fmt.Errorf("load engine balances: companion %q has inconsistent account identity", ref)
		}

		if _, exists := seen[ref]; exists {
			return fmt.Errorf("load engine balances: duplicate companion balance %q", ref)
		}

		seen[ref] = struct{}{}
	}

	return nil
}

func balanceReference(balance *mmodel.Balance) (string, error) {
	if balance == nil {
		return "", fmt.Errorf("load engine balances: nil balance")
	}

	key := strings.TrimSpace(balance.Key)
	if key == "" {
		key = constant.DefaultBalanceKey
	}

	return mtransaction.AliasKey(mtransaction.SplitAlias(balance.Alias), key), nil
}

func sortedUniqueBalanceAliases(aliases []string) []string {
	unique := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		unique[alias] = struct{}{}
	}

	result := make([]string, 0, len(unique))
	for alias := range unique {
		result = append(result, alias)
	}

	sort.Strings(result)

	return result
}

func sortBalancesByReference(balances []*mmodel.Balance) {
	sort.Slice(balances, func(i, j int) bool {
		left, _ := balanceReference(balances[i])
		right, _ := balanceReference(balances[j])

		return left < right
	})
}

// GetBalances retrieves balances for the given aliases using a cache-aside
// pattern: checks Redis first, falls back to PostgreSQL for cache misses.
// This is a pure read -- it does not mutate balances or execute any Lua scripts.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	cached, fromRows, _, err := uc.getBalances(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		return nil, err
	}

	return append(cached, fromRows...), nil
}

// getBalances is GetBalances returning the balances found in the cache apart from
// the ones seeded from their PostgreSQL rows on a cache miss, in that order, and the
// account rows the blocked hydration of those seeds read from the primary.
func (uc *UseCase) getBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []*mmodel.Balance, []*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balances")
	defer span.End()

	cached, uncachedAliases := uc.getBalancesFromCache(ctx, organizationID, ledgerID, aliases)

	var (
		fromRows []*mmodel.Balance
		accounts []*mmodel.Account
	)

	if len(uncachedAliases) > 0 {
		// The first read only turns aliases into account identifiers: an alias names
		// no account until a row says so, so nothing can be admitted before it. Its
		// rows are discarded — they were read outside the admission and may already
		// describe a closed account.
		resolved, err := uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, uncachedAliases)
		if err != nil {
			recordQueryError(ctx, span, logger, "Failed to get balances from database", err)

			return nil, nil, nil, err
		}

		// The seed admission covers the seed read, the blocked hydration and the
		// rebuild. Releasing it earlier would let a closing evict between the seed and
		// the rebuild and hand the engine a balance of an account it already finished
		// closing. It is shared, so concurrent loads of the same account never refuse
		// each other.
		//
		// The seed itself is admitted later, inside the engine, so a caller that goes
		// on to execute accounting installs a sink and takes every admission over: it
		// then ends with the execution's answer instead of with this load.
		var owned []*accountprotection.Admission

		defer func() {
			for i := len(owned) - 1; i >= 0; i-- {
				owned[i].Release(ctx)
			}
		}()

		hold := func(admission *accountprotection.Admission) {
			if !accountprotection.AdoptAdmission(ctx, admission) {
				owned = append(owned, admission)
			}
		}

		admission, err := uc.protectBalanceSeedAdmission(ctx, span, organizationID, ledgerID, resolved)
		if err != nil {
			return nil, nil, nil, err
		}

		hold(admission)

		balancesDB, err := uc.readSeedsUnderAdmission(ctx, span, organizationID, ledgerID, uncachedAliases, admission, hold)
		if err != nil {
			return nil, nil, nil, err
		}

		accounts, err = uc.hydrateAccountBlocked(ctx, organizationID, ledgerID, balancesDB)
		if err != nil {
			// logged once by hydrateAccountBlocked; record on this span only
			spanattr.HandleSpanByErrorClass(span, "Failed to hydrate account blocked state", err)

			return nil, nil, nil, err
		}

		if err := uc.rebuildStaleBalanceSeeds(ctx, span, organizationID, ledgerID, balancesDB); err != nil {
			return nil, nil, nil, err
		}

		fromRows = balancesDB
	}

	return cached, fromRows, accounts, nil
}

// rebuildStaleBalanceSeeds keeps a cache-miss seed from feeding the engine a balance
// that the operation trail has already moved past. Public balance reads use it on a
// cache miss too, so they return the state posting would use.
//
// The balance row is written asynchronously by the sync worker, so losing the cached
// balance while a delta is still pending leaves PostgreSQL behind the trail. Seeding
// from that row restarts the balance from an older state, and every later mutation
// forks off it — money disappears with no error anywhere. The operation at the
// high-water mark records the state it left behind, so when the row is behind it, that
// operation is the truth and the row is not.
//
// Only the monetary fields are taken from the operation; identity, flags and settings
// stay as the row has them. A row at or ahead of the trail is left untouched: a
// completion or recovery that landed after the last operation is not a fork.
//
// Residual window: the trail is written after the engine runs, by the completer or by
// recovery. If the process stalls between the two and the cached balance is evicted in
// that same instant, the mark is one version behind the live state and the seed is
// rebuilt one version short. That window is narrow, it heals when recovery persists the
// operation, and it leaves the seed no worse than the pre-guard behavior, which used the
// stale row unconditionally.
func (uc *UseCase) rebuildStaleBalanceSeeds(
	ctx context.Context,
	span trace.Span,
	organizationID, ledgerID uuid.UUID,
	balances []*mmodel.Balance,
) error {
	if len(balances) == 0 {
		return nil
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	refs := make([]operation.BalanceHWMRef, 0, len(balances))

	for _, b := range balances {
		accountID, err := uuid.Parse(b.AccountID)
		if err != nil {
			recordQueryError(ctx, span, logger, "Invalid account ID on balance", err, libLog.String("balance_id", b.ID))

			return err
		}

		balanceID, err := uuid.Parse(b.ID)
		if err != nil {
			recordQueryError(ctx, span, logger, "Invalid balance ID on balance", err, libLog.String("balance_id", b.ID))

			return err
		}

		refs = append(refs, operation.BalanceHWMRef{AccountID: accountID, BalanceID: balanceID})
	}

	highWaterMarks, err := uc.OperationRepo.ListLatestByBalances(ctx, organizationID, ledgerID, refs)
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to load balance high-water marks", err)

		return err
	}

	rebuilt := 0

	for _, b := range balances {
		hwm := highWaterMarks[b.ID]
		if hwm == nil {
			continue
		}

		// A mark with no after-version is a row behind a trail that cannot be rebuilt
		// from, which applyBalanceHighWaterMark refuses (0513) — deliberately, and not
		// the same case as having no mark at all, which passes through. The schema keeps
		// the column NOT NULL, so this is a guard against a future shape, not a live one:
		// do not turn it into a passthrough.
		if hwm.BalanceAfter.Version != nil && *hwm.BalanceAfter.Version <= b.Version {
			continue
		}

		// Read before the rebuild overwrites it: how far behind the row was is the
		// whole point of the warning.
		rowVersion := b.Version

		if err := applyBalanceHighWaterMark(b, hwm); err != nil {
			recordQueryError(ctx, span, logger, "Failed to rebuild stale balance seed", err, libLog.String("balance_id", b.ID))

			return err
		}

		logger.Log(ctx, libLog.LevelWarn, "Rebuilt stale balance seed from the operation trail",
			libLog.String("balance_id", b.ID),
			libLog.Int("row_version", int(rowVersion)),
			libLog.Int("hwm_version", int(*hwm.BalanceAfter.Version)))

		rebuilt++
	}

	if rebuilt > 0 {
		span.SetAttributes(attribute.Int("app.balance_seed.rebuilt_count", rebuilt))
		uc.recordBalanceSeedRebuilt(ctx, organizationID, ledgerID, rebuilt)
	}

	return nil
}

// applyBalanceHighWaterMark overwrites the balance's monetary state with the state the
// high-water-mark operation left behind. Anything the operation cannot supply makes the
// whole rebuild fail: serving the stale row is the failure this guard exists to prevent,
// so a request that cannot be answered correctly is refused instead.
func applyBalanceHighWaterMark(balance *mmodel.Balance, hwm *operation.Operation) error {
	if hwm.BalanceAfter.Available == nil || hwm.BalanceAfter.OnHold == nil || hwm.BalanceAfter.Version == nil {
		return pkg.ValidateBusinessError(constant.ErrBalanceSeedRebuildInconsistent, constant.EntityBalance)
	}

	balance.Available = *hwm.BalanceAfter.Available
	balance.OnHold = *hwm.BalanceAfter.OnHold
	balance.Version = *hwm.BalanceAfter.Version

	// The overdraft companion's operations carry the DEFAULT balance's overdraft
	// snapshot, mirrored onto them at write time, so that value describes another
	// balance. The companion keeps what its own row holds — and never has to read the
	// snapshot, so an unreadable one cannot fail a rebuild that would ignore it.
	if balance.Key != constant.OverdraftBalanceKey {
		overdraftUsed, err := decimal.NewFromString(hwm.Snapshot.OverdraftUsedAfter)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrBalanceSeedRebuildInconsistent, constant.EntityBalance)
		}

		balance.OverdraftUsed = overdraftUsed
	}

	return nil
}

// recordBalanceSeedRebuilt counts rebuilt seeds for the scope. Best-effort: a metric
// failure never affects the read.
func (uc *UseCase) recordBalanceSeedRebuilt(ctx context.Context, organizationID, ledgerID uuid.UUID, rebuilt int) {
	if uc.MetricsFactory == nil {
		return
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	counter, err := uc.MetricsFactory.Counter(utils.BalanceSeedRebuilt)
	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to create balance seed rebuild counter", libLog.Err(err))

		return
	}

	if addErr := counter.WithLabels(map[string]string{
		"organization_id": organizationID.String(),
		"ledger_id":       ledgerID.String(),
		"tenant_id":       tmcore.GetTenantIDContext(ctx),
	}).Add(ctx, int64(rebuilt)); addErr != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit balance seed rebuild counter", libLog.Err(addErr))
	}
}

// hydrateAccountBlocked stamps database-loaded balances with their owning
// account's blocked flag using ONE batched primary-key lookup over the
// distinct account IDs, and returns the account rows it read. Only the
// cache-miss path reaches here: on a hit the flag is already in the cached
// blob. Failing the lookup fails the read — the block state must never be
// silently assumed open. A balance whose account row is absent (e.g.
// soft-deleted) resolves to not blocked.
func (uc *UseCase) hydrateAccountBlocked(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) ([]*mmodel.Account, error) {
	if len(balances) == 0 {
		return nil, nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balances.hydrate_account_blocked")
	defer span.End()

	seen := make(map[uuid.UUID]struct{}, len(balances))
	accountIDs := make([]uuid.UUID, 0, len(balances))

	for _, b := range balances {
		accountID, err := uuid.Parse(b.AccountID)
		if err != nil {
			recordQueryError(ctx, span, logger, "Invalid account ID on balance", err, libLog.String("balance_id", b.ID))

			return nil, err
		}

		if _, ok := seen[accountID]; !ok {
			seen[accountID] = struct{}{}

			accountIDs = append(accountIDs, accountID)
		}
	}

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to list accounts for blocked hydration", err)

		return nil, err
	}

	byAccount := make(map[string]*mmodel.Account, len(accounts))
	for _, acc := range accounts {
		byAccount[acc.ID] = acc
	}

	// The flag carries the version it was read at, as a cached balance carries the
	// version of its blob's copy, so the engine configuration compares like with like.
	for _, b := range balances {
		b.Blocked, b.AccountConfigVersion = false, 0

		if acc, ok := byAccount[b.AccountID]; ok {
			b.Blocked = acc.Blocked != nil && *acc.Blocked
			b.AccountConfigVersion = acc.ConfigVersion
		}
	}

	return accounts, nil
}

// getBalancesFromCache checks Redis for cached balances. Returns two slices:
// the balances found in cache, and the aliases that were not found (cache misses)
// which need to be fetched from the database.
//
// The cache read opens no span of its own: each lookup is one Redis command the
// driver already traces, so a deserialization failure is recorded on the caller's
// query.get_balances span.
func (uc *UseCase) getBalancesFromCache(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)

	span := trace.SpanFromContext(ctx)

	cached := make([]*mmodel.Balance, 0, len(aliases))
	misses := make([]string, 0, len(aliases))

	for _, alias := range aliases {
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, alias)

		value, err := uc.TransactionRedisRepo.Get(ctx, internalKey)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to read balance from cache, falling back to database", libLog.String("alias", alias), libLog.Err(err))

			misses = append(misses, alias)

			continue
		}

		if libCommons.IsNilOrEmpty(&value) {
			misses = append(misses, alias)

			continue
		}

		snapshot, err := balancecache.DecodeForRead([]byte(value))
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to deserialize cached balance", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to deserialize cached balance, falling back to database", libLog.String("alias", alias), libLog.Err(err))

			misses = append(misses, alias)

			continue
		}

		balanceAlias, balanceKey, _ := strings.Cut(alias, "#")

		balanceKey = strings.TrimSpace(balanceKey)
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		if snapshot.Key != balanceKey || (snapshot.Alias != "" && snapshot.Alias != balanceAlias) {
			logger.Log(ctx, libLog.LevelWarn, "Cached balance identity does not match requested balance, falling back to database", libLog.String("alias", alias))

			misses = append(misses, alias)

			continue
		}

		// Synthesize Settings only when at least one field diverges from the
		// defaults. This preserves nil Settings for legacy balances that never
		// had custom configuration. Mirrors the materialization logic used by
		// consumer.redis.go:balanceRedisToBalance so transaction flows observe
		// the same overdraft configuration whether the balance is read from
		// the cache path (here) or returned from the Lua script.
		var settings *mmodel.BalanceSettings
		if snapshot.AllowOverdraft || snapshot.OverdraftLimitEnabled ||
			(snapshot.BalanceScope != "" && snapshot.BalanceScope != mmodel.BalanceScopeTransactional) ||
			!snapshot.OverdraftLimit.IsZero() {
			settings = &mmodel.BalanceSettings{
				BalanceScope:          snapshot.BalanceScope,
				AllowOverdraft:        snapshot.AllowOverdraft,
				OverdraftLimitEnabled: snapshot.OverdraftLimitEnabled,
			}
			// Only expose OverdraftLimit when the limit is actively enforced.
			// BalanceSettings.Validate() requires OverdraftLimit to be nil
			// whenever OverdraftLimitEnabled is false.
			if snapshot.OverdraftLimitEnabled {
				limit := snapshot.OverdraftLimit.String()
				settings.OverdraftLimit = &limit
			}
		}

		cached = append(cached, &mmodel.Balance{
			ID:             snapshot.ID.String(),
			AccountID:      snapshot.AccountID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          balanceAlias,
			Key:            balanceKey,
			Available:      snapshot.Available,
			OnHold:         snapshot.OnHold,
			Version:        snapshot.Version,
			AccountType:    snapshot.AccountType,
			AllowSending:   snapshot.AllowSending,
			AllowReceiving: snapshot.AllowReceiving,
			Blocked:        snapshot.Blocked,
			AssetCode:      snapshot.AssetCode,
			Direction:      snapshot.Direction,
			OverdraftUsed:  snapshot.OverdraftUsed,
			Settings:       settings,
			// The versions of the blob's configuration copies, so the engine load can
			// tell a copy that diverges under the version it decides with.
			ConfigVersion:        snapshot.ConfigVersion,
			AccountConfigVersion: snapshot.AccountConfigVersion,
		})
	}

	logger.Log(ctx, libLog.LevelDebug, "Balance cache lookup complete",
		libLog.Int("cached", len(cached)),
		libLog.Int("misses", len(misses)))

	return cached, misses
}
