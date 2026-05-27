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
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/segment"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newDeleteSegmentStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the segment.deleted emission. SegmentRepo.Delete
// returns nil for the success path.
func newDeleteSegmentStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockSegmentRepo := segment.NewMockRepository(ctrl)

	mockSegmentRepo.EXPECT().
		Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		SegmentRepo: mockSegmentRepo,
		Streaming:   emitter,
	}
}

// TestDeleteSegmentByID_EmitsSegmentDeletedEvent verifies that a
// successful DeleteSegmentByID call publishes exactly one
// segment.deleted event with the expected resource/event types,
// tenant ID, subject and payload fields.
func TestDeleteSegmentByID_EmitsSegmentDeletedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	segmentID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newDeleteSegmentStreamingTestUseCase(t, ctrl, mockEmitter)

	before := time.Now()
	err := uc.DeleteSegmentByID(context.Background(), orgID, ledgerID, segmentID)
	after := time.Now()
	require.NoError(t, err)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "segment", "deleted")

	evt := events[0]
	assert.Equal(t, "segment.deleted", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, segmentID.String(), evt.Subject, "Subject must be the deleted segment ID")

	// Timestamp must be the wall-clock instant captured at the emit
	// site — bounded by the test's before/after window.
	assert.False(t, evt.Timestamp.Before(before.Add(-time.Second)), "Timestamp earlier than before window")
	assert.False(t, evt.Timestamp.After(after.Add(time.Second)), "Timestamp later than after window")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, segmentID.String(), payload["id"])
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.NotEmpty(t, payload["deletedAt"], "deletedAt must be set (RFC3339)")
}

// TestDeleteSegmentByID_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, DeleteSegmentByID
// succeeds without error and no panic.
func TestDeleteSegmentByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteSegmentStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	err := uc.DeleteSegmentByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}

// TestDeleteSegmentByID_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, DeleteSegmentByID
// must still complete successfully because durability is owned by PG +
// future DLQ/outbox, not by the synchronous Emit call.
func TestDeleteSegmentByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteSegmentStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	err := uc.DeleteSegmentByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
}

// TestDeleteSegmentByID_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field (legacy / partial wiring) still
// completes the request — the emit block must be guarded.
func TestDeleteSegmentByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newDeleteSegmentStreamingTestUseCase(t, ctrl, nil)

	err := uc.DeleteSegmentByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
}
