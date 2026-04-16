// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/protobuf/proto"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

// Rejection codes returned in AuthorizeResponse when authorization fails.
const (
	RejectionInsufficientFunds = "INSUFFICIENT_FUNDS"
	RejectionBalanceNotFound   = "BALANCE_NOT_FOUND"
	RejectionAmountExceedsHold = "AMOUNT_EXCEEDS_HOLD"
	RejectionAccountIneligible = "ACCOUNT_INELIGIBLE"
	RejectionInternalError     = "INTERNAL_ERROR"
	RejectionRequestTooLarge   = "REQUEST_TOO_LARGE"

	defaultMaxAuthorizeOperationsPerRequest = 2048
	defaultMaxUniqueBalancesPerRequest      = 2048
	defaultMaxReplayMutationsPerEntry       = 2048
	defaultMaxReplayUniqueBalancesPerEntry  = 2048
)

type shardWorker struct {
	mu       sync.RWMutex
	balances map[string]*Balance
}

// Observer defines a set of hooks for observing engine operations such as
// lock wait/hold durations, WAL append failures, and replay skip events.
type Observer interface {
	ObserveAuthorizeLockWait(lockCount, shardCount int, wait time.Duration)
	ObserveAuthorizeLockHold(lockCount, shardCount int, hold time.Duration)
	ObserveWALAppendFailure(err error)
	ObserveWALReplaySkipped(reason, transactionID string, entryIndex int)
}

// Engine provides in-memory transaction authorization with deterministic per-balance locking.
type Engine struct {
	router    *shard.Router
	workers   []*shardWorker
	wal       wal.Writer
	observe   Observer
	loaded    atomic.Int64
	prepStore *preparedTxStore
	stopOnce  sync.Once
	stopCh    chan struct{}

	// started is set to true the first time Authorize, PrepareAuthorize, or ReplayEntries
	// is called. After that, ConfigureAuthorizationLimits and ConfigureReplayPolicy will
	// no-op to prevent unsynchronized writes to the fields below while hot-path methods
	// read them concurrently. All configuration must happen before the engine serves traffic.
	started atomic.Bool

	maxAuthorizeOperationsPerRequest int
	maxUniqueBalancesPerRequest      int
	maxReplayMutationsPerEntry       int
	maxReplayUniqueBalancesPerEntry  int
	replayStrictMode                 bool
}

// New creates a new Engine with the given shard router and WAL writer.
// If router or walWriter is nil, sensible defaults are used.
func New(router *shard.Router, walWriter wal.Writer) *Engine {
	if router == nil {
		router = shard.NewRouter(shard.DefaultShardCount)
	}

	if walWriter == nil {
		walWriter = wal.NewNoopWriter()
	}

	workers := make([]*shardWorker, 0, router.ShardCount())
	for i := 0; i < router.ShardCount(); i++ {
		workers = append(workers, &shardWorker{balances: make(map[string]*Balance)})
	}

	eng := &Engine{
		router:                           router,
		workers:                          workers,
		wal:                              walWriter,
		prepStore:                        newPreparedTxStore(DefaultPrepareTimeout, DefaultMaxPreparedTx),
		stopCh:                           make(chan struct{}),
		maxAuthorizeOperationsPerRequest: defaultMaxAuthorizeOperationsPerRequest,
		maxUniqueBalancesPerRequest:      defaultMaxUniqueBalancesPerRequest,
		maxReplayMutationsPerEntry:       defaultMaxReplayMutationsPerEntry,
		maxReplayUniqueBalancesPerEntry:  defaultMaxReplayUniqueBalancesPerEntry,
		replayStrictMode:                 true,
	}

	go eng.reapExpiredPrepared()

	return eng
}

// SetWALWriter replaces the active WAL writer. Unlike ConfigureAuthorizationLimits,
// this is deliberately not guarded by the started flag because:
// (1) the boot sequence calls ReplayEntries (which sets started=true) before
//
//	installing the real WAL writer, and
//
// (2) swapping the writer is a valid recovery operation after WAL append failures.
func (e *Engine) SetWALWriter(writer wal.Writer) {
	if e == nil {
		return
	}

	if writer == nil {
		e.wal = wal.NewNoopWriter()
		return
	}

	e.wal = writer
}

// SetObserver assigns an Observer for engine metrics. Must be called before started.
func (e *Engine) SetObserver(observer Observer) {
	if e == nil || e.started.Load() {
		return
	}

	e.observe = observer
}

// ConfigurePreparedTxStore sets timeout and max pending capacity for the prepared transaction store.
func (e *Engine) ConfigurePreparedTxStore(timeout time.Duration, maxPending int) {
	if e == nil || e.prepStore == nil {
		return
	}

	e.prepStore.mu.Lock()
	defer e.prepStore.mu.Unlock()

	if timeout > 0 {
		e.prepStore.timeout = timeout
	}

	if maxPending > 0 {
		e.prepStore.max = maxPending
	}
}

// ConfigurePreparedTxRetention sets the committed TTL and max commit retries for prepared transactions.
func (e *Engine) ConfigurePreparedTxRetention(committedTTL time.Duration, maxCommitRetries int) {
	if e == nil || e.prepStore == nil {
		return
	}

	e.prepStore.mu.Lock()
	defer e.prepStore.mu.Unlock()

	if committedTTL > 0 {
		e.prepStore.committedTTL = committedTTL
	}

	if maxCommitRetries > 0 {
		e.prepStore.maxRetries = maxCommitRetries
	}
}

// ConfigureAuthorizationLimits sets per-request size limits for the authorization hot path.
// Must be called during startup, before the engine begins serving concurrent requests.
// Calls after the engine has started serving are silently ignored to avoid data races.
func (e *Engine) ConfigureAuthorizationLimits(maxOperationsPerRequest, maxUniqueBalancesPerRequest int) {
	if e == nil || e.started.Load() {
		return
	}

	if maxOperationsPerRequest > 0 {
		e.maxAuthorizeOperationsPerRequest = maxOperationsPerRequest
	}

	if maxUniqueBalancesPerRequest > 0 {
		e.maxUniqueBalancesPerRequest = maxUniqueBalancesPerRequest
	}
}

