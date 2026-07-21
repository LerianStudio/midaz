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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newDeleteTransactionRouteStreamingTestUseCase wires a happy-path
// UseCase suitable for exercising the transaction-route.deleted
// emission. FindByID returns a minimal record with one operation
// route (so the toRemove slice is non-empty), and Delete returns nil.
func newDeleteTransactionRouteStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)

	mockTransactionRouteRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
			return &mmodel.TransactionRoute{
				ID:             id,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				OperationRoutes: []mmodel.OperationRoute{
					{ID: uuid.New()},
				},
			}, nil
		}).AnyTimes()

	mockTransactionRouteRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		Streaming:            emitter,
	}
}

// TestDeleteTransactionRouteByID_EmitsTransactionRouteDeletedEvent
// verifies that a successful DeleteTransactionRouteByID call publishes
// exactly one transaction-route.deleted event with the expected
// resource/event types, tenant ID, subject and payload fields.
func TestDeleteTransactionRouteByID_EmitsTransactionRouteDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	transactionRouteID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeleteTransactionRouteStreamingTestUseCase(t, ctrl, mockEmitter)

	before := time.Now()
	err := uc.DeleteTransactionRouteByID(context.Background(), orgID, ledgerID, transactionRouteID)
	after := time.Now()
	require.NoError(t, err)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction-route", "deleted")

	evt := emitted[0]
	assert.Equal(t, "transaction-route.deleted", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, transactionRouteID.String(), evt.Subject, "Subject must be the deleted transaction route ID")

	// Timestamp must be the wall-clock instant captured at the emit
	// site — bounded by the test's before/after window.
	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)), "Timestamp earlier than before window")
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)), "Timestamp later than after window")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, transactionRouteID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.NotEmpty(t, payload["deletedAt"], "deletedAt must be set (RFC3339)")
}

// TestDeleteTransactionRouteByID_NoopEmitterDoesNotPanic confirms the
// production disabled-flag path.
func TestDeleteTransactionRouteByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteTransactionRouteStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	err := uc.DeleteTransactionRouteByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}

// TestDeleteTransactionRouteByID_EmitFailureDoesNotFailRequest verifies
// the IMPORTANT posture.
func TestDeleteTransactionRouteByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteTransactionRouteStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	err := uc.DeleteTransactionRouteByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
}

// TestDeleteTransactionRouteByID_NilStreamingDoesNotPanic confirms that
// a UseCase with a nil Streaming field still completes the request.
func TestDeleteTransactionRouteByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteTransactionRouteStreamingTestUseCase(t, ctrl, nil)

	err := uc.DeleteTransactionRouteByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}
