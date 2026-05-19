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
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operationroute"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newDeleteOperationRouteStreamingTestUseCase wires a happy-path
// UseCase suitable for exercising the operation-route.deleted
// emission. OperationRouteRepo.HasTransactionRouteLinks reports no
// links (so the delete path is not short-circuited) and
// OperationRouteRepo.Delete returns nil for the success path.
func newDeleteOperationRouteStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	mockOperationRouteRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	mockOperationRouteRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
		Streaming:          emitter,
	}
}

// TestDeleteOperationRouteByID_EmitsOperationRouteDeletedEvent
// verifies that a successful DeleteOperationRouteByID call publishes
// exactly one operation-route.deleted event with the expected
// resource/event types, tenant ID, subject and payload fields.
func TestDeleteOperationRouteByID_EmitsOperationRouteDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeleteOperationRouteStreamingTestUseCase(t, ctrl, mockEmitter)

	before := time.Now()
	err := uc.DeleteOperationRouteByID(context.Background(), orgID, ledgerID, operationRouteID)
	after := time.Now()
	require.NoError(t, err)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "operation-route", "deleted")

	evt := emitted[0]
	assert.Equal(t, "operation-route.deleted", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, operationRouteID.String(), evt.Subject, "Subject must be the deleted operation route ID")

	// Timestamp must be the wall-clock instant captured at the emit
	// site — bounded by the test's before/after window.
	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)), "Timestamp earlier than before window")
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)), "Timestamp later than after window")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, operationRouteID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.NotEmpty(t, payload["deletedAt"], "deletedAt must be set (RFC3339)")
}

// TestDeleteOperationRouteByID_NoopEmitterDoesNotPanic confirms the
// production disabled-flag path: when Streaming is the NoopEmitter,
// DeleteOperationRouteByID succeeds without error and no panic.
func TestDeleteOperationRouteByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteOperationRouteStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	err := uc.DeleteOperationRouteByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}

// TestDeleteOperationRouteByID_EmitFailureDoesNotFailRequest verifies
// the IMPORTANT posture: when Emit returns an error,
// DeleteOperationRouteByID must still complete successfully because
// durability is owned by PG + future DLQ/outbox, not by the
// synchronous Emit call.
func TestDeleteOperationRouteByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteOperationRouteStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	err := uc.DeleteOperationRouteByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
}

// TestDeleteOperationRouteByID_NilStreamingDoesNotPanic confirms that
// a UseCase with a nil Streaming field (legacy / partial wiring) still
// completes the request — the emit block must be guarded.
func TestDeleteOperationRouteByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteOperationRouteStreamingTestUseCase(t, ctrl, nil)

	err := uc.DeleteOperationRouteByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}