// ConfigureReplayPolicy sets WAL replay limits and strict-mode behavior.
// Must be called during startup, before the engine begins serving concurrent requests.
// Calls after the engine has started serving are silently ignored to avoid data races.
func (e *Engine) ConfigureReplayPolicy(maxMutationsPerEntry, maxUniqueBalancesPerEntry int, strictMode bool) {
	if e == nil || e.started.Load() {
		return
	}

	if maxMutationsPerEntry > 0 {
		e.maxReplayMutationsPerEntry = maxMutationsPerEntry
	}

	if maxUniqueBalancesPerEntry > 0 {
		e.maxReplayUniqueBalancesPerEntry = maxUniqueBalancesPerEntry
	}

	e.replayStrictMode = strictMode
}

// ShardCount returns the number of shards in the engine's router.
func (e *Engine) ShardCount() int {
	if e == nil || e.router == nil {
		return 0
	}

	return e.router.ShardCount()
}

// LoadedBalances returns the total number of balances that have been inserted via UpsertBalances.
func (e *Engine) LoadedBalances() int64 {
	if e == nil {
		return 0
	}

	return e.loaded.Load()
}

// ValidateRequestLimits checks whether the request exceeds configured size limits.
// Returns a rejection response if limits are exceeded, or nil if the request is within bounds.
func (e *Engine) ValidateRequestLimits(req *authorizerv1.AuthorizeRequest) *authorizerv1.AuthorizeResponse {
	if e == nil || req == nil || len(req.GetOperations()) == 0 {
		return nil
	}

	if e.maxAuthorizeOperationsPerRequest > 0 && len(req.GetOperations()) > e.maxAuthorizeOperationsPerRequest {
		return &authorizerv1.AuthorizeResponse{
			Authorized:       false,
			RejectionCode:    RejectionRequestTooLarge,
			RejectionMessage: "operations exceed allowed request limit",
		}
	}

	// NOTE: The unique-balance limit is enforced inside prepareAuthorization after
	// normalizeExternalOperations has already been called. Moving the check there
	// avoids a redundant proto.Clone-per-operation on the hot path (see S1-H1).

	return nil
}

// UpsertBalances inserts new balances or updates existing ones using version-gated semantics.
// Returns the number of newly inserted balances.
func (e *Engine) UpsertBalances(balances []*Balance) int64 {
	// Nil-safety: mirror the guard used by sibling methods (ShardCount,
	// GetBalance). A half-constructed engine where router was never set would
	// otherwise panic on the first ResolveBalance call below. The worker-pool
	// length check covers the equally pathological case of a router that
	// resolves to a shard ID outside the materialized workers slice.
	if e == nil || e.router == nil || len(e.workers) == 0 {
		return 0
	}

	// Caller contract: balances passed here must be immutable for the duration of this
	// call. Reusing and mutating the same *Balance concurrently is not supported.

	var inserted int64

	for _, balance := range balances {
		if e.upsertOneBalance(balance) {
			inserted++
		}
	}

	e.loaded.Add(inserted)

	return inserted
}

// upsertOneBalance inserts or version-gated-updates a single balance.
// Returns true when a new entry was inserted (so the caller can increment
// the insertion counter). All nil-safety and bounds checks are co-located
// here so UpsertBalances stays under the cyclomatic complexity threshold.
func (e *Engine) upsertOneBalance(balance *Balance) bool {
	if balance == nil {
		return false
	}

	balanceKey := balance.BalanceKey
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	workerID := e.router.ResolveBalance(balance.AccountAlias, balanceKey)

	// Bounds check protects against a router misconfigured with a shard
	// count larger than the materialized worker pool. Without this, the
	// index below would panic with runtime out-of-range.
	if workerID < 0 || workerID >= len(e.workers) {
		return false
	}

	worker := e.workers[workerID]
	if worker == nil {
		return false
	}

	lookupKey := balanceLookupKey(balance.OrganizationID, balance.LedgerID, balance.AccountAlias, balanceKey)

	// Double-check locking (TOCTOU-safe pattern):
	//
	// 1. RLock the shard to check whether the balance already exists (fast path).
	// 2. If not found, upgrade to a full Lock and re-check (slow path, handles the
	//    race where another goroutine inserted between RUnlock and Lock).
	//
	// For existing balances, the window between RUnlock and the per-balance Lock below
	// is intentional and safe: the pointer stability invariant guarantees the map entry
	// is never replaced, so the pointer obtained under RLock remains valid. Concurrent
	// Authorize/Replay may mutate the balance's Available/OnHold/Version fields during
	// this window. The version-gated overwrite below ensures that only a newer snapshot
	// from the database wins; any in-flight authorization mutations that incremented
	// the version will cause the version guard to skip the overwrite, which is correct
	// because the in-memory state already reflects the latest mutations.
	//
	// Caller contract: the snapshot passed to UpsertBalances must come from a database
	// read that is at least as recent as the last WAL flush. This ensures that the
	// Version field in the snapshot already accounts for all replayed mutations, so the
	// version comparison (balance.Version > existing.Version) produces the correct result.
	worker.mu.RLock()
	existing, exists := worker.balances[lookupKey]
	worker.mu.RUnlock()

	if !exists || existing == nil {
		worker.mu.Lock()

		existing, exists = worker.balances[lookupKey]
		if !exists || existing == nil {
			copyBalance := balance.clone()
			copyBalance.BalanceKey = balanceKey
			// Pointer stability invariant: once a balance pointer is inserted for a lookup key,
			// future upserts mutate that same object in place and never replace the map entry.
			// Authorize/Prepare rely on this invariant between map lookup and balance lock.
			worker.balances[lookupKey] = copyBalance
			worker.mu.Unlock()

			return true
		}

		worker.mu.Unlock()
	}

	existing.mu.Lock()
	if balance.Version > existing.Version {
		existing.overwriteFrom(balance, balanceKey)
	} else if balance.Version == existing.Version {
		existing.overwritePolicyFrom(balance, balanceKey)
	}
	existing.mu.Unlock()

	return false
}

