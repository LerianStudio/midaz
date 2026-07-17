// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// overdraftTransactionFixture wraps three companion overdraft operations
// covering the drawn / repaid / cleared classifier branches. The fixture
// is deliberately self-contained: every operation has BalanceKey =
// "overdraft" so the classifier accepts it, a non-nil Amount.Value, and
// a populated BalanceAfter.Available that lands in the right classifier
// branch.
func overdraftTransactionFixture() *transaction.Transaction {
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	mkOp := func(direction string, amt int64, afterAvail int64) *operation.Operation {
		amount := decimal.NewFromInt(amt)
		after := decimal.NewFromInt(afterAvail)

		return &operation.Operation{
			ID:             uuid.New().String(),
			BalanceID:      uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			AssetCode:      "USD",
			BalanceKey:     constant.OverdraftBalanceKey,
			Direction:      direction,
			Amount: operation.Amount{
				Value: &amount,
			},
			BalanceAfter: operation.Balance{
				Available: &after,
			},
		}
	}

	return &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Operations: []*operation.Operation{
			mkOp(constant.DirectionDebit, 100, 100), // drawn: debit on companion increases usage
			mkOp(constant.DirectionCredit, 30, 70),  // repaid: credit, after still non-zero
			mkOp(constant.DirectionCredit, 70, 0),   // cleared: credit, after reaches zero
		},
	}
}

// newSendOverdraftEventsTestUseCase wires a UseCase whose RabbitMQRepo
// accepts ProducerDefault for the legacy publish (returning the empty
// response and nil error) and whose Streaming is the injected emitter.
//
// Setting RABBITMQ_OVERDRAFT_EVENTS_ENABLED to anything other than
// "false" keeps the feature flag in the enabled state for the test.
func newSendOverdraftEventsTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	// The default value of the env var is "enabled" (unset == enabled).
	// Explicitly clear it for hermetic test runs.
	t.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "")
	t.Setenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE", "test-overdraft-exchange")
	_ = os.Getenv

	mockRabbit := rabbitmq.NewMockProducerRepository(ctrl)
	mockRabbit.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return((*string)(nil), nil).
		AnyTimes()

	return &UseCase{
		RabbitMQRepo: mockRabbit,
		Streaming:    emitter,
	}
}

// TestSendOverdraftEvents_EmitsThreeBalanceOverdraftEvents verifies that
// a transaction with one of each classifier branch fires three
// lib-streaming events (one per branch), each with the correct
// DefinitionKey, Subject, and Action discriminator.
func TestSendOverdraftEvents_EmitsThreeBalanceOverdraftEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendOverdraftEventsTestUseCase(t, ctrl, mockEmitter)

	tran := overdraftTransactionFixture()
	uc.SendOverdraftEvents(context.Background(), tran)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 3, "expected three lib-streaming events (drawn + repaid + cleared)")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "balance", "overdraft-drawn")
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "balance", "overdraft-repaid")
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "balance", "overdraft-cleared")

	for i, expectedAction := range []string{"drawn", "repaid", "cleared"} {
		var payload map[string]any
		require.NoError(t, json.Unmarshal(emitted[i].Payload, &payload))
		assert.Equal(t, expectedAction, payload["action"], "event %d action mismatch", i)
		assert.Equal(t, tran.ID, payload["transactionId"], "event %d transactionId mismatch", i)
		assert.NotEmpty(t, payload["operationId"])
		assert.NotEmpty(t, payload["balanceId"])
		assert.Equal(t, "USD", payload["assetCode"])
		assert.NotEmpty(t, payload["occurredAt"])
	}
}

// TestSendOverdraftEvents_EmptyOperationsEmitsNothing confirms the
// classifier short-circuit: a transaction with no overdraft companion
// operations produces neither rabbit nor lib-streaming events.
func TestSendOverdraftEvents_EmptyOperationsEmitsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendOverdraftEventsTestUseCase(t, ctrl, mockEmitter)

	tran := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		Operations:     []*operation.Operation{},
	}

	uc.SendOverdraftEvents(context.Background(), tran)

	assert.Empty(t, mockEmitter.Events(), "no overdraft companion operations means no lib-streaming events")
}

// TestSendOverdraftEvents_DisabledFlagSkipsBothEmissions confirms that
// setting RABBITMQ_OVERDRAFT_EVENTS_ENABLED=false short-circuits BOTH
// the rabbit publish AND the lib-streaming emission (the legacy feature
// flag controls both transports during the cutover window).
func TestSendOverdraftEvents_DisabledFlagSkipsBothEmissions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()

	t.Setenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", "false")

	// No rabbit expectations: the disabled flag must skip the producer entirely.
	mockRabbit := rabbitmq.NewMockProducerRepository(ctrl)

	uc := &UseCase{
		RabbitMQRepo: mockRabbit,
		Streaming:    mockEmitter,
	}

	uc.SendOverdraftEvents(context.Background(), overdraftTransactionFixture())
	assert.Empty(t, mockEmitter.Events(), "disabled flag must short-circuit lib-streaming emission too")
}

// TestSendOverdraftEvents_EmitFailureDoesNotCrash exercises the
// IMPORTANT posture: a failing lib-streaming emitter must not cause
// SendOverdraftEvents to panic or stop processing remaining operations.
func TestSendOverdraftEvents_EmitFailureDoesNotCrash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newSendOverdraftEventsTestUseCase(t, ctrl, streamingFailingEmitter{})

	// Should complete without panicking.
	uc.SendOverdraftEvents(context.Background(), overdraftTransactionFixture())
}
