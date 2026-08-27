// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
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

// newSendTransactionEventsTestUseCase wires a UseCase whose Streaming is
// the injected emitter. Streaming is the only transport SendTransactionEvents
// uses, so the emitter is the sole observable side effect.
func newSendTransactionEventsTestUseCase(t *testing.T, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	return &UseCase{
		Streaming: emitter,
	}
}

// TestSendTransactionEvents_PhaseCreatedNoParentEmitsPosted locks the
// posted-vs-reverted discrimination: phase=created + nil parent must
// fire transaction.posted, never transaction.reverted.
func TestSendTransactionEvents_PhaseCreatedNoParentEmitsPosted(t *testing.T) {
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

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
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

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
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseUpdated)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "committed")
}

// TestSendTransactionEvents_PhaseUpdatedCanceledEmitsCanceled locks
// phase=updated + CANCELED → transaction.canceled.
func TestSendTransactionEvents_PhaseUpdatedCanceledEmitsCanceled(t *testing.T) {
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

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
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

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
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.NOTED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	assert.Empty(t, mockEmitter.Events(),
		"NOTED transactions must not emit transaction.posted; "+
			"the catalog scope fence excludes annotation-only facts")
}

// TestSendTransactionEvents_PhaseNoopSkipsLibStreaming locks the
// noop-phase contract: when CreateOrUpdateTransaction observed no state
// change (e.g. ineligible unique violation), lib-streaming emits
// nothing. Phase gating alone suppresses the event.
func TestSendTransactionEvents_PhaseNoopSkipsLibStreaming(t *testing.T) {
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseNoop)

	assert.Empty(t, mockEmitter.Events(), "noop phase must not emit any lib-streaming event")
}

// TestSendTransactionEvents_AlwaysEmitsStreamingEvent locks the contract
// that a persisted transaction on the fresh-insert path unconditionally
// fires its streaming lifecycle event. Streaming is the only transport,
// so the mock emitter is the sole observable side effect.
func TestSendTransactionEvents_AlwaysEmitsStreamingEvent(t *testing.T) {
	mockEmitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		Streaming: mockEmitter,
	}

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1,
		"streaming lifecycle event must fire unconditionally on the fresh-insert path")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "posted")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(emitted[0].Payload, &payload))
	assert.Equal(t, tran.ID, payload["id"])
}

// TestSendTransactionEvents_EmitFailureDoesNotCrash exercises the
// IMPORTANT-posture safety: a failing lib-streaming emitter must not
// panic or fail the request. Streaming is the only transport, so a build
// or emit failure is swallowed and the fixture completes cleanly.
func TestSendTransactionEvents_EmitFailureDoesNotCrash(t *testing.T) {
	uc := newSendTransactionEventsTestUseCase(t, streamingFailingEmitter{})

	// Should complete without panicking.
	uc.SendTransactionEvents(context.Background(),
		transactionLifecycleFixture(nil, constant.APPROVED),
		TransactionLifecyclePhaseCreated)
}

// TestSendTransactionEvents_NilStreamingIsAllowed asserts the
// nil-emitter contract: a UseCase with no Streaming wired (nil emitter)
// completes without panicking and emits nothing. The IMPORTANT-posture
// contract treats nil as "streaming disabled".
func TestSendTransactionEvents_NilStreamingIsAllowed(t *testing.T) {
	uc := &UseCase{
		Streaming: nil,
	}

	// Must not panic with a nil emitter; nothing is wired to emit to.
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
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, mockEmitter)

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