// GetBalance returns a snapshot of the balance identified by the given keys.
// Returns false if the balance is not loaded in the engine.
func (e *Engine) GetBalance(organizationID, ledgerID, accountAlias, balanceKey string) (*Balance, bool) {
	if e == nil || e.router == nil {
		return nil, false
	}

	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	workerID := e.router.ResolveBalance(accountAlias, balanceKey)
	worker := e.workers[workerID]
	lookupKey := balanceLookupKey(organizationID, ledgerID, accountAlias, balanceKey)

	worker.mu.RLock()
	balance, ok := worker.balances[lookupKey]
	worker.mu.RUnlock()

	if !ok || balance == nil {
		return nil, false
	}

	balance.mu.Lock()
	snapshot := balance.clone()
	balance.mu.Unlock()

	return snapshot, true
}

// CountShardsForOperations returns the number of unique shards that the given operations span.
func (e *Engine) CountShardsForOperations(ops []*authorizerv1.BalanceOperation) int {
	if e == nil || e.router == nil || len(ops) == 0 {
		return 0
	}

	normalized := normalizeExternalOperations(ops, e.router)
	if len(normalized) == 0 {
		return 0
	}

	shards := make(map[int]struct{}, len(normalized))
	for _, op := range normalized {
		if op == nil {
			continue
		}

		workerID := e.router.ResolveBalance(op.GetAccountAlias(), op.GetBalanceKey())
		shards[workerID] = struct{}{}
	}

	return len(shards)
}

type replayMutationRef struct {
	mutation wal.BalanceMutation
	balance  *Balance
}

type resolvedOp struct {
	op         *authorizerv1.BalanceOperation
	lookupKey  string
	balance    *Balance
	balanceKey string
	alias      string
}

type preparedOperation struct {
	lookupKey      string
	balance        *Balance
	operationAlias string
	canonicalAlias string
	canonicalKey   string
	balanceID      string
	accountID      string
	assetCode      string
	accountType    string
	allowSending   bool
	allowReceiving bool
	scale          int32
	preVersion     uint64
	preAvail       int64
	preHold        int64
	postAvail      int64
	postHold       int64
	hasChange      bool
}

type authorizationDraft struct {
	normalized     []*authorizerv1.BalanceOperation
	prepared       []preparedOperation
	lockedBalances []*Balance
	lockedShards   int
	changedOps     int
}

// Authorize is the fast-path single-instance authorization.
// It prepares and commits in one call (no cross-shard coordination needed).
// For cross-shard transactions, the gRPC service uses PrepareAuthorize + CommitPrepared directly.
func (e *Engine) Authorize(req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	return e.authorize(req, true)
}

func (e *Engine) authorize(req *authorizerv1.AuthorizeRequest, persistWAL bool) (*authorizerv1.AuthorizeResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("%w", constant.ErrAuthorizerEngineNotInitialized)
	}

	e.started.Store(true)

	if req == nil || len(req.Operations) == 0 {
		return &authorizerv1.AuthorizeResponse{Authorized: true}, nil
	}

	if rejection := e.ValidateRequestLimits(req); rejection != nil {
		return rejection, nil
	}

	draft, rejection, releaseLocks := e.prepareAuthorization(req)
	defer releaseLocks() // safety net: ensure locks are always released even on panic

	if rejection != nil {
		releaseLocks()
		return rejection, nil
	}

	organizationID := req.GetOrganizationId()
	ledgerID := req.GetLedgerId()
	pending := req.GetPending()
	transactionStatus := req.GetTransactionStatus()

	if persistWAL && draft.changedOps > 0 {
		mutations := buildWALMutations(draft.prepared)

		err := e.wal.Append(wal.Entry{
			TransactionID:     req.GetTransactionId(),
			OrganizationID:    organizationID,
			LedgerID:          ledgerID,
			Pending:           pending,
			TransactionStatus: transactionStatus,
			Operations:        draft.normalized,
			Mutations:         mutations,
		})
		if err != nil {
			// NOTE: observe may be a typed nil (e.g., (*authorizerMetrics)(nil)).
			// This is safe because all Observer methods guard against nil receiver.
			if e.observe != nil {
				e.observe.ObserveWALAppendFailure(err)
			}

			releaseLocks()

			return nil, fmt.Errorf("append wal entry: %w", err)
		}
	}

	for _, op := range draft.prepared {
		if !op.hasChange {
			continue
		}

		op.balance.Available = op.postAvail
		op.balance.OnHold = op.postHold
		// IMPORTANT: Version increments by exactly 1 per mutation per balance.
		// This must match WAL BalanceMutation.NextVersion = PreviousVersion + 1
		// in buildWALMutations. Changing this increment breaks WAL replay.
		op.balance.Version++
	}

	// Release before building snapshots to minimize lock hold time on hot paths.
	releaseLocks()

	snapshots := buildAuthorizeSnapshots(draft.prepared)

	return &authorizerv1.AuthorizeResponse{
		Authorized: true,
		Balances:   snapshots,
	}, nil
}

func (e *Engine) prepareAuthorization(req *authorizerv1.AuthorizeRequest) (*authorizationDraft, *authorizerv1.AuthorizeResponse, func()) {
	normalized := normalizeExternalOperations(req.Operations, e.router)
	organizationID := req.GetOrganizationId()
	ledgerID := req.GetLedgerId()
	pending := req.GetPending()
	transactionStatus := req.GetTransactionStatus()

	// Phase 1: Resolve all operations to their balance pointers.
	resolved, uniqueBalances, rejection := e.resolveOperationBalances(normalized, organizationID, ledgerID)
	if rejection != nil {
		return nil, rejection, func() {}
	}

	// Phase 2: Acquire per-balance locks in deterministic order.
	lockedBalances := getSortedBalances(uniqueBalances)
	lockedShardCount := countUniqueShardsForBalances(lockedBalances, e.router)

	var lockWaitTotal time.Duration

	for i := range lockedBalances {
		lockStart := time.Now()

		lockedBalances[i].mu.Lock()

		lockWaitTotal += time.Since(lockStart)
	}

	if e.observe != nil {
		e.observe.ObserveAuthorizeLockWait(len(lockedBalances), lockedShardCount, lockWaitTotal)
	}

	lockHoldStart := time.Now()
	locksReleased := false

	releaseLocks := func() {
		if locksReleased {
			return
		}

		if e.observe != nil {
			e.observe.ObserveAuthorizeLockHold(len(lockedBalances), lockedShardCount, time.Since(lockHoldStart))
		}

		// Release in reverse acquisition order.
		unlockBalancesReverse(lockedBalances)

		locksReleased = true
	}

	// Phase 3: Validate and stage all operations (under per-balance locks).
	prepared, changedOperations, stageRejection := stageOperations(resolved, pending, transactionStatus)
	if stageRejection != nil {
		return nil, stageRejection, releaseLocks
	}

	return &authorizationDraft{
		normalized:     normalized,
		prepared:       prepared,
		lockedBalances: lockedBalances,
		lockedShards:   lockedShardCount,
		changedOps:     changedOperations,
	}, nil, releaseLocks
}

