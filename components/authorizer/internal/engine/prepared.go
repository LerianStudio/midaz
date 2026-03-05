// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/wal"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

const (
	// DefaultPrepareTimeout is how long a PreparedTx holds locks before auto-abort.
	DefaultPrepareTimeout = 30 * time.Second

	// DefaultCommittedRetention is how long committed prepared transactions are
	// retained for idempotent CommitPrepared retries.
	DefaultCommittedRetention = 24 * time.Hour

	// DefaultPrepareCommitRetryLimit is the maximum number of WAL append retry
	// cycles allowed for a prepared transaction before the lock is force-released.
	DefaultPrepareCommitRetryLimit = 3

	// DefaultMaxPreparedTx caps the number of concurrently prepared transactions.
	// This bounds lock and memory pressure under malicious or unhealthy traffic.
	DefaultMaxPreparedTx = 10_000

	// preparedCleanupInterval is how often the reaper goroutine checks for expired transactions.
	preparedCleanupInterval = 500 * time.Millisecond
)

// ErrPreparedTxNotFound indicates that the prepared transaction was not found in the store.
var (
	ErrPreparedTxNotFound         = errors.New("prepared transaction not found")
	ErrPreparedTxAlreadyCommitted = errors.New("prepared transaction already committed")
	ErrPreparedTxCommitDecided    = errors.New("prepared transaction commit already decided")
)

// PreparedTx represents a transaction that has been validated and has locks held,
// but has not yet been committed. It is the intermediate state in the 2PC protocol
// used for cross-shard authorizations between authorizer instances.
type PreparedTx struct {
	// ID is the unique handle for this prepared transaction.
	ID string

	// Request is the original AuthorizeRequest (needed for WAL entry construction).
	Request *authorizerv1.AuthorizeRequest

	// Normalized operations after external account rewriting.
	normalized []*authorizerv1.BalanceOperation

	// prepared holds the validated operation results (pre/post balances, changes).
	prepared []preparedOperation

	// changedOps is the number of operations that actually mutate balance state.
	changedOps int

	// lockedBalances holds the per-balance locks acquired during Prepare, in
	// deterministic (sorted lookup-key) order. These MUST be released by
	// CommitPrepared or AbortPrepared. Replaces the previous orderedShards
	// which locked entire shards containing many unrelated accounts.
	lockedBalances []*Balance

	// lockedCount tracks how many unique balances are locked (for observability).
	lockedCount int

	// lockedShardCount tracks how many shards those locked balances span.
	lockedShardCount int

	// createdAt is when the prepare was issued. Used for timeout-based auto-abort.
	createdAt time.Time

	// commitDecided is true once CommitPrepared has been attempted at least once.
	// Commit-decided prepared transactions MUST NOT be auto-aborted by timeout.
	commitDecided bool

	// done is true once CommitPrepared or AbortPrepared has been called.
	done bool

	// commitAttempts counts WAL append failures followed by PutBack cycles.
	commitAttempts int

	// CrossShard indicates this prepared transaction is part of a multi-instance 2PC.
	CrossShard bool

	// Participants lists all authorizer instances involved in this cross-shard transaction.
	Participants []wal.WALParticipant
}

type committedPreparedTx struct {
	response    *authorizerv1.AuthorizeResponse
	committedAt time.Time
}

// preparedTxStore is a thread-safe store for pending prepared transactions.
// The auto-abort goroutine and the commit/abort methods all access this concurrently.
type preparedTxStore struct {
	mu           sync.Mutex
	pending      map[string]*PreparedTx
	committed    map[string]committedPreparedTx
	timeout      time.Duration
	max          int
	committedTTL time.Duration
	maxRetries   int
}

func newPreparedTxStore(timeout time.Duration, maxPending int) *preparedTxStore {
	if timeout <= 0 {
		timeout = DefaultPrepareTimeout
	}

	if maxPending <= 0 {
		maxPending = DefaultMaxPreparedTx
	}

	return &preparedTxStore{
		pending:      make(map[string]*PreparedTx),
		committed:    make(map[string]committedPreparedTx),
		timeout:      timeout,
		max:          maxPending,
		committedTTL: DefaultCommittedRetention,
		maxRetries:   DefaultPrepareCommitRetryLimit,
	}
}

