// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	RejectionInsufficientFunds = "INSUFFICIENT_FUNDS"
	RejectionBalanceNotFound   = "BALANCE_NOT_FOUND"
	RejectionAmountExceedsHold = "AMOUNT_EXCEEDS_HOLD"
	RejectionAccountIneligible = "ACCOUNT_INELIGIBLE"
	RejectionInternalError     = "INTERNAL_ERROR"
)

type shardWorker struct {
	mu       sync.RWMutex
	balances map[string]*Balance
}

type Observer interface {
	ObserveAuthorizeLockWait(shardCount int, wait time.Duration)
	ObserveAuthorizeLockHold(shardCount int, hold time.Duration)
	ObserveWALAppendFailure(err error)
}

// Engine provides in-memory transaction authorization with shard-ordered locking.
type Engine struct {
	router    *shard.Router
	workers   []*shardWorker
	wal       wal.Writer
	observe   Observer
	loaded    atomic.Int64
	prepStore *preparedTxStore
	stopOnce  sync.Once
	stopCh    chan struct{}
}

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
		router:    router,
		workers:   workers,
		wal:       walWriter,
		prepStore: newPreparedTxStore(DefaultPrepareTimeout, DefaultMaxPreparedTx),
		stopCh:    make(chan struct{}),
	}

	go eng.reapExpiredPrepared()

	return eng
}

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

func (e *Engine) SetObserver(observer Observer) {
	if e == nil {
		return
	}

	e.observe = observer
}

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

func (e *Engine) ShardCount() int {
	if e == nil || e.router == nil {
		return 0
	}

	return e.router.ShardCount()
}

func (e *Engine) LoadedBalances() int64 {
	if e == nil {
		return 0
	}

	return e.loaded.Load()
}

func (e *Engine) UpsertBalances(balances []*Balance) int64 {
	if e == nil {
		return 0
	}

	var inserted int64

	for _, balance := range balances {
		if balance == nil {
			continue
		}

		balanceKey := balance.BalanceKey
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		workerID := e.router.ResolveBalance(balance.AccountAlias, balanceKey)
		worker := e.workers[workerID]

		lookupKey := balanceLookupKey(balance.OrganizationID, balance.LedgerID, balance.AccountAlias, balanceKey)

		worker.mu.Lock()
		if _, exists := worker.balances[lookupKey]; !exists {
			inserted++
		}

		copyBalance := balance.clone()
		copyBalance.BalanceKey = balanceKey
		worker.balances[lookupKey] = copyBalance
		worker.mu.Unlock()
	}

	e.loaded.Add(inserted)

	return inserted
}

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
	defer worker.mu.RUnlock()

	balance, ok := worker.balances[lookupKey]
	if !ok {
		return nil, false
	}

	return balance.clone(), true
}

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
	normalized    []*authorizerv1.BalanceOperation
	prepared      []preparedOperation
	orderedShards []int
	changedOps    int
}

// Authorize is the fast-path single-instance authorization.
// It prepares and commits in one call (no cross-shard coordination needed).
// For cross-shard transactions, the gRPC service uses PrepareAuthorize + CommitPrepared directly.
func (e *Engine) Authorize(req *authorizerv1.AuthorizeRequest) (*authorizerv1.AuthorizeResponse, error) {
	return e.authorize(req, true)
}

func (e *Engine) authorize(req *authorizerv1.AuthorizeRequest, persistWAL bool) (*authorizerv1.AuthorizeResponse, error) {
	if e == nil || e.router == nil {
		return nil, errors.New("authorizer engine is not initialized")
	}

	if req == nil || len(req.Operations) == 0 {
		return &authorizerv1.AuthorizeResponse{Authorized: true}, nil
	}

	draft, rejection, releaseLocks, err := e.prepareAuthorization(req)
	if err != nil {
		return nil, err
	}

	if rejection != nil {
		releaseLocks()
		return rejection, nil
	}

	defer releaseLocks()

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
			if e.observe != nil {
				e.observe.ObserveWALAppendFailure(err)
			}

			return nil, fmt.Errorf("append wal entry: %w", err)
		}
	}

	for _, op := range draft.prepared {
		if !op.hasChange {
			continue
		}

		op.balance.Available = op.postAvail
		op.balance.OnHold = op.postHold
		op.balance.Version++
	}

	releaseLocks()

	snapshots := buildAuthorizeSnapshots(draft.prepared)

	return &authorizerv1.AuthorizeResponse{
		Authorized: true,
		Balances:   snapshots,
	}, nil
}