// stageOperations validates and stages all resolved operations, computing pre/post balance states.
// Returns the prepared operations, changed operation count, and a rejection response if validation fails.
func stageOperations(
	resolved []resolvedOp, pending bool, transactionStatus string,
) ([]preparedOperation, int, *authorizerv1.AuthorizeResponse) {
	prepared := make([]preparedOperation, 0, len(resolved))
	staged := make(map[*Balance]*Balance, len(resolved))
	changedOperations := 0

	for _, r := range resolved {
		actualBalance := r.balance

		workingBalance, ok := staged[actualBalance]
		if !ok {
			workingBalance = actualBalance.clone()
			staged[actualBalance] = workingBalance
		}

		amount, rescaleErr := rescaleAmount(r.op.GetAmount(), r.op.GetScale(), workingBalance.Scale)
		if rescaleErr != nil {
			return nil, 0, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionInternalError,
				RejectionMessage: rescaleErr.Error(),
			}
		}

		if amount < 0 {
			return nil, 0, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionInternalError,
				RejectionMessage: "amount must be non-negative",
			}
		}

		preAvail := workingBalance.Available
		preHold := workingBalance.OnHold

		postAvail, postHold, applyErr := applyOperation(
			preAvail,
			preHold,
			pending,
			transactionStatus,
			r.op.GetOperation(),
			amount,
		)
		if applyErr != nil {
			return nil, 0, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionInternalError,
				RejectionMessage: applyErr.Error(),
			}
		}

		ok, rejectionCode, rejectionMessage := validateBalanceRules(
			workingBalance,
			r.op,
			preAvail,
			preHold,
			postAvail,
			postHold,
		)
		if !ok {
			return nil, 0, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    rejectionCode,
				RejectionMessage: rejectionMessage,
			}
		}

		workingBalance.Available = postAvail
		workingBalance.OnHold = postHold

		hasChange := preAvail != postAvail || preHold != postHold
		if hasChange {
			changedOperations++
		}

		prepared = append(prepared, preparedOperation{
			lookupKey:      r.lookupKey,
			balance:        actualBalance,
			operationAlias: r.op.GetOperationAlias(),
			canonicalAlias: r.alias,
			canonicalKey:   r.balanceKey,
			balanceID:      actualBalance.ID,
			accountID:      actualBalance.AccountID,
			assetCode:      actualBalance.AssetCode,
			accountType:    actualBalance.AccountType,
			allowSending:   actualBalance.AllowSending,
			allowReceiving: actualBalance.AllowReceiving,
			scale:          actualBalance.Scale,
			preVersion:     actualBalance.Version,
			preAvail:       preAvail,
			preHold:        preHold,
			postAvail:      postAvail,
			postHold:       postHold,
			hasChange:      hasChange,
		})
	}

	return prepared, changedOperations, nil
}

// resolveOperationBalances resolves each operation to its in-memory balance pointer.
// Returns the resolved operations, unique balance map, and a rejection response if any operation fails.
func (e *Engine) resolveOperationBalances(
	normalized []*authorizerv1.BalanceOperation,
	organizationID, ledgerID string,
) ([]resolvedOp, map[string]*Balance, *authorizerv1.AuthorizeResponse) {
	resolved := make([]resolvedOp, 0, len(normalized))
	uniqueBalances := make(map[string]*Balance, len(normalized))

	for _, op := range normalized {
		if op == nil {
			continue
		}

		balanceKey := op.GetBalanceKey()
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		canonicalAlias := op.GetAccountAlias()
		workerID := e.router.ResolveBalance(canonicalAlias, balanceKey)
		lookupKey := balanceLookupKey(organizationID, ledgerID, canonicalAlias, balanceKey)

		// Check if we already resolved this balance in this batch.
		if bal, ok := uniqueBalances[lookupKey]; ok {
			resolved = append(resolved, resolvedOp{
				op: op, lookupKey: lookupKey, balance: bal,
				balanceKey: balanceKey, alias: canonicalAlias,
			})

			continue
		}

		// Brief shard read-lock for map lookup only.
		// The returned balance pointer remains valid after RUnlock because UpsertBalances
		// never replaces existing map entries (pointer stability invariant).
		worker := e.workers[workerID]
		worker.mu.RLock()
		actualBalance, ok := worker.balances[lookupKey]
		worker.mu.RUnlock()

		if !ok || actualBalance == nil {
			return nil, nil, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionBalanceNotFound,
				RejectionMessage: "balance not found",
			}
		}

		uniqueBalances[lookupKey] = actualBalance

		// Enforce unique-balance limit early, before acquiring any per-balance locks.
		if e.maxUniqueBalancesPerRequest > 0 && len(uniqueBalances) > e.maxUniqueBalancesPerRequest {
			return nil, nil, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionRequestTooLarge,
				RejectionMessage: "unique balances exceed allowed request limit",
			}
		}

		resolved = append(resolved, resolvedOp{
			op: op, lookupKey: lookupKey, balance: actualBalance,
			balanceKey: balanceKey, alias: canonicalAlias,
		})
	}

	return resolved, uniqueBalances, nil
}