// Put stores a prepared transaction. Returns an error if the ID already exists.
func (s *preparedTxStore) Put(ptx *PreparedTx) error {
	if s == nil {
		return fmt.Errorf("%w", constant.ErrPreparedTxStoreNil)
	}

	if ptx == nil {
		return fmt.Errorf("%w", constant.ErrPreparedTransactionNil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pending) >= s.max {
		return fmt.Errorf("%w: max=%d", constant.ErrPreparedTxCapacityExceeded, s.max)
	}

	if _, exists := s.pending[ptx.ID]; exists {
		return fmt.Errorf("%w: %s", constant.ErrPreparedTxAlreadyExists, ptx.ID)
	}

	ptx.done = false
	ptx.commitDecided = false
	ptx.commitAttempts = 0
	s.pending[ptx.ID] = ptx

	return nil
}

// PutBack re-enqueues a transaction that was Taken but could not be committed.
// This preserves the prepared state for coordinator retry/abort.
func (s *preparedTxStore) PutBack(ptx *PreparedTx) error {
	if s == nil {
		return fmt.Errorf("%w", constant.ErrPreparedTxStoreNil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if ptx == nil {
		return fmt.Errorf("%w", constant.ErrPreparedTransactionNil)
	}

	if len(s.pending) >= s.max {
		return fmt.Errorf("%w: max=%d", constant.ErrPreparedTxCapacityExceeded, s.max)
	}

	if _, exists := s.pending[ptx.ID]; exists {
		return fmt.Errorf("%w: %s", constant.ErrPreparedTxAlreadyExists, ptx.ID)
	}

	ptx.commitAttempts++

	if s.maxRetries > 0 && ptx.commitAttempts >= s.maxRetries {
		return fmt.Errorf("%w: %s (limit=%d)", constant.ErrPreparedTxRetryLimitExceeded, ptx.ID, s.maxRetries)
	}

	ptx.done = false
	ptx.createdAt = time.Now()
	s.pending[ptx.ID] = ptx

	return nil
}

// TakeForCommit removes and returns a prepared transaction for commit.
// If the transaction was already committed, it returns the committed response for idempotent replay.
// The boolean return indicates whether the transaction ID exists in either pending or committed state.
func (s *preparedTxStore) TakeForCommit(id string) (*PreparedTx, *authorizerv1.AuthorizeResponse, bool) {
	if s == nil {
		return nil, nil, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredCommittedLocked(time.Now())

	if committed, ok := s.committed[id]; ok {
		return nil, cloneAuthorizeResponse(committed.response), true
	}

	ptx, ok := s.pending[id]
	if !ok || ptx.done {
		return nil, nil, false
	}

	ptx.commitDecided = true
	ptx.done = true

	delete(s.pending, id)

	return ptx, nil, true
}

// TakeForAbort removes and returns a prepared transaction for abort.
func (s *preparedTxStore) TakeForAbort(id string) (*PreparedTx, error) {
	if s == nil {
		return nil, ErrPreparedTxNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredCommittedLocked(time.Now())

	if _, committed := s.committed[id]; committed {
		return nil, ErrPreparedTxAlreadyCommitted
	}

	ptx, ok := s.pending[id]
	if !ok || ptx.done {
		return nil, ErrPreparedTxNotFound
	}

	if ptx.commitDecided {
		return nil, ErrPreparedTxCommitDecided
	}

	ptx.done = true

	delete(s.pending, id)

	return ptx, nil
}

// MarkCommitted stores a committed response for idempotent replay handling.
func (s *preparedTxStore) MarkCommitted(id string, resp *authorizerv1.AuthorizeResponse) {
	if s == nil {
		return
	}

	if id == "" || resp == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredCommittedLocked(time.Now())

	s.committed[id] = committedPreparedTx{
		response:    cloneAuthorizeResponse(resp),
		committedAt: time.Now(),
	}
}

// Expired returns all prepared transactions that have exceeded the timeout.
// The returned transactions are removed from the store and marked done.
func (s *preparedTxStore) Expired() []*PreparedTx {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.pruneExpiredCommittedLocked(now)

	var expired []*PreparedTx

	for id, ptx := range s.pending {
		if ptx.done {
			delete(s.pending, id)
			continue
		}

		if ptx.commitDecided {
			continue
		}

		if now.Sub(ptx.createdAt) > s.timeout {
			ptx.done = true

			delete(s.pending, id)

			expired = append(expired, ptx)
		}
	}

	return expired
}

// Len returns the number of pending prepared transactions.
func (s *preparedTxStore) Len() int {
	if s == nil {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.pending)
}

func (s *preparedTxStore) pruneExpiredCommittedLocked(now time.Time) {
	if s == nil {
		return
	}

	if s.committedTTL <= 0 {
		return
	}

	for id, committed := range s.committed {
		if now.Sub(committed.committedAt) > s.committedTTL {
			delete(s.committed, id)
		}
	}
}

func cloneAuthorizeResponse(resp *authorizerv1.AuthorizeResponse) *authorizerv1.AuthorizeResponse {
	if resp == nil {
		return nil
	}

	cloned, ok := proto.Clone(resp).(*authorizerv1.AuthorizeResponse)
	if !ok {
		return nil
	}

	return cloned
}
