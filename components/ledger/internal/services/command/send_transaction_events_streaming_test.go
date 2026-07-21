// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// transactionLifecycleFixture builds a Transaction with one operation
// attached, status APPROVED, and an Amount populated. The fixture is
// reused across the four lifecycle scenarios.
func transactionLifecycleFixture(parentID *string, status string) *transaction.Transaction {
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	tranID := uuid.New().String()
	amount := decimal.NewFromInt(1500)
	statusCode := status

	op := &operation.Operation{
		ID:             uuid.New().String(),
		TransactionID:  tranID,
		AccountID:      uuid.New().String(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AssetCode:      "USD",
		Direction:      constant.DirectionDebit,
		Type:           "DEBIT",
	}

	return &transaction.Transaction{
		ID:                       tranID,
		ParentTransactionID:      parentID,
		OrganizationID:           orgID,
		LedgerID:                 ledgerID,
		Description:              "lifecycle fixture",
		Status:                   transaction.Status{Code: statusCode, Description: &statusCode},
		Amount:                   &amount,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "default",
		Source:                   []string{"@external/cash"},
		Destination:              []string{"@person1"},
		Route:                    "default-route",
		Operations:               []*operation.Operation{op},
	}
}

// newSendTransactionEventsTestUseCase wires a UseCase whose RabbitMQRepo
// accepts the legacy publish (returning nil/nil) and whose Streaming is
// the injected emitter. RABBITMQ_TRANSACTION_EVENTS_ENABLED is left at
// its enabled default so both transports are exercised.
func newSendTransactionEventsTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "")
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE", "test-transaction-exchange")

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

// TestSendTransactionEvents_PhaseCreatedNoParentEmitsPosted locks the
// posted-vs-reverted discrimination: phase=created + nil parent must
// fire transaction.posted, never transaction.reverted.
func TestSendTransactionEvents_PhaseCreatedNoParentEmitsPosted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "phase=created with nil parent must emit exactly one lib-streaming event")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "posted")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, tran.ID, payload["id"])
	assert.NotContains(t, payload, "parentTransactionId", "posted must omit parentTransactionId")
}

// TestSendTransactionEvents_PhaseCreatedWithParentEmitsReverted locks
// the inverse: phase=created + non-nil parent fires transaction.reverted.
func TestSendTransactionEvents_PhaseCreatedWithParentEmitsReverted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	parentID := uuid.New().String()
	tran := transactionLifecycleFixture(&parentID, constant.APPROVED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "reverted")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, tran.ID, payload["id"])
	assert.Equal(t, parentID, payload["parentTransactionId"], "reverted must populate parentTransactionId")
}

// TestSendTransactionEvents_PhaseUpdatedApprovedEmitsCommitted locks
// phase=updated + APPROVED → transaction.committed (idempotency-branch
// commit path).
func TestSendTransactionEvents_PhaseUpdatedApprovedEmitsCommitted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseUpdated)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "committed")
}

// TestSendTransactionEvents_PhaseUpdatedCanceledEmitsCanceled locks
// phase=updated + CANCELED → transaction.canceled.
func TestSendTransactionEvents_PhaseUpdatedCanceledEmitsCanceled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.CANCELED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseUpdated)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "canceled")
}

// TestSendTransactionEvents_PhaseCreatedPendingSkipsLibStreaming locks
// the scope-fence contract: PENDING transactions on the fresh-insert
// path do NOT emit transaction.posted. PENDING is a pre-commit state;
// the broadcast happens later via transaction.committed or
// transaction.canceled.
func TestSendTransactionEvents_PhaseCreatedPendingSkipsLibStreaming(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.PENDING)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	assert.Empty(t, mockEmitter.Events(),
		"PENDING transactions on fresh-insert path must not emit transaction.posted; "+
			"the broadcast fires later via transaction.committed or transaction.canceled")
}