func buildAuthorizeSnapshots(prepared []preparedOperation) []*authorizerv1.BalanceSnapshot {
	snapshots := make([]*authorizerv1.BalanceSnapshot, 0, len(prepared))

	for _, op := range prepared {
		if !op.hasChange {
			continue
		}

		// BalanceSnapshot intentionally reports pre-mutation state for idempotent,
		// deterministic replay and client-side reconciliation against the original
		// authorization baseline.
		snapshots = append(snapshots, &authorizerv1.BalanceSnapshot{
			OperationAlias: op.operationAlias,
			AccountAlias:   op.canonicalAlias,
			BalanceKey:     op.canonicalKey,
			BalanceId:      op.balanceID,
			AccountId:      op.accountID,
			AssetCode:      op.assetCode,
			AccountType:    op.accountType,
			AllowSending:   op.allowSending,
			AllowReceiving: op.allowReceiving,
			Available:      op.preAvail,
			OnHold:         op.preHold,
			Scale:          op.scale,
			Version:        op.preVersion,
		})
	}

	return snapshots
}

func buildWALMutations(operations []preparedOperation) []wal.BalanceMutation {
	byLookup := make(map[string]*wal.BalanceMutation, len(operations))

	for _, op := range operations {
		if !op.hasChange {
			continue
		}

		mutation, exists := byLookup[op.lookupKey]
		if !exists {
			// IMPORTANT: NextVersion = PreviousVersion + 1 must match the Version++
			// increment in Authorize and CommitPrepared commit paths. Changing
			// this relationship breaks WAL replay version gating.
			mutation = &wal.BalanceMutation{
				AccountAlias:    op.canonicalAlias,
				BalanceKey:      op.canonicalKey,
				Available:       op.postAvail,
				OnHold:          op.postHold,
				PreviousVersion: op.preVersion,
				NextVersion:     op.preVersion + 1,
			}
			byLookup[op.lookupKey] = mutation

			continue
		}

		mutation.Available = op.postAvail
		mutation.OnHold = op.postHold
		mutation.NextVersion++
	}

	if len(byLookup) == 0 {
		return nil
	}

	lookupKeys := make([]string, 0, len(byLookup))
	for lookupKey := range byLookup {
		lookupKeys = append(lookupKeys, lookupKey)
	}

	sort.Strings(lookupKeys)

	mutations := make([]wal.BalanceMutation, 0, len(lookupKeys))
	for _, lookupKey := range lookupKeys {
		mutations = append(mutations, *byLookup[lookupKey])
	}

	return mutations
}

// ReplayEntries applies WAL entries to the in-memory balance state.
// It is used during startup recovery to bring the engine up to date.
func (e *Engine) ReplayEntries(entries []wal.Entry) error {
	if e == nil || e.router == nil {
		return fmt.Errorf("%w", constant.ErrAuthorizerEngineNotInitialized)
	}

	e.started.Store(true)

	replaySkipped := func(reason, transactionID string, entryIndex int) error {
		if e.observe != nil {
			e.observe.ObserveWALReplaySkipped(reason, transactionID, entryIndex)
		}

		if e.replayStrictMode {
			return fmt.Errorf("%w: %s", constant.ErrWALReplayStrictModeRejected, reason)
		}

		return nil
	}

	for entryIndex, entry := range entries {
		if len(entry.Mutations) == 0 {
			continue
		}

		if e.maxReplayMutationsPerEntry > 0 && len(entry.Mutations) > e.maxReplayMutationsPerEntry {
			if err := replaySkipped("mutation_limit_exceeded", entry.TransactionID, entryIndex); err != nil {
				return err
			}

			continue
		}

		if err := e.replayEntry(entry, entryIndex, replaySkipped); err != nil {
			return err
		}
	}

	return nil
}

func (e *Engine) replayEntry(entry wal.Entry, entryIndex int, replaySkipped func(string, string, int) error) error {
	// Resolve all mutation balances and collect unique locks.
	refs := make([]replayMutationRef, 0, len(entry.Mutations))
	uniqueLocks := make(map[string]*Balance, len(entry.Mutations))

	for _, mutation := range entry.Mutations {
		balanceKey := mutation.BalanceKey
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		workerID := e.router.ResolveBalance(mutation.AccountAlias, balanceKey)
		lookupKey := balanceLookupKey(entry.OrganizationID, entry.LedgerID, mutation.AccountAlias, balanceKey)

		worker := e.workers[workerID]
		worker.mu.RLock()
		balance, ok := worker.balances[lookupKey]
		worker.mu.RUnlock()

		if !ok || balance == nil {
			// Balance not loaded yet -- skip entire entry.
			if err := replaySkipped("missing_balance", entry.TransactionID, entryIndex); err != nil {
				return err
			}

			return nil
		}

		uniqueLocks[lookupKey] = balance

		if e.maxReplayUniqueBalancesPerEntry > 0 && len(uniqueLocks) > e.maxReplayUniqueBalancesPerEntry {
			if err := replaySkipped("lock_limit_exceeded", entry.TransactionID, entryIndex); err != nil {
				return err
			}

			return nil
		}

		refs = append(refs, replayMutationRef{mutation: mutation, balance: balance})
	}

	if len(refs) == 0 {
		return nil
	}

	// Lock balances in deterministic order.
	lockOrder := getSortedBalances(uniqueLocks)

	for i := range lockOrder {
		lockOrder[i].mu.Lock()
	}

	defer unlockBalancesReverse(lockOrder)

	return e.applyReplayRefs(refs, entry, entryIndex, replaySkipped)
}

func (e *Engine) applyReplayRefs(refs []replayMutationRef, entry wal.Entry, entryIndex int, replaySkipped func(string, string, int) error) error {
	for _, ref := range refs {
		bal := ref.balance
		mut := ref.mutation

		if bal.Version == mut.NextVersion && bal.Available == mut.Available && bal.OnHold == mut.OnHold {
			continue
		}

		if bal.Version != mut.PreviousVersion {
			if err := replaySkipped("version_mismatch", entry.TransactionID, entryIndex); err != nil {
				return err
			}

			return nil
		}
	}

	for _, ref := range refs {
		bal := ref.balance
		mut := ref.mutation

		if bal.Version == mut.NextVersion && bal.Available == mut.Available && bal.OnHold == mut.OnHold {
			continue
		}

		bal.Available = mut.Available
		bal.OnHold = mut.OnHold
		bal.Version = mut.NextVersion
	}

	return nil
}

