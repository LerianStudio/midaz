// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newCreateSegmentStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the segment.created emission. SegmentRepo.Create
// echoes the input with a server-assigned ID so the test body can assert
// the emitted Subject and payload.id without prior coordination. The
// tests do not exercise the metadata branch — CreateSegmentInput.Metadata
// is nil, so CreateOnboardingMetadata short-circuits before calling the
// metadata repo.
func newCreateSegmentStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter) *UseCase {
	t.Helper()

	mockSegmentRepo := segment.NewMockRepository(ctrl)

	mockSegmentRepo.EXPECT().
		ExistsByName(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, nil).AnyTimes()

	mockSegmentRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *mmodel.Segment) (*mmodel.Segment, error) {
			out := *in
			out.ID = uuid.New().String()
			return &out, nil
		}).AnyTimes()

	return &UseCase{
		SegmentRepo: mockSegmentRepo,
		Streaming:   emitter,
	}
}

// TestCreateSegment_EmitsSegmentCreatedEvent verifies that a successful
// CreateSegment call publishes exactly one segment.created event with
// the expected resource/event types, tenant ID, subject and payload
// fields.
func TestCreateSegment_EmitsSegmentCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newCreateSegmentStreamingTestUseCase(t, ctrl, mockEmitter)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateSegmentInput{
		Name: "Retail Segment",
	}

	s, err := uc.CreateSegment(ctx, orgID, ledgerID, input)
	require.NoError(t, err)
	require.NotNil(t, s)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "segment", "created")

	evt := events[0]
	assert.Equal(t, "segment.created", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, s.ID, evt.Subject, "Subject must be the new segment ID")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, s.ID, payload["id"])
	assert.Equal(t, s.OrganizationID, payload["organizationId"])
	assert.Equal(t, s.LedgerID, payload["ledgerId"])
	assert.Equal(t, "Retail Segment", payload["name"])
	assert.NotEmpty(t, payload["createdAt"], "createdAt must be set (RFC3339)")
	assert.NotEmpty(t, payload["updatedAt"], "updatedAt must be set (RFC3339)")
	assert.Contains(t, payload, "status")
}

// TestCreateSegment_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, CreateSegment
// succeeds without error and no panic.
func TestCreateSegment_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateSegmentStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter())

	input := &mmodel.CreateSegmentInput{Name: "Noop Segment"}

	s, err := uc.CreateSegment(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, s)
}

// TestCreateSegment_EmitFailureDoesNotFailRequest verifies the IMPORTANT
// posture: when Emit returns an error, CreateSegment must still return
// the successfully-persisted segment because durability is owned by
// PG + future DLQ/outbox, not by the synchronous Emit call.
func TestCreateSegment_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateSegmentStreamingTestUseCase(t, ctrl, streamingFailingEmitter{})

	input := &mmodel.CreateSegmentInput{Name: "Emit Fail Segment"}

	s, err := uc.CreateSegment(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, s)
}

// TestCreateSegment_NilStreamingDoesNotPanic confirms that a UseCase
// with a nil Streaming field (legacy / partial wiring) still completes
// the request — the emit block must be guarded.
func TestCreateSegment_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := newCreateSegmentStreamingTestUseCase(t, ctrl, nil)

	input := &mmodel.CreateSegmentInput{Name: "Nil Streaming Segment"}

	s, err := uc.CreateSegment(context.Background(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, s)
}