// TestSendTransactionEvents_PhaseCreatedNotedSkipsLibStreaming locks
// the scope contract for NOTED transactions. NOTED is annotation-only
// (no balance impact, no operations) and is not a broadcastable
// business fact. The fresh-insert path must skip emission entirely for
// NOTED status.
func TestSendTransactionEvents_PhaseCreatedNotedSkipsLibStreaming(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.NOTED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	assert.Empty(t, mockEmitter.Events(),
		"NOTED transactions must not emit transaction.posted; "+
			"the catalog scope fence excludes annotation-only facts")
}

// TestSendTransactionEvents_PhaseNoopSkipsLibStreaming locks the
// noop-phase contract: when CreateOrUpdateTransaction observed no state
// change (e.g. ineligible unique violation), lib-streaming emits
// nothing. The legacy rabbit publish still fires because the
// RABBITMQ_TRANSACTION_EVENTS_ENABLED flag controls only the
// transports, not the phase gating.
func TestSendTransactionEvents_PhaseNoopSkipsLibStreaming(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseNoop)

	assert.Empty(t, mockEmitter.Events(), "noop phase must not emit any lib-streaming event")
}

// TestSendTransactionEvents_DisabledFlagSkipsBothTransports asserts the
// cutover-window flag short-circuits BOTH legacy rabbit AND
// lib-streaming. This mirrors the SendOverdraftEvents contract — the
// disabled flag is a single switch that operators can flip during
// incidents without leaving the events flowing through one transport.
func TestSendTransactionEvents_DisabledFlagSkipsBothTransports(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()

	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")

	// No rabbit expectations: the disabled flag must skip the producer entirely.
	mockRabbit := rabbitmq.NewMockProducerRepository(ctrl)

	uc := &UseCase{
		RabbitMQRepo: mockRabbit,
		Streaming:    mockEmitter,
	}

	uc.SendTransactionEvents(context.Background(),
		transactionLifecycleFixture(nil, constant.APPROVED),
		TransactionLifecyclePhaseCreated)

	assert.Empty(t, mockEmitter.Events(), "disabled flag must short-circuit lib-streaming emission")
}

// TestSendTransactionEvents_EmitFailureDoesNotCrash exercises the
// IMPORTANT-posture safety: a failing lib-streaming emitter must not
// fail the request. The fixture transitions through the helper without
// panicking; the legacy rabbit publish still runs.
func TestSendTransactionEvents_EmitFailureDoesNotCrash(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newSendTransactionEventsTestUseCase(t, ctrl, streamingFailingEmitter{})

	// Should complete without panicking.
	uc.SendTransactionEvents(context.Background(),
		transactionLifecycleFixture(nil, constant.APPROVED),
		TransactionLifecyclePhaseCreated)
}

// TestSendTransactionEvents_NilStreamingIsAllowed asserts that a
// UseCase with no Streaming wired (nil emitter) completes the legacy
// path without panicking — the IMPORTANT-posture contract treats nil
// as "streaming disabled" and skips lib-streaming silently.
func TestSendTransactionEvents_NilStreamingIsAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "")
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE", "test-transaction-exchange")

	mockRabbit := rabbitmq.NewMockProducerRepository(ctrl)
	mockRabbit.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return((*string)(nil), nil).
		Times(1)

	uc := &UseCase{
		RabbitMQRepo: mockRabbit,
		Streaming:    nil,
	}

	uc.SendTransactionEvents(context.Background(),
		transactionLifecycleFixture(nil, constant.APPROVED),
		TransactionLifecyclePhaseCreated)
}

// TestSendTransactionEvents_PayloadCarriesOperations confirms the
// operations array makes it onto the wire. The events package uses
// json.RawMessage for operations so the per-operation marshaling
// happens inside buildTransactionEventSource — this test locks the
// wire shape.
func TestSendTransactionEvents_PayloadCarriesOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))

	operations, ok := payload["operations"].([]any)
	require.True(t, ok, "operations must be a JSON array on the wire")
	require.Len(t, operations, 1)

	op, ok := operations[0].(map[string]any)
	require.True(t, ok, "operations[0] must be a JSON object on the wire")
	assert.Equal(t, tran.Operations[0].ID, op["id"])
	assert.Equal(t, "USD", op["assetCode"])
}
