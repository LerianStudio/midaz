// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
)

// Both post-commit emitter fan-outs spawn a detached goroutine that declares a
// WaitGroup count and then blocks in wg.Wait() until that many wg.Done() calls
// arrive. The count and the number of spawned emitter goroutines are two numbers
// that must agree, and NOTHING about the surrounding code makes them agree: they
// are edited independently, one line apart.
//
// An over-count (wg.Add(N) with N-1 goroutines) parks the fan-out goroutine in
// wg.Wait() FOREVER. Because that goroutine is detached, the request still
// returns, every assertion on the returned value still holds, and the whole
// package's test suite still passes green while one goroutine is stranded per
// committed transaction — a leak that only shows up as unbounded goroutine
// growth under production traffic.
//
// The two helpers below close that hole by observing the fan-out goroutine
// itself: the emitters are unexported and detached, so the only honest way to
// assert the fan-out DRAINED is to look for its stack frame parked on a
// WaitGroup after the call returns.

// fanOutGoroutineParked reports whether any live goroutine is blocked in
// sync.WaitGroup.Wait beneath fnName. Matching on BOTH frames is what keeps this
// precise: it cannot be tripped by an unrelated WaitGroup elsewhere in the
// package, nor by another test's goroutines running in parallel.
func fanOutGoroutineParked(fnName string) bool {
	buf := make([]byte, 1<<20)

	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]

			break
		}

		buf = make([]byte, 2*len(buf))
	}

	// runtime.Stack separates goroutines with a blank line; inspect each
	// independently so the two frames must co-occur in ONE goroutine.
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "sync.(*WaitGroup).Wait") && strings.Contains(g, fnName) {
			return true
		}
	}

	return false
}

// requireFanOutDrains fails the test if the fan-out goroutine beneath fnName is
// still parked on its WaitGroup once it has had ample time to finish.
//
// The settle window is load-bearing and MUST come before the first assertion.
// Checking immediately and passing on the first "not parked" reading would let
// the test pass by observing the goroutine BEFORE it started — the emitters are
// spawned asynchronously, so "absent" at t=0 means nothing. A correct fan-out
// drains in microseconds and is long gone by the time the window elapses; a
// miscounted one is parked forever, so it is still there at every later reading.
func requireFanOutDrains(t *testing.T, fnName string) {
	t.Helper()

	const settle = 500 * time.Millisecond

	time.Sleep(settle)

	// Past the settle window a correct fan-out has completed. Keep re-checking
	// for a while anyway so a heavily loaded CI box cannot fail this on timing
	// alone; only a goroutine that never drains reaches the Fatalf.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if !fanOutGoroutineParked(fnName) {
			return
		}

		if time.Now().After(deadline) {
			break
		}

		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("emitter fan-out goroutine in %s is still parked in sync.WaitGroup.Wait: "+
		"the wg.Add(N) count does not match the number of emitter goroutines spawned, "+
		"so the fan-out goroutine leaks on every committed transaction", fnName)
}

// TestCreateBalanceTransactionOperationsAsync_EmitterFanOutDrains pins the
// wg.Add count in the single-transaction post-commit fan-out against the number
// of emitter goroutines it actually starts.
func TestCreateBalanceTransactionOperationsAsync_EmitterFanOutDrains(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
		BalanceRepo:             mockBalanceRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Status:         transaction.Status{Code: constant.APPROVED},
		Operations:     []*operation.Operation{},
		Metadata:       map[string]any{},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    &mtransaction.Responses{Aliases: []string{"alias1"}},
		Balances: []*mmodel.Balance{
			{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)},
		},
		Input:   &mtransaction.Transaction{},
		Version: "v2",
	}

	payloadBytes, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	queue := mmodel.Queue{
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: payloadBytes}},
	}

	mockTransactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(tran, nil).AnyTimes()
	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()
	mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	require.NoError(t, uc.CreateBalanceTransactionOperationsAsync(context.Background(), queue))

	requireFanOutDrains(t, "CreateBalanceTransactionOperationsAsync")
}

// TestCreateBulkTransactionOperationsAsync_EmitterFanOutDrains pins the same
// invariant on the bulk path, which carries its own independent copy of the
// wg.Add / goroutine pair.
func TestCreateBulkTransactionOperationsAsync_EmitterFanOutDrains(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	tx := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Status:         transaction.Status{Code: constant.APPROVED},
		Operations: []*operation.Operation{
			{ID: uuid.New().String(), TransactionID: transactionID},
		},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction:   tx,
		Validate:      &mtransaction.Responses{Aliases: []string{"alias1"}},
		Balances:      []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)}},
		BalancesAfter: []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(50)}},
		Version:       "v2",
	}

	mockTx := &mockDBTransaction{}
	mockTransactionRepo.EXPECT().BeginTx(gomock.Any()).Return(mockTx, nil).Times(1)
	mockTransactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{Attempted: 1, Inserted: 1}, nil).Times(1)
	mockOperationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{Attempted: 1, Inserted: 1}, nil).Times(1)
	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()
	mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	_, err := uc.CreateBulkTransactionOperationsAsync(
		context.Background(), []transaction.TransactionProcessingPayload{payload},
	)
	require.NoError(t, err)

	requireFanOutDrains(t, "processMetadataAndEvents")
}
