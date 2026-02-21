// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
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
	mu       sync.Mutex
	balances map[string]*Balance
}

// Engine provides in-memory transaction authorization with shard-ordered locking.
type Engine struct {
	router  *shard.Router
	workers []*shardWorker
	wal     wal.Writer
	loaded  atomic.Int64
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

	return &Engine{router: router, workers: workers, wal: walWriter}
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

	worker.mu.Lock()
	defer worker.mu.Unlock()

	balance, ok := worker.balances[lookupKey]
	if !ok {
		return nil, false
	}

	return balance.clone(), true
}

type preparedOperation struct {
	op              *authorizerv1.BalanceOperation
	lookupKey       string
	workerID        int
	balance         *Balance
	preAvail        int64
	preHold         int64
	postAvail       int64
	postHold        int64
	preVersion      uint64
	hasChange       bool
	canonicalAlias  string
	canonicalKey    string
	transactionOp   string
	transactionType string
	amount          int64
}

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

	normalized := normalizeExternalOperations(req.Operations, e.router)

	shards := make(map[int]struct{}, len(normalized))
	for _, op := range normalized {
		workerID := e.router.ResolveBalance(op.GetAccountAlias(), op.GetBalanceKey())
		shards[workerID] = struct{}{}
	}

	orderedShards := make([]int, 0, len(shards))
	for workerID := range shards {
		orderedShards = append(orderedShards, workerID)
	}
	sort.Ints(orderedShards)

	for _, workerID := range orderedShards {
		e.workers[workerID].mu.Lock()
	}
	defer func() {
		for i := len(orderedShards) - 1; i >= 0; i-- {
			e.workers[orderedShards[i]].mu.Unlock()
		}
	}()

	prepared := make([]preparedOperation, 0, len(normalized))
	staged := make(map[string]*Balance, len(normalized))

	for _, op := range normalized {
		balanceKey := op.GetBalanceKey()
		if balanceKey == "" {
			balanceKey = constant.DefaultBalanceKey
		}

		canonicalAlias := op.GetAccountAlias()
		workerID := e.router.ResolveBalance(canonicalAlias, balanceKey)
		lookupKey := balanceLookupKey(req.GetOrganizationId(), req.GetLedgerId(), canonicalAlias, balanceKey)

		actualBalance, ok := e.workers[workerID].balances[lookupKey]
		if !ok {
			return &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionBalanceNotFound,
				RejectionMessage: "balance not found",
			}, nil
		}

		workingBalance, ok := staged[lookupKey]
		if !ok {
			workingBalance = actualBalance.clone()
			staged[lookupKey] = workingBalance
		}

		amount, err := rescaleAmount(op.GetAmount(), op.GetScale(), workingBalance.Scale)
		if err != nil {
			return &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    RejectionInternalError,
				RejectionMessage: err.Error(),
			}, nil
		}

		preAvail := workingBalance.Available
		preHold := workingBalance.OnHold
		postAvail, postHold := applyOperation(
			preAvail,
			preHold,
			req.GetPending(),
			req.GetTransactionStatus(),
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
			return &authorizerv1.AuthorizeResponse{
				Authorized:       false,
				RejectionCode:    rejectionCode,
				RejectionMessage: rejectionMessage,
			}, nil
		}

		workingBalance.Available = postAvail
		workingBalance.OnHold = postHold

		prepared = append(prepared, preparedOperation{
			op:             op,
			lookupKey:      lookupKey,
			workerID:       workerID,
			balance:        actualBalance,
			preVersion:     actualBalance.Version,
			preAvail:       preAvail,
			preHold:        preHold,
			postAvail:      postAvail,
			postHold:       postHold,
			hasChange:      preAvail != postAvail || preHold != postHold,
			canonicalAlias: canonicalAlias,
			canonicalKey:   balanceKey,
			amount:         amount,
		})
	}

	snapshots := make([]*authorizerv1.BalanceSnapshot, 0, len(prepared))

	for _, op := range prepared {
		if !op.hasChange {
			continue
		}

		snapshots = append(snapshots, &authorizerv1.BalanceSnapshot{
			OperationAlias: op.op.GetOperationAlias(),
			AccountAlias:   op.canonicalAlias,
			BalanceKey:     op.canonicalKey,
			BalanceId:      op.balance.ID,
			AccountId:      op.balance.AccountID,
			AssetCode:      op.balance.AssetCode,
			AccountType:    op.balance.AccountType,
			AllowSending:   op.balance.AllowSending,
			AllowReceiving: op.balance.AllowReceiving,
			Available:      op.preAvail,
			OnHold:         op.preHold,
			Scale:          op.balance.Scale,
			Version:        op.balance.Version,
		})

		op.balance.Available = op.postAvail
		op.balance.OnHold = op.postHold
		op.balance.Version++
	}

	if persistWAL && len(snapshots) > 0 {
		mutations := buildWALMutations(prepared)

		_ = e.wal.Append(wal.Entry{
			TransactionID:     req.GetTransactionId(),
			OrganizationID:    req.GetOrganizationId(),
			LedgerID:          req.GetLedgerId(),
			Pending:           req.GetPending(),
			TransactionStatus: req.GetTransactionStatus(),
			Operations:        normalized,
			Mutations:         mutations,
		})
	}

	return &authorizerv1.AuthorizeResponse{
		Authorized: true,
		Balances:   snapshots,
	}, nil
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
