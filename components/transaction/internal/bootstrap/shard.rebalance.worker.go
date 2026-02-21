// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	"github.com/google/uuid"
)

type ShardRebalanceWorker struct {
	logger libLog.Logger

	manager shardRebalanceManager
	router  *shard.Router

	interval           time.Duration
	loadWindow         time.Duration
	imbalanceThreshold float64
	candidateLimit     int
	isolationShare     float64
	isolationMinLoad   int64
}

type shardRebalanceManager interface {
	IsRebalancerPaused(ctx context.Context) (bool, error)
	GetShardLoads(ctx context.Context, shardCount int, window time.Duration) ([]internalsharding.ShardLoad, error)
	TopHotAccounts(ctx context.Context, shardID int, window time.Duration, limit int) ([]internalsharding.HotAccount, error)
	GetShardIsolationCounts(ctx context.Context, shardCount int) (map[int]int64, error)
	TryAcquireRebalancePermits(ctx context.Context, sourceShard, targetShard int, account internalsharding.HotAccount) (bool, error)
	MigrateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string, targetShard int, knownBalanceKeys []string) (*internalsharding.MigrationResult, error)
	MarkAccountIsolated(ctx context.Context, account internalsharding.HotAccount, shardID int) error
}

func NewShardRebalanceWorker(
	logger libLog.Logger,
	manager shardRebalanceManager,
	router *shard.Router,
	interval time.Duration,
	loadWindow time.Duration,
	imbalanceThreshold float64,
	candidateLimit int,
	isolationShare float64,
	isolationMinLoad int64,
) *ShardRebalanceWorker {
	if manager == nil || router == nil {
		return nil
	}

	if interval <= 0 {
		interval = 5 * time.Second
	}

	if loadWindow <= 0 {
		loadWindow = 60 * time.Second
	}

	if imbalanceThreshold <= 1.0 {
		imbalanceThreshold = 1.5
	}

	if candidateLimit <= 0 {
		candidateLimit = 8
	}

	if isolationShare <= 0 || isolationShare > 1 {
		isolationShare = 0.7
	}

	if isolationMinLoad < 0 {
		isolationMinLoad = 0
	}

	return &ShardRebalanceWorker{
		logger:             logger,
		manager:            manager,
		router:             router,
		interval:           interval,
		loadWindow:         loadWindow,
		imbalanceThreshold: imbalanceThreshold,
		candidateLimit:     candidateLimit,
		isolationShare:     isolationShare,
		isolationMinLoad:   isolationMinLoad,
	}
}

func (w *ShardRebalanceWorker) Run(_ *libCommons.Launcher) error {
	if w == nil || w.manager == nil || w.router == nil {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Info("ShardRebalanceWorker started")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("ShardRebalanceWorker: shutting down")

			return nil
		case <-ticker.C:
			if err := w.rebalanceOnce(ctx); err != nil {
				w.logger.Warnf("ShardRebalanceWorker: rebalance cycle failed: %v", err)
			}
		}
	}
}

func (w *ShardRebalanceWorker) rebalanceOnce(ctx context.Context) error {
	paused, err := w.manager.IsRebalancerPaused(ctx)
	if err != nil {
		return err
	}

	if paused {
		w.logger.Info("ShardRebalanceWorker: paused")

		return nil
	}

	loads, err := w.manager.GetShardLoads(ctx, w.router.ShardCount(), w.loadWindow)
	if err != nil {
		return err
	}

	maxLoad, ok := w.detectImbalance(loads)
	if !ok {
		return nil
	}

	hotAccounts, err := w.manager.TopHotAccounts(ctx, maxLoad.ShardID, w.loadWindow, w.candidateLimit)
	if err != nil {
		return err
	}

	if len(hotAccounts) == 0 {
		return nil
	}

	loadByShard := make(map[int]int64, len(loads))
	for _, load := range loads {
		loadByShard[load.ShardID] = load.Load
	}

	isolationCounts, err := w.manager.GetShardIsolationCounts(ctx, w.router.ShardCount())
	if err != nil {
		return err
	}

	for _, hot := range hotAccounts {
		if hot.Alias == "" || hot.Load <= 0 || shard.IsExternal(hot.Alias) {
			continue
		}

		done, migrateErr := w.tryMigrateHotAccount(ctx, hot, maxLoad, loadByShard, isolationCounts)
		if migrateErr != nil {
			return migrateErr
		}

		if done {
			return nil
		}
	}

	return nil
}

// detectImbalance checks whether the load distribution is imbalanced enough
// to warrant rebalancing. Returns the heaviest shard and true if rebalancing
// should proceed.
func (w *ShardRebalanceWorker) detectImbalance(loads []internalsharding.ShardLoad) (internalsharding.ShardLoad, bool) {
	if len(loads) < 2 {
		return internalsharding.ShardLoad{}, false
	}

	maxLoad := loads[0]
	minLoad := loads[len(loads)-1]

	if maxLoad.ShardID == minLoad.ShardID {
		return internalsharding.ShardLoad{}, false
	}

	var total int64
	for _, load := range loads {
		total += load.Load
	}

	if total == 0 {
		return internalsharding.ShardLoad{}, false
	}

	avg := float64(total) / float64(len(loads))
	if avg <= 0 {
		return internalsharding.ShardLoad{}, false
	}

	if float64(maxLoad.Load) <= avg*w.imbalanceThreshold {
		return internalsharding.ShardLoad{}, false
	}

	return maxLoad, true
}

