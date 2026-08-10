// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// commitOrCancelTransaction acquires the distributed pending-transaction lock (Redis
// SetNX, code 0486 on contention) and separately guards the transaction's status (code
// 0099 when the transaction is no longer PENDING). The ORDER of those two checks is
// load-bearing: because the lock survives the success path for the async write-behind
// window, a second commit/cancel on an already-terminal transaction would fail SetNX
// and mask the true cause (terminal status, 0099) behind a spurious "Transaction Locked"
// (0486) if the lock were checked first. The status precondition must therefore
// short-circuit BEFORE the lock is ever acquired.
//
// These cases exercise only the error-ordering seam: every case returns before the
// heavy state-machine machinery (balances, operations, write), so no Query dependency
// is touched. SetNX is stubbed to report the lock as already held (success=false) so
// that lock-first ordering would surface as 0486.

// stateLockOrderingTransaction builds a minimal persisted transaction addressed by the
// given status code. Valid UUID strings are required because commitOrCancelTransaction
// parses OrganizationID/LedgerID up front with uuid.MustParse.
func stateLockOrderingTransaction(statusCode string) *transaction.Transaction {
	return &transaction.Transaction{
		ID:             "11111111-1111-1111-1111-111111111111",
		OrganizationID: "22222222-2222-2222-2222-222222222222",
		LedgerID:       "33333333-3333-3333-3333-333333333333",
		Status:         transaction.Status{Code: statusCode},
	}
}

// TestCommitOrCancelTransaction_StatusCheckedBeforeLock proves the status precondition
// is evaluated before the Redis lock: a terminal-status request must return 0099 (not
// 0486) even when the lock reports as held, while a genuinely PENDING request under a
// held lock still returns 0486.
func TestCommitOrCancelTransaction_StatusCheckedBeforeLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		statusCode        string
		transactionStatus string
		wantCode          string
	}{
		{
			name:              "terminal approved under held lock returns invalid status",
			statusCode:        constant.APPROVED,
			transactionStatus: constant.APPROVED,
			wantCode:          constant.ErrCommitTransactionNotPending.Error(), // 0099
		},
		{
			name:              "terminal canceled under held lock returns invalid status",
			statusCode:        constant.CANCELED,
			transactionStatus: constant.CANCELED,
			wantCode:          constant.ErrCommitTransactionNotPending.Error(), // 0099
		},
		{
			name:              "pending under held lock returns transaction locked",
			statusCode:        constant.PENDING,
			transactionStatus: constant.APPROVED,
			wantCode:          constant.ErrPendingTransactionLocked.Error(), // 0486
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRedis := redis.NewMockRedisRepository(ctrl)
			// Exact call counts enforce the "no Redis work before status rejection"
			// invariant directly: a terminal-status request (APPROVED/CANCELED) must
			// short-circuit before ever reaching SetNX (0 calls), while the PENDING
			// request reaches the held lock exactly once (1 call).
			if tt.statusCode == constant.PENDING {
				mockRedis.EXPECT().
					SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(false, nil).
					Times(1)
			} else {
				mockRedis.EXPECT().
					SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			}
			// The function returns before owning the lock in every case here
			// (terminal statuses short-circuit before SetNX; PENDING gets
			// SetNX == false), so deleteLockOnError/Del must never run. Times(0)
			// makes an erroneous unlock of another request's lock fail the test.
			mockRedis.EXPECT().
				Del(gomock.Any(), gomock.Any()).
				Times(0)

			handler := &TransactionHandler{
				Command: &command.UseCase{TransactionRedisRepo: mockRedis},
				Query:   &query.UseCase{},
			}

			tran := stateLockOrderingTransaction(tt.statusCode)

			result, err := handler.commitOrCancelTransaction(context.Background(), tran, tt.transactionStatus)

			require.Error(t, err)
			require.Nil(t, result)

			var conflictErr pkg.EntityConflictError
			require.ErrorAs(t, err, &conflictErr)
			assert.Equal(t, tt.wantCode, conflictErr.Code,
				"error code must reflect the status precondition (0099) before the lock contention (0486)")
		})
	}
}

// TestCommitOrCancelTransaction_ContextCanceledBeforeLock proves the request is
// short-circuited on a canceled/deadline-exceeded context BEFORE the Redis SetNX
// network round-trip: with a genuinely PENDING transaction (so the status guard passes)
// and an already-canceled context, commitOrCancelTransaction returns the raw
// context.Canceled and never reaches SetNX.
func TestCommitOrCancelTransaction_ContextCanceledBeforeLock(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := redis.NewMockRedisRepository(ctrl)
	// The canceled-context guard must run before the lock, so SetNX is never reached.
	mockRedis.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	handler := &TransactionHandler{
		Command: &command.UseCase{TransactionRedisRepo: mockRedis},
		Query:   &query.UseCase{},
	}

	// PENDING so the status precondition passes and control reaches the ctx guard.
	tran := stateLockOrderingTransaction(constant.PENDING)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := handler.commitOrCancelTransaction(ctx, tran, constant.APPROVED)

	require.Error(t, err)
	require.Nil(t, result)
	assert.True(t, errors.Is(err, context.Canceled),
		"a canceled context must short-circuit before the Redis lock and return the raw context error")
}
