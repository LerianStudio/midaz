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
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newUpdateSegmentStreamingTestUseCase wires a happy-path UseCase
// suitable for exercising the segment.updated emission.
//
// SegmentRepo.Update is mocked to return a post-commit record carrying
// the request identity (ID/OrganizationID/LedgerID) plus the persisted
// UpdatedAt and a canonical name preserved from create time. This
// mirrors the contract of the refactored squirrel + RETURNING repo
// method — the use case trusts the repo's return value directly without
// merging against a pre-update fetch.
func newUpdateSegmentStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedUpdatedAt time.Time, canonicalName string) *UseCase {
	t.Helper()

	mockSegmentRepo := segment.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	mockSegmentRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, id uuid.UUID, in *mmodel.Segment) (*mmodel.Segment, error) {
			out := &mmodel.Segment{
				ID:             id.String(),
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Name:           canonicalName,
				Status:         in.Status,
				UpdatedAt:      fixedUpdatedAt,
			}
			// Mirror partial-update fidelity: if caller supplied a non-empty
			// Name, that beats the canonical name. If empty, canonical wins
			// (preserving create-time state).
			if in.Name != "" {
				out.Name = in.Name
			}
			return out, nil
		}).AnyTimes()

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mongodb.Metadata{Data: map[string]any{}}, nil).AnyTimes()

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		SegmentRepo:            mockSegmentRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		Streaming:              emitter,
	}
}

// TestUpdateSegmentByID_EmitsSegmentUpdatedEvent verifies that a
// successful UpdateSegmentByID call publishes exactly one
// segment.updated event with the expected resource/event types, tenant
// ID, subject and payload fields. Subject and identity fields must
// source from the post-commit repo return value (NOT a re-fetch).
func TestUpdateSegmentByID_EmitsSegmentUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newUpdateSegmentStreamingTestUseCase(t, ctrl, mockEmitter, fixedUpdatedAt, "Canonical Name")

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	segmentID := uuid.New()

	input := &mmodel.UpdateSegmentInput{
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	s, err := uc.UpdateSegmentByID(ctx, orgID, ledgerID, segmentID, input)
	require.NoError(t, err)
	require.NotNil(t, s)

	events := mockEmitter.Events()
	require.Len(t, events, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "segment", "updated")

	evt := events[0]
	assert.Equal(t, "segment.updated", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, segmentID.String(), evt.Subject, "Subject must be the request-path segment ID (from repo RETURNING)")
	assert.Equal(t, fixedUpdatedAt, evt.Timestamp, "Timestamp must pin to persisted UpdatedAt")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, segmentID.String(), payload["id"], "payload.id must source from repo RETURNING, not regenerated UUID")
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	// Name not provided in input → canonical preserved from create time.
	assert.Equal(t, "Canonical Name", payload["name"])
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])
	assert.Contains(t, payload, "status")
}

// TestUpdateSegmentByID_NoopEmitterDoesNotPanic confirms the production
// disabled-flag path: when Streaming is the NoopEmitter, UpdateSegmentByID
// succeeds without error and no panic.
func TestUpdateSegmentByID_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateSegmentStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedUpdatedAt, "Canonical Name")

	input := &mmodel.UpdateSegmentInput{
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	s, err := uc.UpdateSegmentByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, s)
}

// TestUpdateSegmentByID_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, UpdateSegmentByID must
// still return the successfully-persisted segment because durability is
// owned by PG + future DLQ/outbox, not by the synchronous Emit call.
func TestUpdateSegmentByID_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateSegmentStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedUpdatedAt, "Canonical Name")

	input := &mmodel.UpdateSegmentInput{
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	s, err := uc.UpdateSegmentByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, s)
}

// TestUpdateSegmentByID_NilStreamingDoesNotPanic confirms that a UseCase
// with a nil Streaming field (legacy / partial wiring) still completes
// the request — the emit block must be guarded.
func TestUpdateSegmentByID_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateSegmentStreamingTestUseCase(t, ctrl, nil, fixedUpdatedAt, "Canonical Name")

	input := &mmodel.UpdateSegmentInput{
		Status: mmodel.Status{Code: "ACTIVE"},
	}

	s, err := uc.UpdateSegmentByID(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, s)
}