func (w *ShardRebalanceWorker) tryMigrateHotAccount(
	ctx context.Context,
	hot internalsharding.HotAccount,
	maxLoad internalsharding.ShardLoad,
	loadByShard map[int]int64,
	isolationCounts map[int]int64,
) (bool, error) {
	share := float64(hot.Load) / float64(maxLoad.Load)
	mustIsolate := share >= w.isolationShare && hot.Load >= w.isolationMinLoad

	targetShard, ok := selectTargetShard(loadByShard, maxLoad.ShardID, hot.Load, mustIsolate, isolationCounts)
	if !ok {
		return false, nil
	}

	permitOK, permitErr := w.manager.TryAcquireRebalancePermits(ctx, maxLoad.ShardID, targetShard, hot)
	if permitErr != nil {
		return false, permitErr
	}

	if !permitOK {
		return false, nil
	}

	result, migrationErr := w.manager.MigrateAccount(ctx, hot.OrganizationID, hot.LedgerID, hot.Alias, targetShard, nil)
	if migrationErr != nil {
		w.logger.Warnf(
			"ShardRebalanceWorker: migration failed alias=%s org=%s ledger=%s from=%d to=%d err=%v",
			hot.Alias,
			hot.OrganizationID.String(),
			hot.LedgerID.String(),
			maxLoad.ShardID,
			targetShard,
			migrationErr,
		)

		return false, nil
	}

	if result == nil {
		w.logger.Warnf(
			"ShardRebalanceWorker: migration returned nil result alias=%s org=%s ledger=%s from=%d to=%d",
			hot.Alias,
			hot.OrganizationID.String(),
			hot.LedgerID.String(),
			maxLoad.ShardID,
			targetShard,
		)

		return false, nil
	}

	if mustIsolate {
		if markErr := w.manager.MarkAccountIsolated(ctx, hot, targetShard); markErr != nil {
			w.logger.Warnf(
				"ShardRebalanceWorker: failed to mark account isolation alias=%s shard=%d err=%v",
				hot.Alias,
				targetShard,
				markErr,
			)
		}
	}

	w.logger.Infof(
		"ShardRebalanceWorker: migrated alias=%s org=%s ledger=%s from=%d to=%d load=%d isolated=%v migrated_keys=%d",
		hot.Alias,
		hot.OrganizationID.String(),
		hot.LedgerID.String(),
		result.SourceShard,
		result.TargetShard,
		hot.Load,
		mustIsolate,
		result.MigratedKeys,
	)

	return true, nil
}

type shardCandidate struct {
	shardID      int
	maxProjected int64
	spread       int64
	projected    int64
}

func selectTargetShard(loadByShard map[int]int64, sourceShard int, accountLoad int64, mustIsolate bool, isolationCounts map[int]int64) (int, bool) {
	if accountLoad <= 0 {
		return 0, false
	}

	candidates := buildShardCandidates(loadByShard, sourceShard, accountLoad, isolationCounts)

	if len(candidates) == 0 {
		if mustIsolate {
			return 0, false
		}

		candidates = buildFallbackCandidates(loadByShard, sourceShard, accountLoad)
	}

	if len(candidates) == 0 {
		return 0, false
	}

	sortShardCandidates(candidates)

	return candidates[0].shardID, true
}

func buildShardCandidates(loadByShard map[int]int64, sourceShard int, accountLoad int64, isolationCounts map[int]int64) []shardCandidate {
	candidates := make([]shardCandidate, 0, len(loadByShard))

	for shardID, targetLoad := range loadByShard {
		if shardID == sourceShard || isolationCounts[shardID] > 0 {
			continue
		}

		c := computeProjectedCandidate(loadByShard, sourceShard, shardID, targetLoad, accountLoad)
		candidates = append(candidates, c)
	}

	return candidates
}

func computeProjectedCandidate(loadByShard map[int]int64, sourceShard, shardID int, targetLoad, accountLoad int64) shardCandidate {
	projectedLoads := make([]int64, 0, len(loadByShard))

	for currentShard, currentLoad := range loadByShard {
		switch currentShard {
		case sourceShard:
			projectedLoads = append(projectedLoads, maxInt64(0, currentLoad-accountLoad))
		case shardID:
			projectedLoads = append(projectedLoads, targetLoad+accountLoad)
		default:
			projectedLoads = append(projectedLoads, currentLoad)
		}
	}

	sort.Slice(projectedLoads, func(i, j int) bool { return projectedLoads[i] > projectedLoads[j] })

	maxProjected := projectedLoads[0]
	minProjected := projectedLoads[len(projectedLoads)-1]

	return shardCandidate{
		shardID:      shardID,
		maxProjected: maxProjected,
		spread:       maxProjected - minProjected,
		projected:    targetLoad + accountLoad,
	}
}

func buildFallbackCandidates(loadByShard map[int]int64, sourceShard int, accountLoad int64) []shardCandidate {
	candidates := make([]shardCandidate, 0, len(loadByShard))

	for shardID, targetLoad := range loadByShard {
		if shardID == sourceShard {
			continue
		}

		projected := targetLoad + accountLoad
		candidates = append(candidates, shardCandidate{
			shardID:      shardID,
			maxProjected: projected,
			spread:       projected,
			projected:    projected,
		})
	}

	return candidates
}

func sortShardCandidates(candidates []shardCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		ci, cj := candidates[i], candidates[j]
		if ci.maxProjected != cj.maxProjected {
			return ci.maxProjected < cj.maxProjected
		}

		if ci.spread != cj.spread {
			return ci.spread < cj.spread
		}

		if ci.projected != cj.projected {
			return ci.projected < cj.projected
		}

		return ci.shardID < cj.shardID
	})
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}

	return b
}
