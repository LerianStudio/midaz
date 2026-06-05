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
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newUpdateOperationRouteStreamingTestUseCase wires a happy-path
// UseCase suitable for exercising the operation-route.updated
// emission.
//
// OperationRouteRepo.Update is mocked to return a post-commit record
// carrying the request identity (ID/OrganizationID/LedgerID), the
// canonical OperationType preserved from create time, and the
// persisted UpdatedAt. This mirrors the contract of the refactored
// squirrel + RETURNING repo method — the use case trusts the repo's
// return value directly without merging against a pre-update fetch.
func newUpdateOperationRouteStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedUpdatedAt time.Time, canonicalTitle, canonicalOperationType string) *UseCase {
	t.Helper()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	mockOperationRouteRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID uuid.UUID, id uuid.UUID, in *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			out := &mmodel.OperationRoute{
				ID:                id,
				OrganizationID:    orgID,
				LedgerID:          ledgerID,
				Title:             canonicalTitle,
				Description:       in.Description,
				Code:              in.Code,
				OperationType:     canonicalOperationType,
				Account:           in.Account,
				AccountingEntries: in.AccountingEntries,
				UpdatedAt:         fixedUpdatedAt,
			}
			// Mirror partial-update fidelity: if caller supplied a non-empty
			// Title, that beats the canonical title. If empty, canonical
			// wins (preserving create-time state).
			if in.Title != "" {
				out.Title = in.Title
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
		OperationRouteRepo:      mockOperationRouteRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		Streaming:               emitter,
	}
}

// TestUpdateOperationRoute_EmitsOperationRouteUpdatedEvent verifies
// that a successful UpdateOperationRoute call publishes exactly one
// operation-route.updated event with the expected resource/event
// types, tenant ID, subject and payload fields. Subject and identity
// fields must source from the post-commit repo return value (NOT a
// re-fetch).
func TestUpdateOperationRoute_EmitsOperationRouteUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newUpdateOperationRouteStreamingTestUseCase(t, ctrl, mockEmitter, fixedUpdatedAt, "Canonical Title", "source")

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	operationRouteID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:       "Updated Operation Route Title",
		Description: "Updated description",
	}

	o, err := uc.UpdateOperationRoute(ctx, orgID, ledgerID, operationRouteID, input)
	require.NoError(t, err)
	require.NotNil(t, o)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "operation-route", "updated")

	evt := emitted[0]
	assert.Equal(t, "operation-route.updated", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, operationRouteID.String(), evt.Subject, "Subject must be the request-path operation route ID (from repo RETURNING)")
	assert.Equal(t, fixedUpdatedAt, evt.Timestamp, "Timestamp must pin to persisted UpdatedAt")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, operationRouteID.String(), payload["id"], "payload.id must source from repo RETURNING")
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.Equal(t, "Updated Operation Route Title", payload["title"])
	assert.Equal(t, "Updated description", payload["description"])
	assert.Equal(t, "source", payload["operationType"], "operationType is immutable post-create; canonical preserved")
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])
}

// TestUpdateOperationRoute_NoopEmitterDoesNotPanic confirms the
// production disabled-flag path: when Streaming is the NoopEmitter,
// UpdateOperationRoute succeeds without error and no panic.
func TestUpdateOperationRoute_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateOperationRouteStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedUpdatedAt, "Canonical Title", "source")

	input := &mmodel.UpdateOperationRouteInput{Title: "Noop Updated Operation Route"}

	o, err := uc.UpdateOperationRoute(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, o)
}

// TestUpdateOperationRoute_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture: when Emit returns an error, UpdateOperationRoute
// must still return the successfully-persisted operation route because
// durability is owned by PG + future DLQ/outbox, not by the synchronous
// Emit call.
func TestUpdateOperationRoute_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateOperationRouteStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedUpdatedAt, "Canonical Title", "source")

	input := &mmodel.UpdateOperationRouteInput{Title: "Emit Fail Updated Operation Route"}

	o, err := uc.UpdateOperationRoute(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, o)
}

// TestUpdateOperationRoute_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field (legacy / partial wiring) still
// completes the request — the emit block must be guarded.
func TestUpdateOperationRoute_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateOperationRouteStreamingTestUseCase(t, ctrl, nil, fixedUpdatedAt, "Canonical Title", "source")

	input := &mmodel.UpdateOperationRouteInput{Title: "Nil Streaming Updated Operation Route"}

	o, err := uc.UpdateOperationRoute(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, o)
}
