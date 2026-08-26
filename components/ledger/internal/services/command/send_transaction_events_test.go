// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestSendTransactionEvents locks the post-legacy contract: a persisted
// transaction ALWAYS produces its lib-streaming lifecycle event and NEVER
// publishes to the retired transaction-events RabbitMQ exchange. The
// (phase, status, parent) triple selects the emitted event; the mock
// emitter is the only asserted transport, and the UseCase carries no
// producer repository so a legacy publish is impossible by construction.
func TestSendTransactionEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		phase        string
		status       string
		withParent   bool
		wantResource string
		wantEvent    string
	}{
		{
			name:         "posted on created + approved + no parent",
			phase:        TransactionLifecyclePhaseCreated,
			status:       constant.APPROVED,
			wantResource: "transaction",
			wantEvent:    "posted",
		},
		{
			name:         "reverted on created + approved + parent",
			phase:        TransactionLifecyclePhaseCreated,
			status:       constant.APPROVED,
			withParent:   true,
			wantResource: "transaction",
			wantEvent:    "reverted",
		},
		{
			name:         "committed on updated + approved",
			phase:        TransactionLifecyclePhaseUpdated,
			status:       constant.APPROVED,
			wantResource: "transaction",
			wantEvent:    "committed",
		},
		{
			name:         "canceled on updated + canceled",
			phase:        TransactionLifecyclePhaseUpdated,
			status:       constant.CANCELED,
			wantResource: "transaction",
			wantEvent:    "canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockEmitter := pkgStreaming.NewMockEmitter()
			uc := &UseCase{Streaming: mockEmitter}

			var parentID *string
			if tt.withParent {
				pid := uuid.New().String()
				parentID = &pid
			}

			tran := transactionLifecycleFixture(parentID, tt.status)
			uc.SendTransactionEvents(context.Background(), tran, tt.phase)

			emitted := mockEmitter.Events()
			require.Len(t, emitted, 1, "exactly one lifecycle streaming event must fire")
			pkgStreaming.AssertEventEmitted(t, mockEmitter, tt.wantResource, tt.wantEvent)
		})
	}
}

// TestSendTransactionEvents_PostedWithFeeMetadataEmitsFeeCharge locks that a
// posted transaction carrying charged-fee metadata emits both
// transaction.posted and fee_charge.applied on the same fresh-insert emit,
// and still never touches the legacy rabbit exchange.
func TestSendTransactionEvents_PostedWithFeeMetadataEmitsFeeCharge(t *testing.T) {
	t.Parallel()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := &UseCase{Streaming: mockEmitter}

	tran := transactionLifecycleFixture(nil, constant.APPROVED)
	tran.CreatedAt = time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	packageID := uuid.New().String()
	tran.Metadata = map[string]any{
		"feeApplied":       "true",
		"packageAppliedID": packageID,
	}

	uc.SendTransactionEvents(context.Background(), tran, TransactionLifecyclePhaseCreated)

	require.Len(t, mockEmitter.Events(), 2, "posted + fee_charge.applied only")
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction", "posted")
	pkgStreaming.AssertEventEmitted(t, mockEmitter, "fee_charge", "applied")
}