func (e *Engine) prepareAuthorization(req *authorizerv1.AuthorizeRequest) (*authorizationDraft, *authorizerv1.AuthorizeResponse, func(), error) {
	normalized := normalizeExternalOperations(req.Operations, e.router)
	organizationID := req.GetOrganizationId()
	ledgerID := req.GetLedgerId()
	pending := req.GetPending()
	transactionStatus := req.GetTransactionStatus()

	shards := make(map[int]struct{}, len(normalized))
	for _, op := range normalized {
		if op == nil {
			continue
		}

		workerID := e.router.ResolveBalance(op.GetAccountAlias(), op.GetBalanceKey())
		shards[workerID] = struct{}{}
	}

	orderedShards := make([]int, 0, len(shards))
	for workerID := range shards {
		orderedShards = append(orderedShards, workerID)
	}
	sort.Ints(orderedShards)

	var lockWaitTotal time.Duration
	for _, workerID := range orderedShards {
		lockStart := time.Now()
		e.workers[workerID].mu.Lock()
		lockWaitTotal += time.Since(lockStart)
	}

	if e.observe != nil {
		e.observe.ObserveAuthorizeLockWait(len(orderedShards), lockWaitTotal)
	}

	lockHoldStart := time.Now()
	locksReleased := false
	releaseLocks := func() {
		if locksReleased {
			return
		}

		if e.observe != nil {
			e.observe.ObserveAuthorizeLockHold(len(orderedShards), time.Since(lockHoldStart))
		}

		for i := len(orderedShards) - 1; i >= 0; i-- {
			e.workers[orderedShards[i]].mu.Unlock()
		}

		locksReleased = true
	}

	prepared := make([]preparedOperation, 0, len(normalized))
	staged := make(map[*Balance]*Balance, len(normalized))
	changedOperations := 0

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

		actualBalance, ok := e.workers[workerID].balances[lookupKey]
		if !ok {
			return nil, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionBalanceNotFound,
				RejectionMessage: "balance not found",
			}, releaseLocks, nil
		}

		workingBalance, ok := staged[actualBalance]
		if !ok {
			workingBalance = actualBalance.clone()
			staged[actualBalance] = workingBalance
		}

		amount, err := rescaleAmount(op.GetAmount(), op.GetScale(), workingBalance.Scale)
		if err != nil {
			return nil, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionInternalError,
				RejectionMessage: err.Error(),
			}, releaseLocks, nil
		}

		preAvail := workingBalance.Available
		preHold := workingBalance.OnHold
		postAvail, postHold := applyOperation(
			preAvail,
			preHold,
			pending,
			transactionStatus,
			op.GetOperation(),
			amount,
		)

		ok, rejectionCode, rejectionMessage := validateBalanceRules(
			workingBalance,
			op,
			preAvail,
			preHold,
			postAvail,
			postHold,
		)
		if !ok {
			return nil, &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    rejectionCode,
				RejectionMessage: rejectionMessage,
			}, releaseLocks, nil
		}

		workingBalance.Available = postAvail
		workingBalance.OnHold = postHold
		hasChange := preAvail != postAvail || preHold != postHold
		if hasChange {
			changedOperations++
		}

		prepared = append(prepared, preparedOperation{
			lookupKey:      lookupKey,
			balance:        actualBalance,
			operationAlias: op.GetOperationAlias(),
			canonicalAlias: canonicalAlias,
			canonicalKey:   balanceKey,
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

	return &authorizationDraft{
		normalized:    normalized,
		prepared:      prepared,
		orderedShards: orderedShards,
		changedOps:    changedOperations,
	}, nil, releaseLocks, nil
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

func (e *Engine) ReplayEntries(entries []wal.Entry) error {
	if e == nil || e.router == nil {
		return errors.New("authorizer engine is not initialized")
	}

	for _, entry := range entries {
		if len(entry.Mutations) == 0 {
			continue
		}

		shardSet := make(map[int]struct{}, len(entry.Mutations))
		for _, mutation := range entry.Mutations {
			shardID := e.router.ResolveBalance(mutation.AccountAlias, mutation.BalanceKey)
			shardSet[shardID] = struct{}{}
		}

		orderedShards := make([]int, 0, len(shardSet))
		for shardID := range shardSet {
			orderedShards = append(orderedShards, shardID)
		}
		sort.Ints(orderedShards)

		for _, shardID := range orderedShards {
			e.workers[shardID].mu.Lock()
		}

		skipEntry := false
		for _, mutation := range entry.Mutations {
			balanceKey := mutation.BalanceKey
			if balanceKey == "" {
				balanceKey = constant.DefaultBalanceKey
			}

			workerID := e.router.ResolveBalance(mutation.AccountAlias, balanceKey)
			lookupKey := balanceLookupKey(entry.OrganizationID, entry.LedgerID, mutation.AccountAlias, balanceKey)
			balance, ok := e.workers[workerID].balances[lookupKey]
			if !ok {
				skipEntry = true
				break
			}

			if balance.Version == mutation.NextVersion && balance.Available == mutation.Available && balance.OnHold == mutation.OnHold {
				continue
			}

			if balance.Version != mutation.PreviousVersion {
				skipEntry = true
				break
			}
		}

		if !skipEntry {
			for _, mutation := range entry.Mutations {
				balanceKey := mutation.BalanceKey
				if balanceKey == "" {
					balanceKey = constant.DefaultBalanceKey
				}

				workerID := e.router.ResolveBalance(mutation.AccountAlias, balanceKey)
				lookupKey := balanceLookupKey(entry.OrganizationID, entry.LedgerID, mutation.AccountAlias, balanceKey)
				balance := e.workers[workerID].balances[lookupKey]

				if balance.Version == mutation.NextVersion && balance.Available == mutation.Available && balance.OnHold == mutation.OnHold {
					continue
				}

				balance.Available = mutation.Available
				balance.OnHold = mutation.OnHold
				balance.Version = mutation.NextVersion
			}
		}

		for i := len(orderedShards) - 1; i >= 0; i-- {
			e.workers[orderedShards[i]].mu.Unlock()
		}
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
	for i, op := range ops {
		if op == nil {
			continue
		}

		copyOp := *op

		if shard.IsExternal(copyOp.GetAccountAlias()) && !shard.IsExternalBalanceKey(copyOp.GetBalanceKey()) {
			counterparty := nonExternalAliases[0]
			if len(nonExternalAliases) > i {
				counterparty = nonExternalAliases[i]
			}

			copyOp.BalanceKey = router.ResolveExternalBalanceKey(counterparty)
		}

		resolved = append(resolved, &copyOp)
	}

	return resolved
}

func applyOperation(available, onHold int64, pending bool, transactionStatus, operation string, amount int64) (int64, int64) {
	if amount == 0 {
		return available, onHold
	}

	op := strings.ToUpper(operation)
	status := strings.ToUpper(transactionStatus)

	if pending {
		switch {
		case op == constant.ONHOLD && status == constant.PENDING:
			available -= amount
			onHold += amount
		case op == constant.RELEASE && status == constant.CANCELED:
			onHold -= amount
			available += amount
		case status == "APPROVED_COMPENSATE":
			switch op {
			case constant.DEBIT:
				onHold += amount
			case constant.CREDIT:
				available -= amount
			case constant.RELEASE:
				onHold += amount
				available -= amount
			case constant.ONHOLD:
				onHold -= amount
				available += amount
			}
		case status == constant.APPROVED:
			switch op {
			case constant.DEBIT:
				onHold -= amount
			case constant.RELEASE:
				onHold -= amount
				available += amount
			default:
				available += amount
			}
		}

		return available, onHold
	}

	if op == constant.DEBIT {
		available -= amount
	} else {
		available += amount
	}

	return available, onHold
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

// PrepareAuthorize validates operations and holds shard locks without committing.
// The returned PreparedTx handle must be completed via CommitPrepared or AbortPrepared.
// If validation fails, locks are released immediately and no handle is returned.
func (e *Engine) PrepareAuthorize(req *authorizerv1.AuthorizeRequest) (*PreparedTx, *authorizerv1.AuthorizeResponse, error) {
	if e == nil || e.router == nil {
		return nil, nil, errors.New("authorizer engine is not initialized")
	}

	if req == nil || len(req.Operations) == 0 {
		return nil, &authorizerv1.AuthorizeResponse{Authorized: true}, nil
	}

	draft, rejection, releaseLocks, err := e.prepareAuthorization(req)
	if err != nil {
		return nil, nil, err
	}

	if rejection != nil {
		releaseLocks()
		return nil, rejection, nil
	}

	// Validation passed. Build the PreparedTx handle and hold locks.
	txID := "ptx-" + uuid.NewString()

	ptx := &PreparedTx{
		ID:            txID,
		Request:       req,
		normalized:    draft.normalized,
		prepared:      draft.prepared,
		changedOps:    draft.changedOps,
		orderedShards: draft.orderedShards,
		createdAt:     time.Now(),
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
		return nil, fmt.Errorf("commit prepared %s: prepared transaction is nil", txID)
	}

	lockHoldStart := ptx.createdAt
	releaseLocks := true

	defer func() {
		if !releaseLocks {
			return
		}

		if e.observe != nil {
			e.observe.ObserveAuthorizeLockHold(len(ptx.orderedShards), time.Since(lockHoldStart))
		}

		for i := len(ptx.orderedShards) - 1; i >= 0; i-- {
			e.workers[ptx.orderedShards[i]].mu.Unlock()
		}
	}()

	// Write WAL entry.
	if ptx.changedOps > 0 {
		mutations := buildWALMutations(ptx.prepared)

		err := e.wal.Append(wal.Entry{
			TransactionID:     ptx.Request.GetTransactionId(),
			OrganizationID:    ptx.Request.GetOrganizationId(),
			LedgerID:          ptx.Request.GetLedgerId(),
			Pending:           ptx.Request.GetPending(),
			TransactionStatus: ptx.Request.GetTransactionStatus(),
			Operations:        ptx.normalized,
			Mutations:         mutations,
		})
		if err != nil {
			if e.observe != nil {
				e.observe.ObserveWALAppendFailure(err)
			}

			if putBackErr := e.prepStore.PutBack(ptx); putBackErr != nil {
				return nil, fmt.Errorf(
					"commit prepared %s: WAL append failed: %w (also failed to preserve prepared state: %v)",
					txID,
					err,
					putBackErr,
				)
			}

			releaseLocks = false

			return nil, fmt.Errorf("commit prepared %s: WAL append failed: %w", txID, err)
		}
	}

	// Mutate live balances.
	for _, op := range ptx.prepared {
		if !op.hasChange {
			continue
		}

		op.balance.Available = op.postAvail
		op.balance.OnHold = op.postHold
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

	if e.observe != nil {
		e.observe.ObserveAuthorizeLockHold(len(ptx.orderedShards), time.Since(ptx.createdAt))
	}

	for i := len(ptx.orderedShards) - 1; i >= 0; i-- {
		e.workers[ptx.orderedShards[i]].mu.Unlock()
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
				if e.observe != nil {
					e.observe.ObserveAuthorizeLockHold(len(ptx.orderedShards), time.Since(ptx.createdAt))
				}

				for i := len(ptx.orderedShards) - 1; i >= 0; i-- {
					e.workers[ptx.orderedShards[i]].mu.Unlock()
				}
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
		return nil
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
