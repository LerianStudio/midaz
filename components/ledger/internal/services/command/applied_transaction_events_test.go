// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// appliedEventsUseCase wires a streaming recorder and a RabbitMQ producer that
// counts the legacy overdraft publishes, so a test can observe every channel the
// applied-transaction fan-out writes to.
func appliedEventsUseCase(t *testing.T) (*UseCase, *pkgStreaming.MockEmitter, *atomic.Int64) {
	t.Helper()

	t.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "")
	t.Setenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE", "test-overdraft-exchange")

	var rabbitPublishes atomic.Int64

	producer := rabbitmq.NewMockProducerRepository(gomock.NewController(t))
	producer.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, string, string, []byte) (*string, error) {
			rabbitPublishes.Add(1)
			return nil, nil
		}).
		AnyTimes()

	recorder := pkgStreaming.NewMockEmitter()

	return &UseCase{RabbitMQRepo: producer, Streaming: recorder}, recorder, &rabbitPublishes
}

// balanceAffectingOverdraftTransaction is an approved transaction whose three
// overdraft companion operations each move a balance, so a full fan-out emits
// one balance.changed and one balance.overdraft_* per operation.
func balanceAffectingOverdraftTransaction() *transaction.Transaction {
	tran := overdraftTransactionFixture()
	for _, op := range tran.Operations {
		op.BalanceAffected = true
	}

	approved := constant.APPROVED
	tran.Status = transaction.Status{Code: approved, Description: &approved}

	return tran
}

func countEventsByKey(recorder *pkgStreaming.MockEmitter, key string) int {
	count := 0

	for _, event := range recorder.Events() {
		if event.DefinitionKey == key {
			count++
		}
	}

	return count
}

// A completion that persisted no new operation is a replay of one that already
// published; repeating its balance and overdraft events would hand consumers
// duplicates.
func TestPublishAppliedTransactionEventsEmitsNothingForNoopPhase(t *testing.T) {
	uc, recorder, rabbitPublishes := appliedEventsUseCase(t)

	uc.PublishAppliedTransactionEvents(context.Background(), balanceAffectingOverdraftTransaction(), TransactionLifecyclePhaseNoop)

	assert.Never(t, func() bool {
		return len(recorder.Events()) > 0 || rabbitPublishes.Load() > 0
	}, 300*time.Millisecond, 10*time.Millisecond, "a noop completion must not publish on any channel")
}

func TestPublishAppliedTransactionEventsEmitsBalanceAndOverdraftForPersistingPhases(t *testing.T) {
	for _, phase := range []string{TransactionLifecyclePhaseCreated, TransactionLifecyclePhaseUpdated} {
		t.Run(phase, func(t *testing.T) {
			uc, recorder, rabbitPublishes := appliedEventsUseCase(t)

			uc.PublishAppliedTransactionEvents(context.Background(), balanceAffectingOverdraftTransaction(), phase)

			require.Eventually(t, func() bool {
				return countEventsByKey(recorder, "balance.changed") == 3 &&
					countEventsByKey(recorder, "balance.overdraft_drawn") == 1 &&
					countEventsByKey(recorder, "balance.overdraft_repaid") == 1 &&
					countEventsByKey(recorder, "balance.overdraft_cleared") == 1 &&
					rabbitPublishes.Load() == 3
			}, 5*time.Second, 10*time.Millisecond, "a persisting completion publishes balance and overdraft events")
		})
	}
}