func normalizeExternalOperations(ops []*authorizerv1.BalanceOperation, router *shard.Router) []*authorizerv1.BalanceOperation {
	if len(ops) == 0 {
		return ops
	}

	nonExternalAliases := make([]string, 0, len(ops))

	for _, op := range ops {
		if op == nil {
			continue
		}

		if !shard.IsExternal(op.GetAccountAlias()) {
			nonExternalAliases = append(nonExternalAliases, op.GetAccountAlias())
		}
	}

	if len(nonExternalAliases) == 0 {
		return ops
	}

	resolved := make([]*authorizerv1.BalanceOperation, 0, len(ops))
	externalIndex := 0

	for _, op := range ops {
		if op == nil {
			continue
		}

		cloned, ok := proto.Clone(op).(*authorizerv1.BalanceOperation)
		if !ok || cloned == nil {
			// proto.Clone failed or returned an unexpected type. Skipping the operation
			// prevents silent mutation of the caller's request through the original pointer.
			continue
		}

		if shard.IsExternal(cloned.GetAccountAlias()) && !shard.IsExternalBalanceKey(cloned.GetBalanceKey()) {
			counterparty := nonExternalAliases[externalIndex%len(nonExternalAliases)]

			cloned.BalanceKey = router.ResolveExternalBalanceKey(counterparty)
			externalIndex++
		}

		resolved = append(resolved, cloned)
	}

	return resolved
}

// safeAdd returns a + b, or an error if the result would overflow int64.
func safeAdd(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, fmt.Errorf("%w: %d + %d exceeds MaxInt64", constant.ErrIntegerOverflow, a, b)
	}

	if b < 0 && a < math.MinInt64-b {
		return 0, fmt.Errorf("%w: %d + %d exceeds MinInt64", constant.ErrIntegerUnderflow, a, b)
	}

	return a + b, nil
}

// safeSub returns a - b, or an error if the result would overflow/underflow int64.
func safeSub(a, b int64) (int64, error) {
	if b < 0 && a > math.MaxInt64+b {
		return 0, fmt.Errorf("%w: %d - %d exceeds MaxInt64", constant.ErrIntegerOverflow, a, b)
	}

	if b > 0 && a < math.MinInt64+b {
		return 0, fmt.Errorf("%w: %d - %d exceeds MinInt64", constant.ErrIntegerUnderflow, a, b)
	}

	return a - b, nil
}

func applyOperation(available, onHold int64, pending bool, transactionStatus, operation string, amount int64) (int64, int64, error) {
	if amount == 0 {
		return available, onHold, nil
	}

	op := strings.ToUpper(operation)
	status := strings.ToUpper(transactionStatus)

	if pending {
		return applyPendingOperation(available, onHold, status, op, amount)
	}

	return applyNonPendingOperation(available, onHold, op, amount)
}

func applyPendingOperation(available, onHold int64, status, op string, amount int64) (int64, int64, error) {
	var err error

	switch {
	case op == constant.ONHOLD && status == constant.PENDING:
		if available, err = safeSub(available, amount); err != nil {
			return 0, 0, err
		}

		if onHold, err = safeAdd(onHold, amount); err != nil {
			return 0, 0, err
		}
	case op == constant.RELEASE && status == constant.CANCELED:
		if onHold, err = safeSub(onHold, amount); err != nil {
			return 0, 0, err
		}

		if available, err = safeAdd(available, amount); err != nil {
			return 0, 0, err
		}
	case status == constant.TransactionStatusApprovedCompensate:
		available, onHold, err = applyApprovedCompensate(available, onHold, op, amount)
		if err != nil {
			return 0, 0, err
		}
	case status == constant.APPROVED:
		available, onHold, err = applyApproved(available, onHold, op, amount)
		if err != nil {
			return 0, 0, err
		}
	}

	return available, onHold, nil
}

func applyApprovedCompensate(available, onHold int64, op string, amount int64) (int64, int64, error) {
	var err error

	switch op {
	case constant.DEBIT:
		if onHold, err = safeAdd(onHold, amount); err != nil {
			return 0, 0, err
		}
	case constant.CREDIT:
		if available, err = safeSub(available, amount); err != nil {
			return 0, 0, err
		}
	case constant.RELEASE:
		if onHold, err = safeAdd(onHold, amount); err != nil {
			return 0, 0, err
		}

		if available, err = safeSub(available, amount); err != nil {
			return 0, 0, err
		}
	case constant.ONHOLD:
		if onHold, err = safeSub(onHold, amount); err != nil {
			return 0, 0, err
		}

		if available, err = safeAdd(available, amount); err != nil {
			return 0, 0, err
		}
	}

	return available, onHold, nil
}

func applyApproved(available, onHold int64, op string, amount int64) (int64, int64, error) {
	var err error

	switch op {
	case constant.DEBIT:
		if onHold, err = safeSub(onHold, amount); err != nil {
			return 0, 0, err
		}
	case constant.RELEASE:
		if onHold, err = safeSub(onHold, amount); err != nil {
			return 0, 0, err
		}

		if available, err = safeAdd(available, amount); err != nil {
			return 0, 0, err
		}
	default:
		if available, err = safeAdd(available, amount); err != nil {
			return 0, 0, err
		}
	}

	return available, onHold, nil
}

func applyNonPendingOperation(available, onHold int64, op string, amount int64) (int64, int64, error) {
	var err error

	if op == constant.DEBIT {
		if available, err = safeSub(available, amount); err != nil {
			return 0, 0, err
		}
	} else {
		if available, err = safeAdd(available, amount); err != nil {
			return 0, 0, err
		}
	}

	return available, onHold, nil
}

// AppendWALEntry surfaces the engine's configured WAL writer to bootstrap
// callers that need to persist out-of-band records (e.g., prepared-intent
// markers used for post-restart 2PC recovery). The engine does NOT consume
// these records itself — the bootstrap replay path reads them and takes the
// appropriate action. Returns the underlying writer's error verbatim.
//
// Callers MUST supply entries that are safe to round-trip through encode /
// replay: JSON-serializable, HMAC-coverable, and idempotent when replayed
// alongside mutation entries.
func (e *Engine) AppendWALEntry(entry wal.Entry) error {
	if e == nil || e.wal == nil {
		return fmt.Errorf("%w", constant.ErrAuthorizerEngineNotInitialized)
	}

	return e.wal.Append(entry) //nolint:wrapcheck // forwarded verbatim
}

