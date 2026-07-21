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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// newUpdateTransactionRouteStreamingTestUseCase wires a happy-path
// UseCase suitable for exercising the transaction-route.updated
// emission.
//
// TransactionRouteRepo.Update returns a post-commit record carrying
// the request identity (ID/OrganizationID/LedgerID) and the persisted
// UpdatedAt — mirroring the contract of the refactored squirrel +
// RETURNING repo method.
//
// FindOperationRouteIDsByTransactionRouteIDs returns the configured
// preserved IDs so the post-update hydration path covers the
// `input.OperationRoutes == nil` case (i.e. PATCHes that omit the
// link set should still see the canonical post-state on the wire).
func newUpdateTransactionRouteStreamingTestUseCase(t *testing.T, ctrl *gomock.Controller, emitter libStreaming.Emitter, fixedUpdatedAt time.Time, canonicalTitle string, preservedOperationRouteIDs []uuid.UUID) *UseCase {
	t.Helper()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	mockTransactionRouteRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, orgID, ledgerID, id uuid.UUID, in *mmodel.TransactionRoute, _, _ []uuid.UUID) (*mmodel.TransactionRoute, error) {
			out := &mmodel.TransactionRoute{
				ID:             id,
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Title:          canonicalTitle,
				Description:    in.Description,
				UpdatedAt:      fixedUpdatedAt,
			}
			// Partial-update fidelity: if caller supplied a non-empty
			// Title, that beats canonical.
			if in.Title != "" {
				out.Title = in.Title
			}
			return out, nil
		}).AnyTimes()

	mockTransactionRouteRepo.EXPECT().
		FindOperationRouteIDsByTransactionRouteIDs(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, trIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
			result := make(map[uuid.UUID][]uuid.UUID)
			if len(trIDs) > 0 {
				result[trIDs[0]] = preservedOperationRouteIDs
			}
			return result, nil
		}).AnyTimes()

	// Hydration step: return mirror OperationRoute objects so the
	// streaming payload's operationRouteIds slice is populated.
	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, ids []uuid.UUID) ([]*mmodel.OperationRoute, error) {
			routes := make([]*mmodel.OperationRoute, 0, len(ids))
			for _, id := range ids {
				routes = append(routes, &mmodel.OperationRoute{ID: id, OperationType: "source"})
			}
			return routes, nil
		}).AnyTimes()

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mongodb.Metadata{Data: map[string]any{}}, nil).AnyTimes()

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	return &UseCase{
		TransactionRouteRepo:    mockTransactionRouteRepo,
		OperationRouteRepo:      mockOperationRouteRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		Streaming:               emitter,
	}
}

// TestUpdateTransactionRoute_EmitsTransactionRouteUpdatedEvent verifies
// that a successful UpdateTransactionRoute call (with omitted
// operationRoutes — exercising the post-update hydration path)
// publishes exactly one transaction-route.updated event with the
// expected resource/event types, tenant ID, subject and payload
// fields.
func TestUpdateTransactionRoute_EmitsTransactionRouteUpdatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	preservedOR1 := uuid.New()
	preservedOR2 := uuid.New()

	mockEmitter := pkgStreaming.NewMockEmitter()
	uc := newUpdateTransactionRouteStreamingTestUseCase(t, ctrl, mockEmitter, fixedUpdatedAt, "Canonical Title", []uuid.UUID{preservedOR1, preservedOR2})

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionRouteID := uuid.New()

	input := &mmodel.UpdateTransactionRouteInput{
		Title:       "Updated Transaction Route Title",
		Description: "Updated description",
	}

	tr, err := uc.UpdateTransactionRoute(ctx, orgID, ledgerID, transactionRouteID, input)
	require.NoError(t, err)
	require.NotNil(t, tr)

	emitted := mockEmitter.Events()
	require.Len(t, emitted, 1, "expected exactly one Emit call")

	pkgStreaming.AssertEventEmitted(t, mockEmitter, "transaction-route", "updated")

	evt := emitted[0]
	assert.Equal(t, "transaction-route.updated", evt.DefinitionKey, "DefinitionKey must match the catalog key")
	assert.Equal(t, "default", evt.TenantID, "TenantID must come from ResolveTenantID (default fallback when no multi-tenant context)")
	assert.Equal(t, transactionRouteID.String(), evt.Subject, "Subject must be the request-path transaction route ID (from repo RETURNING)")
	assert.Equal(t, fixedUpdatedAt, evt.Timestamp, "Timestamp must pin to persisted UpdatedAt")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(evt.Payload, &payload))

	assert.Equal(t, transactionRouteID.String(), payload["id"], "payload.id must source from repo RETURNING")
	assert.Equal(t, orgID.String(), payload["organizationId"])
	assert.Equal(t, ledgerID.String(), payload["ledgerId"])
	assert.Equal(t, "Updated Transaction Route Title", payload["title"])
	assert.Equal(t, "Updated description", payload["description"])
	assert.Equal(t, "2026-05-13T12:34:56Z", payload["updatedAt"])

	// === Post-update link set ===
	// Since the caller omitted OperationRoutes from the PATCH, the
	// producer MUST source the canonical post-state from the join
	// table via FindOperationRouteIDsByTransactionRouteIDs +
	// FindByIDs.
	ids, ok := payload["operationRouteIds"].([]any)
	require.True(t, ok, "operationRouteIds must be present as an array")
	require.Len(t, ids, 2, "operationRouteIds must include the preserved canonical links")
}

// TestUpdateTransactionRoute_NoopEmitterDoesNotPanic confirms the
// production disabled-flag path.
func TestUpdateTransactionRoute_NoopEmitterDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateTransactionRouteStreamingTestUseCase(t, ctrl, libStreaming.NewNoopEmitter(), fixedUpdatedAt, "Canonical Title", nil)

	input := &mmodel.UpdateTransactionRouteInput{Title: "Noop Updated Transaction Route"}

	tr, err := uc.UpdateTransactionRoute(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, tr)
}

// TestUpdateTransactionRoute_EmitFailureDoesNotFailRequest verifies the
// IMPORTANT posture.
func TestUpdateTransactionRoute_EmitFailureDoesNotFailRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateTransactionRouteStreamingTestUseCase(t, ctrl, streamingFailingEmitter{}, fixedUpdatedAt, "Canonical Title", nil)

	input := &mmodel.UpdateTransactionRouteInput{Title: "Emit Fail Updated Transaction Route"}

	tr, err := uc.UpdateTransactionRoute(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err, "Emit failure must NOT fail the request (IMPORTANT posture)")
	require.NotNil(t, tr)
}

// TestUpdateTransactionRoute_NilStreamingDoesNotPanic confirms that a
// UseCase with a nil Streaming field still completes the request.
func TestUpdateTransactionRoute_NilStreamingDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixedUpdatedAt := time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	uc := newUpdateTransactionRouteStreamingTestUseCase(t, ctrl, nil, fixedUpdatedAt, "Canonical Title", nil)

	input := &mmodel.UpdateTransactionRouteInput{Title: "Nil Streaming Updated Transaction Route"}

	tr, err := uc.UpdateTransactionRoute(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
	require.NoError(t, err)
	require.NotNil(t, tr)
}
