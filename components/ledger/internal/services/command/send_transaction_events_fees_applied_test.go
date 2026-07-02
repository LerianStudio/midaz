// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestSendTransactionEvents_FeesAppliedEmittedOnPostedCharge locks the
// charged-only + posted-only contract: a POSTED (created + APPROVED + nil
// parent) transaction carrying feeApplied=true and packageAppliedID emits both
// transaction.posted AND fees.applied.
func TestSendTransactionEvents_FeesAppliedEmittedOnPostedCharge(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	tran.CreatedAt = time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	packageID := uuid.New().String()
	tran.Metadata = map[string]any{
		"feeApplied":       "true",
		"packageAppliedID": packageID,
	}

	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "posted")
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "fees", "applied")

	var feesApplied *libStreaming.EmitRequest
	for i := range mockEmitter.Events() {
		if mockEmitter.Events()[i].DefinitionKey == "fees.applied" {
			feesApplied = &mockEmitter.Events()[i]
			break
		}
	}
	require.NotNil(t, feesApplied, "fees.applied request must be present in emitted events")

	assert.Equal(t, tran.ID, feesApplied.Subject,
		"fees.applied Subject must be the transaction id")

	var payload events.FeesAppliedPayload
	require.NoError(t, json.Unmarshal(feesApplied.Payload, &payload))
	assert.Equal(t, tran.ID, payload.TransactionID,
		"payload transactionId must match the fixture transaction id")
	assert.Equal(t, packageID, payload.FeePackageID,
		"payload feePackageId must match packageAppliedID from metadata")
}

// TestSendTransactionEvents_FeesAppliedSkippedWhenExemptionOnly locks the
// charged-only fence: a POSTED transaction with packageAppliedID but WITHOUT
// feeApplied (exemption-only) emits transaction.posted but NOT fees.applied.
func TestSendTransactionEvents_FeesAppliedSkippedWhenExemptionOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	tran.Metadata = map[string]any{
		"packageAppliedID": "pkg-123",
	}

	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "posted")

	for _, e := range mockEmitter.Events() {
		assert.NotEqual(t, "fees.applied", e.DefinitionKey,
			"exemption-only transaction must not emit fees.applied")
	}
}

// TestSendTransactionEvents_FeesAppliedSkippedWhenPackageIDEmpty locks the
// second emit guard: a POSTED transaction with feeApplied=true but an empty
// packageAppliedID emits transaction.posted but NOT fees.applied.
func TestSendTransactionEvents_FeesAppliedSkippedWhenPackageIDEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	tran.Metadata = map[string]any{
		"feeApplied":       "true",
		"packageAppliedID": "",
	}

	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "posted")

	for _, e := range mockEmitter.Events() {
		assert.NotEqual(t, "fees.applied", e.DefinitionKey,
			"empty packageAppliedID must not emit fees.applied")
	}
}

// TestSendTransactionEvents_FeesAppliedSkippedOnNonPostedPhase locks the
// posted-only fence: a charged transaction on the updated (committed) phase
// does NOT re-emit fees.applied.
func TestSendTransactionEvents_FeesAppliedSkippedOnNonPostedPhase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newSendTransactionEventsTestUseCase(t, ctrl, mockEmitter)

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	tran.Metadata = map[string]any{
		"feeApplied":       "true",
		"packageAppliedID": "pkg-123",
	}

	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseUpdated)

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "committed")

	for _, e := range mockEmitter.Events() {
		assert.NotEqual(t, "fees.applied", e.DefinitionKey,
			"committed (updated) phase must not emit fees.applied")
	}
}

// TestSendTransactionEvents_FeesAppliedNilStreamingDoesNotPanic asserts the
// IMPORTANT-posture nil-emitter contract: a charged POSTED transaction with a
// NoopEmitter completes without panicking.
func TestSendTransactionEvents_FeesAppliedNilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newSendTransactionEventsTestUseCase(t, ctrl, streamingFailingEmitter{})

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	tran.Metadata = map[string]any{
		"feeApplied":       "true",
		"packageAppliedID": "pkg-123",
	}

	// Must not panic under a failing emitter (IMPORTANT posture).
	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)
}