// LookupPreparedTxByID returns the prepStore-managed PreparedTx for a given
// engine-assigned prepared tx ID. Returns nil if not present. Used by the
// prepared-intent replay path to detect whether an intent has already been
// restored by an earlier replay pass (idempotency guarantee).
func (e *Engine) LookupPreparedTxByID(preparedTxID string) *PreparedTx {
	if e == nil || e.prepStore == nil {
		return nil
	}

	e.prepStore.mu.Lock()
	defer e.prepStore.mu.Unlock()

	ptx, ok := e.prepStore.pending[preparedTxID]
	if !ok {
		return nil
	}

	return ptx
}

// RestorePreparedTxWithID overrides the engine-generated ID on a PreparedTx.
// Used only by the prepared-intent replay path so the restored prepared tx
// appears in prepStore under the SAME ID as the pre-crash original —
// coordinators calling CommitPrepared(originalID) after the restart will
// therefore find and commit the restored entry. The swap is done under
// the prepStore lock to ensure consistency with concurrent reapers.
//
// This method is intentionally limited to the bootstrap replay flow; the
// engine never calls it internally. Calling it on a PreparedTx already
// present in prepStore returns an error to prevent accidental collisions.
func (e *Engine) RestorePreparedTxWithID(restoredID string, original *PreparedTx) error {
	if e == nil || e.prepStore == nil {
		return fmt.Errorf("%w", constant.ErrPreparedTxStoreNil)
	}

	if original == nil {
		return fmt.Errorf("%w", constant.ErrPreparedTransactionNil)
	}

	e.prepStore.mu.Lock()
	defer e.prepStore.mu.Unlock()

	if _, exists := e.prepStore.pending[restoredID]; exists {
		return fmt.Errorf("%w: %s", constant.ErrPreparedTxAlreadyExists, restoredID)
	}

	// Remove the engine-assigned placeholder key and reinsert under the
	// originally-persisted ID, rewriting the PreparedTx.ID field so commit /
	// abort paths produce the correct audit trail.
	delete(e.prepStore.pending, original.ID)
	original.ID = restoredID
	e.prepStore.pending[restoredID] = original

	return nil
}

// Close stops the auto-abort goroutine. Safe to call multiple times.
func (e *Engine) Close() {
	if e == nil {
		return
	}

	e.stopOnce.Do(func() {
		close(e.stopCh)
	})
}

// TagCrossShard marks a pending prepared transaction as part of a cross-shard 2PC
// and records the participant list. Returns true if the transaction was found and tagged.
func (e *Engine) TagCrossShard(txID string, participants []wal.WALParticipant) bool {
	if e == nil || e.prepStore == nil {
		return false
	}

	e.prepStore.mu.Lock()
	defer e.prepStore.mu.Unlock()

	ptx, ok := e.prepStore.pending[txID]
	if !ok || ptx == nil || ptx.done {
		return false
	}

	ptx.CrossShard = true
	ptx.Participants = participants

	return true
}

// PrepareAuthorize validates operations and holds per-balance locks without committing.
// The returned PreparedTx handle must be completed via CommitPrepared or AbortPrepared.
// If validation fails, locks are released immediately and no handle is returned.
func (e *Engine) PrepareAuthorize(req *authorizerv1.AuthorizeRequest) (*PreparedTx, *authorizerv1.AuthorizeResponse, error) {
	if e == nil || e.router == nil {
		return nil, nil, fmt.Errorf("%w", constant.ErrAuthorizerEngineNotInitialized)
	}

	e.started.Store(true)

	if req == nil || len(req.Operations) == 0 {
		return nil, &authorizerv1.AuthorizeResponse{Authorized: true}, nil
	}

	if rejection := e.ValidateRequestLimits(req); rejection != nil {
		return nil, rejection, nil
	}

	draft, rejection, releaseLocks := e.prepareAuthorization(req)
	if rejection != nil {
		releaseLocks()
		return nil, rejection, nil
	}

	// Validation passed. Build the PreparedTx handle and hold locks.
	txID := "ptx-" + uuid.NewString()

	ptx := &PreparedTx{
		ID:               txID,
		Request:          req,
		normalized:       draft.normalized,
		prepared:         draft.prepared,
		changedOps:       draft.changedOps,
		lockedBalances:   draft.lockedBalances,
		lockedCount:      len(draft.lockedBalances),
		lockedShardCount: draft.lockedShards,
		createdAt:        time.Now(),
	}

	if err := e.prepStore.Put(ptx); err != nil {
		// Extremely unlikely (ID collision). Release locks and fail.
		releaseLocks()

		return nil, nil, err
	}

	// Build response snapshots for the caller (the prepare response).
	snapshots := buildAuthorizeSnapshots(draft.prepared)

	return ptx, &authorizerv1.AuthorizeResponse{
		Authorized: true,
		Balances:   snapshots,
	}, nil
}

// CommitPrepared writes the WAL entry, mutates live balances, and releases locks.
// Returns the balance snapshots after mutation.
// Idempotent: replaying CommitPrepared for an already committed prepared_tx_id
// returns the original committed response.
func (e *Engine) CommitPrepared(txID string) (*authorizerv1.AuthorizeResponse, error) {
	ptx, committedResp, found := e.prepStore.TakeForCommit(txID)
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrPreparedTxNotFound, txID)
	}

	if committedResp != nil {
		return committedResp, nil
	}

	if ptx == nil {
		return nil, fmt.Errorf("commit prepared %s: %w", txID, constant.ErrPreparedTransactionNil)
	}

	lockHoldStart := ptx.createdAt
	releaseLocks := true

	defer func() {
		if !releaseLocks {
			return
		}

		if e.observe != nil {
			e.observe.ObserveAuthorizeLockHold(ptx.lockedCount, ptx.lockedShardCount, time.Since(lockHoldStart))
		}

		// Release per-balance locks in reverse acquisition order.
		unlockBalancesReverse(ptx.lockedBalances)
	}()

	// Write WAL entry.
	if ptx.changedOps > 0 {
		if err := e.commitPreparedWAL(ptx, txID); err != nil {
			releaseLocks = false

			return nil, err
		}
	}

	// Mutate live balances.
	for _, op := range ptx.prepared {
		if !op.hasChange {
			continue
		}

		op.balance.Available = op.postAvail
		op.balance.OnHold = op.postHold
		// IMPORTANT: Version increments by exactly 1 per mutation per balance.
		// This must match WAL BalanceMutation.NextVersion = PreviousVersion + 1
		// in buildWALMutations. Changing this increment breaks WAL replay.
		op.balance.Version++
	}

	snapshots := buildAuthorizeSnapshots(ptx.prepared)
	resp := &authorizerv1.AuthorizeResponse{
		Authorized: true,
		Balances:   snapshots,
	}

	e.prepStore.MarkCommitted(txID, resp)

	return resp, nil
}

func (e *Engine) commitPreparedWAL(ptx *PreparedTx, txID string) error {
	mutations := buildWALMutations(ptx.prepared)

	err := e.wal.Append(wal.Entry{
		TransactionID:     ptx.Request.GetTransactionId(),
		OrganizationID:    ptx.Request.GetOrganizationId(),
		LedgerID:          ptx.Request.GetLedgerId(),
		Pending:           ptx.Request.GetPending(),
		TransactionStatus: ptx.Request.GetTransactionStatus(),
		Operations:        ptx.normalized,
		Mutations:         mutations,
		CrossShard:        ptx.CrossShard,
		Participants:      ptx.Participants,
	})
	if err != nil {
		if e.observe != nil {
			e.observe.ObserveWALAppendFailure(err)
		}

		if putBackErr := e.prepStore.PutBack(ptx); putBackErr != nil {
			return fmt.Errorf(
				"commit prepared %s: WAL append failed: %w (also failed to preserve prepared state: %s)",
				txID,
				err,
				putBackErr.Error(),
			)
		}

		return fmt.Errorf("commit prepared %s: WAL append failed: %w", txID, err)
	}

	return nil
}

// AbortPrepared releases locks without mutating any state.
// Returns an error if the transaction ID is not found.
func (e *Engine) AbortPrepared(txID string) error {
	ptx, err := e.prepStore.TakeForAbort(txID)
	if err != nil {
		if errors.Is(err, ErrPreparedTxNotFound) {
			return fmt.Errorf("%w: %s", ErrPreparedTxNotFound, txID)
		}

		return fmt.Errorf("%w: %s", err, txID)
	}

	if ptx == nil {
		return fmt.Errorf("abort prepared %s: %w", txID, constant.ErrPreparedTransactionNil)
	}

	defer func() {
		unlockBalancesReverse(ptx.lockedBalances)
	}()

	if e.observe != nil {
		e.observe.ObserveAuthorizeLockHold(ptx.lockedCount, ptx.lockedShardCount, time.Since(ptx.createdAt))
	}

	return nil
}

// reapExpiredPrepared runs as a goroutine and auto-aborts any prepared transactions
// that exceed the timeout. This prevents lock starvation if a coordinator crashes
// before calling CommitPrepared or AbortPrepared.
func (e *Engine) reapExpiredPrepared() {
	ticker := time.NewTicker(preparedCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			expired := e.prepStore.Expired()
			for _, ptx := range expired {
				if ptx == nil {
					continue
				}

				func(ptx *PreparedTx) {
					defer func() {
						unlockBalancesReverse(ptx.lockedBalances)
					}()

					if e.observe != nil {
						e.observe.ObserveAuthorizeLockHold(ptx.lockedCount, ptx.lockedShardCount, time.Since(ptx.createdAt))
					}
				}(ptx)
			}
		}
	}
}

// ResolveOperationShards normalizes external operations and groups them by shard ID.
// This exposes the same normalization logic used internally by PrepareAuthorize,
// allowing the gRPC service layer to detect cross-shard transactions and route
// operations to the correct authorizer instance before invoking the 2PC protocol.
func (e *Engine) ResolveOperationShards(ops []*authorizerv1.BalanceOperation) map[int][]*authorizerv1.BalanceOperation {
	if e == nil || e.router == nil || len(ops) == 0 {
		return make(map[int][]*authorizerv1.BalanceOperation)
	}

	normalized := normalizeExternalOperations(ops, e.router)

	result := make(map[int][]*authorizerv1.BalanceOperation, len(normalized))
	for _, op := range normalized {
		if op == nil {
			continue
		}

		shardID := e.router.ResolveBalance(op.GetAccountAlias(), op.GetBalanceKey())
		result[shardID] = append(result[shardID], op)
	}

	return result
}

func rescaleAmount(amount int64, fromScale, toScale int32) (int64, error) {
	if fromScale <= 0 {
		fromScale = pkgTransaction.DefaultScale
	}

	if toScale <= 0 {
		toScale = pkgTransaction.DefaultScale
	}

	if fromScale == toScale {
		return amount, nil
	}

	value := decimal.New(amount, -fromScale)

	return pkgTransaction.ScaleToInt(value, toScale)
}

func countUniqueShardsForBalances(balances []*Balance, router *shard.Router) int {
	if len(balances) == 0 || router == nil {
		return 0
	}

	shards := make(map[int]struct{}, len(balances))

	for _, balance := range balances {
		if balance == nil {
			continue
		}

		balanceKey := balance.BalanceKey
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		workerID := router.ResolveBalance(balance.AccountAlias, balanceKey)
		shards[workerID] = struct{}{}
	}

	return len(shards)
}

func getSortedBalances(m map[string]*Balance) []*Balance {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	res := make([]*Balance, len(keys))
	writeIdx := 0

	for _, k := range keys {
		balance := m[k]
		if balance == nil {
			continue
		}

		res[writeIdx] = balance
		writeIdx++
	}

	return res[:writeIdx]
}

func unlockBalancesReverse(balances []*Balance) {
	for i := len(balances) - 1; i >= 0; i-- {
		if balances[i] == nil {
			continue
		}

		balances[i].mu.Unlock()
	}
}
